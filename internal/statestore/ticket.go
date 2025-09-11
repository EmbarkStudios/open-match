// Copyright 2019 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package statestore

import (
	"context"
	"fmt"
	"github.com/sirupsen/logrus"
	"iter"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/gomodule/redigo/redis"
	"github.com/pkg/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"open-match.dev/open-match/internal/config"
	"open-match.dev/open-match/pkg/pb"
)

const (
	allTickets        = "allTickets"
	proposedTicketIDs = "proposed_ticket_ids"
	allTicketsWithTTL = "allTicketsWithTTL"
)

// CreateTicket creates a new Ticket in the state storage. If the id already exists, it will be overwritten.
func (rb *redisBackend) CreateTicket(ctx context.Context, ticket *pb.Ticket) error {

	redisConn, err := rb.redisPool.GetContext(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "CreateTicket, id: %s, failed to connect to redis: %v", ticket.GetId(), err)
	}
	defer handleConnectionClose(&redisConn)

	value, err := proto.Marshal(ticket)
	if err != nil {
		err = errors.Wrapf(err, "failed to marshal the ticket proto, id: %s", ticket.GetId())
		return status.Errorf(codes.Internal, "%v", err)
	}

	if value == nil {
		return status.Errorf(codes.Internal, "failed to marshal the ticket proto, id: %s: proto: Marshal called with nil", ticket.GetId())
	}

	_, err = redisConn.Do("SET", ticket.GetId(), value)
	if err != nil {
		err = errors.Wrapf(err, "failed to set the value for ticket, id: %s", ticket.GetId())
		return status.Errorf(codes.Internal, "%v", err)
	}

	return nil
}

// GetTicket gets the Ticket with the specified id from state storage. This method fails if the Ticket does not exist.
func (rb *redisBackend) GetTicket(ctx context.Context, id string) (*pb.Ticket, error) {
	redisConn, err := rb.redisPool.GetContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "GetTicket, id: %s, failed to connect to redis: %v", id, err)
	}
	defer handleConnectionClose(&redisConn)

	value, err := redis.Bytes(redisConn.Do("GET", id))
	if err != nil {
		// Return NotFound if redigo did not find the ticket in storage.
		if err == redis.ErrNil {
			msg := fmt.Sprintf("Ticket id: %s not found", id)
			return nil, status.Error(codes.NotFound, msg)
		}

		err = errors.Wrapf(err, "failed to get the ticket from state storage, id: %s", id)
		return nil, status.Errorf(codes.Internal, "%v", err)
	}

	if value == nil {
		msg := fmt.Sprintf("Ticket id: %s not found", id)
		return nil, status.Error(codes.NotFound, msg)
	}

	ticket := &pb.Ticket{}
	err = proto.Unmarshal(value, ticket)
	if err != nil {
		err = errors.Wrapf(err, "failed to unmarshal the ticket proto, id: %s", id)
		return nil, status.Errorf(codes.Internal, "%v", err)
	}

	// if a ticket is assigned, its given an TTL and automatically de-indexed
	if ticket.Assignment == nil {
		expiredKey, err := rb.checkIfExpired(ctx, id)
		if err != nil {
			err = errors.Wrapf(err, "failed to check if ticket is expired, id: %s", id)
			return nil, err
		}

		if !expiredKey {
			ticketTTL := getTicketReleaseTimeout(rb.cfg)
			expiry := time.Now().Add(ticketTTL).UnixNano()

			_, err = redisConn.Do("ZADD", allTicketsWithTTL, "XX", "CH", expiry, id)
			if err != nil {
				err = errors.Wrapf(err, "failed to update score for ticket id: %s", id)
				return nil, status.Errorf(codes.Internal, "%v", err)
			}
		}
	}

	return ticket, nil
}

func (rb *redisBackend) UpdateIndexedTicketTTL(ctx context.Context, ticketId string) error {
	m := rb.NewMutex(ticketId)
	//  TODO: should we add a timeout to this acquisition
	err := m.Lock(ctx)
	if err != nil {
		return err
	}

	defer func() {
		if _, err = m.Unlock(context.Background()); err != nil {
			logger.WithError(err).Error("error on mutex unlock")
		}
	}()

	redisConn, err := rb.redisPool.GetContext(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "checkIfExpired, id: %s, failed to connect to redis: %v", ticketId, err)
	}
	defer handleConnectionClose(&redisConn)

	score, err := redis.Float64(redisConn.Do("ZSCORE", allTicketsWithTTL, ticketId))
	if errors.Is(err, redis.ErrNil) {
		logger.WithError(err).Errorf("Ticket id: %s not found", ticketId)
		return nil
	}
	if err != nil {
		return err
	}

	if score <= float64(time.Now().UnixNano()) {
		return errors.New("ticket is already expired")
	}

	ticketTTL := getTicketReleaseTimeout(rb.cfg)
	expiry := time.Now().Add(ticketTTL).UnixNano()

	_, err = redisConn.Do("ZADD", allTicketsWithTTL, "XX", "CH", expiry, ticketId)
	if err != nil {
		err = errors.Wrapf(err, "failed to update score for ticket id: %s", ticketId)
		return status.Errorf(codes.Internal, "%v", err)
	}

	return nil
}

func (rb *redisBackend) checkIfExpired(ctx context.Context, id string) (bool, error) {
	m := rb.NewMutex(id)
	//  TODO: should we add a timeout to this acquisition
	err := m.Lock(ctx)
	if err != nil {
		return false, err
	}

	defer func() {
		if _, err = m.Unlock(context.Background()); err != nil {
			logger.WithError(err).Error("error on mutex unlock")
		}
	}()

	redisConn, err := rb.redisPool.GetContext(ctx)
	if err != nil {
		return false, status.Errorf(codes.Unavailable, "checkIfExpired, id: %s, failed to connect to redis: %v", id, err)
	}
	defer handleConnectionClose(&redisConn)

	score, err := redis.Float64(redisConn.Do("ZSCORE", allTicketsWithTTL, id))
	if errors.Is(err, redis.ErrNil) {
		logger.WithError(err).Errorf("Ticket id: %s not found", id)
		return true, nil
	}
	if err != nil {
		return false, err
	}

	return score <= float64(time.Now().UnixNano()), nil
}

// DeleteTicket removes the Ticket with the specified id from state storage.
func (rb *redisBackend) DeleteTicket(ctx context.Context, id string) error {
	redisConn, err := rb.redisPool.GetContext(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "DeleteTicket, id: %s, failed to connect to redis: %v", id, err)
	}
	defer handleConnectionClose(&redisConn)

	value, err := redis.Int(redisConn.Do("DEL", id))
	if err != nil {
		err = errors.Wrapf(err, "failed to delete the ticket from state storage, id: %s", id)
		return status.Errorf(codes.Internal, "%v", err)
	}

	if value == 0 {
		return status.Errorf(codes.NotFound, "Ticket id: %s not found", id)
	}

	return nil
}

// DeleteTickets removes the Tickets with the specified id from state storage.
func (rb *redisBackend) DeleteTickets(ctx context.Context, ids []string) error {
	redisConn, err := rb.redisPool.GetContext(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "DeleteTickets, id: %v, failed to connect to redis: %v", ids, err)
	}
	defer handleConnectionClose(&redisConn)

	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	value, err := redis.Int(redisConn.Do("DEL", args...))
	if err != nil {
		err = errors.Wrapf(err, "failed to delete tickets from state storage, ids: %v", ids)
		return status.Errorf(codes.Internal, "%v", err)
	}

	if value == 0 {
		return status.Errorf(codes.NotFound, "Ticket ids: %s not found", ids)
	}

	return nil
}

// IndexTicket indexes the Ticket id for the configured index fields.
func (rb *redisBackend) IndexTicket(ctx context.Context, ticket *pb.Ticket) error {
	redisConn, err := rb.redisPool.GetContext(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "IndexTicket, id: %s, failed to connect to redis: %v", ticket.GetId(), err)
	}
	defer handleConnectionClose(&redisConn)

	err = redisConn.Send("SADD", allTickets, ticket.Id)
	if err != nil {
		err = errors.Wrapf(err, "failed to add ticket to all tickets, id: %s", ticket.Id)
		return status.Errorf(codes.Internal, "%v", err)
	}

	ticketTTL := getTicketReleaseTimeout(rb.cfg)
	expiry := time.Now().Add(ticketTTL).UnixNano()
	err = redisConn.Send("ZADD", allTicketsWithTTL, expiry, ticket.Id)
	if err != nil {
		err = errors.Wrapf(err, "failed to add ticket to all tickets, id: %s", ticket.Id)
		return status.Errorf(codes.Internal, "%v", err)
	}

	return nil
}

// DeindexTicket removes the indexing for the specified Ticket. Only the indexes are removed but the Ticket continues to exist.
func (rb *redisBackend) DeindexTicket(ctx context.Context, id string) error {
	redisConn, err := rb.redisPool.GetContext(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "DeindexTicket, id: %s, failed to connect to redis: %v", id, err)
	}
	defer handleConnectionClose(&redisConn)

	err = redisConn.Send("SREM", allTickets, id)
	if err != nil {
		err = errors.Wrapf(err, "failed to remove ticket from all tickets, id: %s", id)
		return status.Errorf(codes.Internal, "%v", err)
	}

	err = redisConn.Send("ZREM", allTicketsWithTTL, id)
	if err != nil {
		err = errors.Wrapf(err, "failed to remove ticket from all tickets, id: %s", id)
		return status.Errorf(codes.Internal, "%v", err)
	}

	return nil
}

// DeindexTickets removes the indexing for the specified Tickets. Only the indexes are removed but Tickets continues to exist.
func (rb *redisBackend) DeindexTickets(ctx context.Context, ids []string) error {
	redisConn, err := rb.redisPool.GetContext(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "DeindexTickets, id: %v, failed to connect to redis: %v", ids, err)
	}
	defer handleConnectionClose(&redisConn)

	args := make([]any, len(ids)+1)
	args[0] = allTickets
	for i, id := range ids {
		args[i+1] = id
	}

	err = redisConn.Send("SREM", args...)
	if err != nil {
		err = errors.Wrapf(err, "failed to remove ticket from all tickets, id: %v", ids)
		return status.Errorf(codes.Internal, "%v", err)
	}

	args[0] = allTicketsWithTTL
	err = redisConn.Send("ZREM", args...)
	if err != nil {
		err = errors.Wrapf(err, "failed to remove ticket from all tickets, id: %v", ids)
		return status.Errorf(codes.Internal, "%v", err)
	}

	return nil
}

// GetIndexedIds returns the ids of all tickets currently indexed.
func (rb *redisBackend) GetIndexedIDSet(ctx context.Context) (map[string]struct{}, error) {
	redisConn, err := rb.redisPool.GetContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "GetIndexedIDSet, failed to connect to redis: %v", err)
	}
	defer handleConnectionClose(&redisConn)

	ttl := getBackfillReleaseTimeout(rb.cfg)
	curTime := time.Now()
	endTimeInt := curTime.Add(time.Hour).UnixNano()
	startTimeInt := curTime.Add(-ttl).UnixNano()

	// Filter out tickets that are fetched but not assigned within ttl time (ms).
	idsInPendingReleases, err := redis.Strings(redisConn.Do("ZRANGEBYSCORE", proposedTicketIDs, startTimeInt, endTimeInt))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error getting pending release %v", err)
	}

	idsIndexed, err := redis.Strings(redisConn.Do("SMEMBERS", allTickets))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error getting all indexed ticket ids %v", err)
	}

	r := make(map[string]struct{}, len(idsIndexed))
	for _, id := range idsIndexed {
		r[id] = struct{}{}
	}
	for _, id := range idsInPendingReleases {
		delete(r, id)
	}

	return r, nil
}

// GetIndexedIDSetWithTTL returns the ids of all tickets currently indexed but within a given TTL.
func (rb *redisBackend) GetIndexedIDSetWithTTL(ctx context.Context) (map[string]struct{}, error) {
	redisConn, err := rb.redisPool.GetContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "GetIndexedIDSetWithTTL, failed to connect to redis: %v", err)
	}
	defer handleConnectionClose(&redisConn)

	ttl := getBackfillReleaseTimeout(rb.cfg)
	curTime := time.Now()
	endTimeInt := curTime.Add(time.Hour).UnixNano()
	startTimeInt := curTime.Add(-ttl).UnixNano()

	// Filter out tickets that are fetched but not assigned within ttl time (ms).
	idsInPendingReleases, err := redis.Strings(redisConn.Do("ZRANGEBYSCORE", proposedTicketIDs, startTimeInt, endTimeInt))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error getting pending release %v", err)
	}

	curTimeUnix := curTime.UnixNano()
	// fetch only tickets with a score or ttl ahead of or equal to current time
	idsIndexed, err := redis.Strings(redisConn.Do("ZRANGEBYSCORE", allTicketsWithTTL, curTimeUnix, "+inf"))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error getting all indexed ticket ids %v", err)
	}

	r := make(map[string]struct{}, len(idsIndexed))
	for _, id := range idsIndexed {
		r[id] = struct{}{}
	}
	for _, id := range idsInPendingReleases {
		delete(r, id)
	}

	return r, nil
}

// StreamIndexedIDSet returns an iter that streams ticketIds in the configured batch Size
func (rb *redisBackend) StreamIndexedIDSet(ctx context.Context, queryLimit int) (iter.Seq2[map[string]struct{}, error], error) {
	redisConn, err := rb.redisPool.GetContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "GetIndexedIDSetWithTTL, failed to connect to redis: %v", err)
	}

	ttl := getBackfillReleaseTimeout(rb.cfg)
	curTime := time.Now()
	endTimeInt := curTime.Add(time.Hour).UnixNano()
	startTimeInt := curTime.Add(-ttl).UnixNano()

	// Filter out tickets that are fetched but not assigned within ttl time (ms).
	idsInPendingReleases, err := redis.Strings(redisConn.Do("ZRANGEBYSCORE", proposedTicketIDs, startTimeInt, endTimeInt))
	if err != nil {
		handleConnectionClose(&redisConn)
		return nil, status.Errorf(codes.Internal, "error getting pending release %v", err)
	}

	idsInPendingReleasesSet := make(map[string]struct{}, len(idsInPendingReleases))
	for _, id := range idsInPendingReleases {
		idsInPendingReleasesSet[id] = struct{}{}
	}

	return func(yield func(map[string]struct{}, error) bool) {
		defer handleConnectionClose(&redisConn)

		batchSize := getIndexedTicketBatchSize(rb.cfg)

		lastScoreCount := 0
		lastScore := curTime.UnixNano()

		remaining := queryLimit
		if remaining == 0 {
			// for continuous streaming, canellable only if ctx is cancelled
			remaining = math.MaxInt
		}

		for remaining > 0 {
			select {
			case <-ctx.Done():
				yield(nil, ctx.Err())
				return
			default:
			}

			batchLimit := min(batchSize, remaining)

			indexedIds, err := redis.Strings(redisConn.Do(
				"ZRANGEBYSCORE", allTicketsWithTTL, fmt.Sprintf("(%d", lastScore), "+inf",
				"WITHSCORES", "LIMIT", lastScoreCount, batchLimit,
			))
			if err != nil {
				err = status.Errorf(codes.Internal, "ZRANGEBYSCORE failed: %v", err)
				yield(nil, err)
				break
			}

			if len(indexedIds) == 0 {
				yield(nil, nil)
				break
			}

			results := make(map[string]struct{}, len(indexedIds)/2)

			// indexedIds is [id1, score1, id2, score2, ...]
			for i := 0; i < len(indexedIds); i += 2 {
				id := indexedIds[i]

				if _, exists := idsInPendingReleasesSet[id]; !exists {
					results[id] = struct{}{}
				}

				// Track last score, we can just log the errors
				score, err := strconv.ParseInt(indexedIds[i+1], 10, 64)
				if err != nil {
					logger.WithError(err).Error("error parsing ticket score")
				}

				if score > lastScore {
					lastScore = score
					lastScoreCount = 0
				} else if score == lastScore {
					lastScoreCount++
				}
			}

			if !yield(results, nil) {
				break
			}

			remaining -= len(results)
		}
	}, nil
}

// GetIndexedTicketCount retrieves the current ticket count
func (rb *redisBackend) GetIndexedTicketCount(ctx context.Context) (int, error) {
	redisConn, err := rb.redisPool.GetContext(ctx)
	if err != nil {
		return 0, status.Errorf(codes.Unavailable, "GetIndexedTicketCount, failed to connect to redis: %v", err)
	}
	defer handleConnectionClose(&redisConn)

	now := time.Now().UnixNano()
	// fetch only tickets with a score or ttl ahead of or equal to current time
	count, err := redis.Int(redisConn.Do("ZCOUNT", allTicketsWithTTL, now, "+inf"))
	if err != nil {
		err = errors.Wrap(err, "failed to lookup ticket count")
		return 0, status.Errorf(codes.Internal, "%v", err)
	}

	return count, nil
}

// GetTickets returns multiple tickets from storage.  Missing tickets are
// silently ignored.
func (rb *redisBackend) GetTickets(ctx context.Context, ids []string) ([]*pb.Ticket, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	redisConn, err := rb.redisPool.GetContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "GetTickets, failed to connect to redis: %v", err)
	}
	defer handleConnectionClose(&redisConn)

	queryParams := make([]interface{}, len(ids))
	for i, id := range ids {
		queryParams[i] = id
	}

	ticketBytes, err := redis.ByteSlices(redisConn.Do("MGET", queryParams...))
	if err != nil {
		err = errors.Wrapf(err, "failed to lookup tickets %v", ids)
		return nil, status.Errorf(codes.Internal, "%v", err)
	}

	r := make([]*pb.Ticket, 0, len(ids))
	ticketTTL := getTicketReleaseTimeout(rb.cfg)

	for i, b := range ticketBytes {
		// Tickets may be deleted by the time we read it from redis.
		if b != nil {
			t := &pb.Ticket{}
			err = proto.Unmarshal(b, t)
			if err != nil {
				err = errors.Wrapf(err, "failed to unmarshal ticket from redis, key %s", ids[i])
				return nil, status.Errorf(codes.Internal, "%v", err)
			}
			r = append(r, t)

			if t.Assignment == nil {
				// if a ticket is assigned, its given an TTL and automatically de-indexed
				expiredKey, err := rb.checkIfExpired(ctx, t.Id)
				if err != nil {
					err = errors.Wrapf(err, "failed to check ticket expiry, %v", err)
					return nil, status.Errorf(codes.Internal, "%v", err)
				}

				if expiredKey {
					continue
				}

				expiry := time.Now().Add(ticketTTL).UnixNano()
				// update the indexed ticket's ttl. careful this might become slow.
				err = redisConn.Send("ZADD", allTicketsWithTTL, "XX", "CH", expiry, t.Id)
				if err != nil {
					err = errors.Wrapf(err, "failed to update ttl for ticket id: %s", t.Id)
					return nil, status.Errorf(codes.Internal, "%v", err)
				}
			}
		}
	}

	err = redisConn.Flush()
	if err != nil {
		return nil, errors.Wrap(err, "error executing ticket ttl update")
	}

	return r, nil
}

// UpdateAssignments update using the request's specified tickets with assignments.
func (rb *redisBackend) UpdateAssignments(ctx context.Context, req *pb.AssignTicketsRequest) (*pb.AssignTicketsResponse, []*pb.Ticket, error) {
	resp := &pb.AssignTicketsResponse{}
	if len(req.Assignments) == 0 {
		return resp, []*pb.Ticket{}, nil
	}

	redisConn, err := rb.redisPool.GetContext(ctx)
	if err != nil {
		return nil, nil, status.Errorf(codes.Unavailable, "UpdateAssignments, failed to connect to redis: %v", err)
	}
	defer handleConnectionClose(&redisConn)

	idToA := make(map[string]*pb.Assignment)
	ids := make([]string, 0)
	idsI := make([]interface{}, 0)
	for _, a := range req.Assignments {
		if a.Assignment == nil {
			return nil, nil, status.Error(codes.InvalidArgument, "AssignmentGroup.Assignment is required")
		}

		for _, id := range a.TicketIds {
			if _, ok := idToA[id]; ok {
				return nil, nil, status.Errorf(codes.InvalidArgument, "Ticket id %s is assigned multiple times in one assign tickets call", id)
			}

			idToA[id] = a.Assignment
			ids = append(ids, id)
			idsI = append(idsI, id)
		}
	}

	if len(idsI) == 0 {
		return nil, nil, status.Error(codes.InvalidArgument, "AssignmentGroupTicketIds is empty")
	}

	ticketBytes, err := redis.ByteSlices(redisConn.Do("MGET", idsI...))
	if err != nil {
		return nil, nil, err
	}

	tickets := make([]*pb.Ticket, 0, len(ticketBytes))
	for i, ticketByte := range ticketBytes {
		// Tickets may be deleted by the time we read it from redis.
		if ticketByte == nil {
			resp.Failures = append(resp.Failures, &pb.AssignmentFailure{
				TicketId: ids[i],
				Cause:    pb.AssignmentFailure_TICKET_NOT_FOUND,
			})
		} else {
			t := &pb.Ticket{}
			err = proto.Unmarshal(ticketByte, t)
			if err != nil {
				err = errors.Wrapf(err, "failed to unmarshal ticket from redis %s", ids[i])
				return nil, nil, status.Errorf(codes.Internal, "%v", err)
			}
			tickets = append(tickets, t)
		}
	}
	assignmentTimeout := getAssignedDeleteTimeout(rb.cfg) / time.Millisecond
	err = redisConn.Send("MULTI")
	if err != nil {
		return nil, nil, errors.Wrap(err, "error starting redis multi")
	}

	for _, ticket := range tickets {
		ticket.Assignment = idToA[ticket.Id]

		var ticketByte []byte
		ticketByte, err = proto.Marshal(ticket)
		if err != nil {
			return nil, nil, status.Errorf(codes.Internal, "failed to marshal ticket %s", ticket.GetId())
		}

		err = redisConn.Send("SET", ticket.Id, ticketByte, "PX", int64(assignmentTimeout), "XX")
		if err != nil {
			return nil, nil, errors.Wrap(err, "error sending ticket assignment set")
		}
	}

	wasSet, err := redis.Values(redisConn.Do("EXEC"))
	if err != nil {
		return nil, nil, errors.Wrap(err, "error executing assignment set")
	}

	if len(wasSet) != len(tickets) {
		return nil, nil, status.Errorf(codes.Internal, "sent %d tickets to redis, but received %d back", len(tickets), len(wasSet))
	}

	assignedTickets := make([]*pb.Ticket, 0, len(tickets))
	for i, ticket := range tickets {
		v, err := redis.String(wasSet[i], nil)
		if err == redis.ErrNil {
			resp.Failures = append(resp.Failures, &pb.AssignmentFailure{
				TicketId: ticket.Id,
				Cause:    pb.AssignmentFailure_TICKET_NOT_FOUND,
			})
			continue
		}
		if err != nil {
			return nil, nil, errors.Wrap(err, "unexpected error from redis multi set")
		}
		if v != "OK" {
			return nil, nil, status.Errorf(codes.Internal, "unexpected response from redis: %s", v)
		}
		assignedTickets = append(assignedTickets, ticket)
	}

	return resp, assignedTickets, nil
}

// GetAssignments returns the assignment associated with the input ticket id
func (rb *redisBackend) GetAssignments(ctx context.Context, id string, callback func(*pb.Assignment) error) error {
	redisConn, err := rb.redisPool.GetContext(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "GetAssignments, id: %s, failed to connect to redis: %v", id, err)
	}
	defer handleConnectionClose(&redisConn)

	backoffOperation := func() error {
		var ticket *pb.Ticket
		ticket, err = rb.GetTicket(ctx, id)
		if err != nil {
			return backoff.Permanent(err)
		}

		err = callback(ticket.GetAssignment())
		if err != nil {
			return backoff.Permanent(err)
		}

		return status.Error(codes.Unavailable, "listening on assignment updates, waiting for the next backoff")
	}

	err = backoff.Retry(backoffOperation, rb.newConstantBackoffStrategy())
	if err != nil {
		return err
	}
	return nil
}

// AddTicketsToPendingRelease appends new proposed tickets to the proposed sorted set with current timestamp
func (rb *redisBackend) AddTicketsToPendingRelease(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	redisConn, err := rb.redisPool.GetContext(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "AddTicketsToPendingRelease, failed to connect to redis: %v", err)
	}
	defer handleConnectionClose(&redisConn)

	currentTime := time.Now().UnixNano()
	cmds := make([]interface{}, 0, 2*len(ids)+1)
	cmds = append(cmds, proposedTicketIDs)
	for _, id := range ids {
		cmds = append(cmds, currentTime, id)
	}

	_, err = redisConn.Do("ZADD", cmds...)
	if err != nil {
		err = errors.Wrap(err, "failed to append proposed tickets to pending release")
		return status.Error(codes.Internal, err.Error())
	}

	return nil
}

// DeleteTicketsFromPendingRelease deletes tickets from the proposed sorted set
func (rb *redisBackend) DeleteTicketsFromPendingRelease(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	redisConn, err := rb.redisPool.GetContext(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "DeleteTicketsFromPendingRelease, failed to connect to redis: %v", err)
	}
	defer handleConnectionClose(&redisConn)

	cmds := make([]interface{}, 0, len(ids)+1)
	cmds = append(cmds, proposedTicketIDs)
	for _, id := range ids {
		cmds = append(cmds, id)
	}

	_, err = redisConn.Do("ZREM", cmds...)
	if err != nil {
		err = errors.Wrap(err, "failed to delete proposed tickets from pending release")
		return status.Error(codes.Internal, err.Error())
	}

	return nil
}

func (rb *redisBackend) ReleaseAllTickets(ctx context.Context) error {
	redisConn, err := rb.redisPool.GetContext(ctx)
	if err != nil {
		return status.Errorf(codes.Unavailable, "ReleaseAllTickets, failed to connect to redis: %v", err)
	}
	defer handleConnectionClose(&redisConn)

	_, err = redisConn.Do("DEL", proposedTicketIDs)
	return err
}

func (rb *redisBackend) newConstantBackoffStrategy() backoff.BackOff {
	backoffStrat := backoff.NewConstantBackOff(rb.cfg.GetDuration("backoff.initialInterval"))
	return backoff.BackOff(backoffStrat)
}

// GetExpiredTicketIDs gets all ticket IDs which are expired
func (rb *redisBackend) GetExpiredTicketIDs(ctx context.Context) ([]string, error) {
	redisConn, err := rb.redisPool.GetContext(ctx)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "GetExpiredBackfillIDs, failed to connect to redis: %v", err)
	}
	defer handleConnectionClose(&redisConn)

	ticketTTL := getTicketReleaseTimeout(rb.cfg)
	curTime := time.Now()
	endTimeInt := curTime.Add(-ticketTTL).UnixNano() // anything before the now - ttl
	startTimeInt := 0                                // unix epoc start time

	// Filter out ticket IDs that are fetched but not assigned within TTL time (ms).
	expiredTicketIds, err := redis.Strings(redisConn.Do("ZRANGEBYSCORE", allTicketsWithTTL, startTimeInt, endTimeInt))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error getting expired tickets %v", err)
	}

	return expiredTicketIds, nil
}

// DeleteTicketCompletely performs a set of operations to remove the ticket and all related entities.
func (rb *redisBackend) DeleteTicketCompletely(ctx context.Context, id string) error {
	m := rb.NewMutex(id)
	err := m.Lock(ctx)
	if err != nil {
		return err
	}

	defer func() {
		if _, err = m.Unlock(context.Background()); err != nil {
			logger.WithError(err).Error("error on mutex unlock")
		}
	}()

	// 1. deindex ticket
	err = rb.DeindexTicket(ctx, id)
	if err != nil {
		return err
	}

	// log the errors and try to perform as mush actions as possible

	// 2. delete the ticket
	err = rb.DeleteTicket(ctx, id)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"error":       err.Error(),
			"backfill_id": id,
		}).Error("DeleteTicketCompletely - failed to DeleteTicket")
	}

	// 3. delete ticket from pending release state
	err = rb.DeleteTicketsFromPendingRelease(ctx, []string{id})
	if err != nil {
		logger.WithFields(logrus.Fields{
			"error":     err.Error(),
			"ticket_id": id,
		}).Error("DeleteTicketCompletely - failed to DeleteTicketsFromPendingRelease")
	}

	return nil
}

func (rb *redisBackend) cleanupTicketsWorker(ctx context.Context, ticketIDsCh <-chan string, wg *sync.WaitGroup) {
	var err error
	for id := range ticketIDsCh {
		logger.WithFields(logrus.Fields{
			"ticket_id": id,
		}).Info("cleanupTicketsWorker - starting")
		err = rb.DeleteTicketCompletely(ctx, id)
		if err != nil {
			logger.WithFields(logrus.Fields{
				"error":     err.Error(),
				"ticket_id": id,
			}).Error("CleanupTickets")
		}
		wg.Done()
	}
}

func (rb *redisBackend) CleanupTickets(ctx context.Context) error {
	expiredTicketIDs, err := rb.GetExpiredTicketIDs(ctx)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(len(expiredTicketIDs))
	ticketIDsCh := make(chan string, len(expiredTicketIDs))

	for w := 1; w <= 3; w++ {
		go rb.cleanupTicketsWorker(ctx, ticketIDsCh, &wg)
	}

	for _, id := range expiredTicketIDs {
		ticketIDsCh <- id
	}
	close(ticketIDsCh)

	wg.Wait()

	return nil
}

// TODO: add cache the backoff object
// nolint: unused
func (rb *redisBackend) newExponentialBackoffStrategy() backoff.BackOff {
	backoffStrat := backoff.NewExponentialBackOff()
	backoffStrat.InitialInterval = rb.cfg.GetDuration("backoff.initialInterval")
	backoffStrat.RandomizationFactor = rb.cfg.GetFloat64("backoff.randFactor")
	backoffStrat.Multiplier = rb.cfg.GetFloat64("backoff.multiplier")
	backoffStrat.MaxInterval = rb.cfg.GetDuration("backoff.maxInterval")
	backoffStrat.MaxElapsedTime = rb.cfg.GetDuration("backoff.maxElapsedTime")
	return backoff.BackOff(backoffStrat)
}

func getAssignedDeleteTimeout(cfg config.View) time.Duration {
	const (
		name = "assignedDeleteTimeout"
		// Default timeout to delete tickets after assignment. This value
		// will be used if assignedDeleteTimeout is not configured.
		defaultAssignedDeleteTimeout time.Duration = 10 * time.Minute
	)

	if !cfg.IsSet(name) {
		return defaultAssignedDeleteTimeout
	}

	return cfg.GetDuration(name)
}

func getTicketReleaseTimeout(cfg config.View) time.Duration {
	const (
		name = "ticketDeleteTimeout"
		// Default timeout to delete tickets if not assigned or queried
		defaultTicketDeleteTimeout = time.Minute
	)

	if !cfg.IsSet(name) {
		return defaultTicketDeleteTimeout
	}

	return cfg.GetDuration(name)
}

func getIndexedTicketBatchSize(cfg config.View) int {
	const (
		name = "indexedTicketBatchSize"
		// Default batch size to query indexed ticket by if not configured
		defaultIndexedTicketBatchSize = 5000
	)

	if !cfg.IsSet(name) {
		return defaultIndexedTicketBatchSize
	}

	return cfg.GetInt(name)
}

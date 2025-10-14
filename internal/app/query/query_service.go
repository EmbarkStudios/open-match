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

package query

import (
	"context"
	"fmt"
	"hash/crc32"
	"runtime/trace"
	"sync"

	lru "github.com/hashicorp/golang-lru"
	"go.opencensus.io/stats"
	"golang.org/x/sync/singleflight"
	"google.golang.org/protobuf/proto"
	"open-match.dev/open-match/internal/statestore"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"open-match.dev/open-match/internal/config"
	"open-match.dev/open-match/internal/filter"
	"open-match.dev/open-match/pkg/pb"
)

var (
	logger = logrus.WithFields(logrus.Fields{
		"app":       "openmatch",
		"component": "app.query",
	})
)

// queryService API provides utility functions for common MMF functionality such
// as retrieving Tickets from state storage.
type queryService struct {
	cfg               config.View
	tc                *cache
	bc                *cache
	store             statestore.Service
	batchCache        *lru.Cache
	batchSingleFlight singleflight.Group
}

func (s *queryService) QueryTickets(req *pb.QueryTicketsRequest, responseServer pb.QueryService_QueryTicketsServer) error {
	ctx := responseServer.Context()
	pool := req.GetPool()
	if pool == nil {
		return status.Error(codes.InvalidArgument, ".pool is required")
	}

	pf, err := filter.NewPoolFilter(pool)
	if err != nil {
		return err
	}

	var results []*pb.Ticket
	err = s.tc.request(ctx, func(value interface{}) {
		tickets, ok := value.(map[string]*pb.Ticket)
		if !ok {
			logger.Errorf("expecting value type map[string]*pb.Ticket, but got: %T", value)
			return
		}

		for _, ticket := range tickets {
			if pf.In(ticket) {
				results = append(results, ticket)
			}
		}
	})
	if err != nil {
		err = errors.Wrap(err, "QueryTickets: failed to run request")
		return err
	}
	stats.Record(ctx, ticketsPerQuery.M(int64(len(results))))

	pSize := getPageSize(s.cfg)
	for start := 0; start < len(results); start += pSize {
		end := start + pSize
		if end > len(results) {
			end = len(results)
		}

		err := responseServer.Send(&pb.QueryTicketsResponse{
			Tickets: results[start:end],
		})
		if err != nil {
			return err
		}
	}

	return nil
}

const (
	defaultBatchQueryTicketsQueryLimit  = 10_000
	defaultBatchQueryTicketsResultLimit = 1_000
)

func (s *queryService) BatchQueryTickets(ctx context.Context, req *pb.BatchQueryTicketsRequest) (*pb.BatchQueryTicketsResponse, error) {
	queryLimit := defaultBatchQueryTicketsQueryLimit
	if req.QueryLimit > 0 {
		queryLimit = int(req.QueryLimit)
	}

	resultLimit := defaultBatchQueryTicketsResultLimit
	if req.ResultLimit > 0 {
		resultLimit = int(req.ResultLimit)
	}

	// TODO: If the properties of the pools is not stable in order when we create them, we can just go by the pool
	//  names here instead of the full request.
	data, _ := proto.Marshal(req)
	h := crc32.NewIEEE()
	_, _ = h.Write(data)
	singleFlightKey := string(h.Sum(nil))

	v, err, _ := s.batchSingleFlight.Do(singleFlightKey, func() (interface{}, error) {
		r := trace.StartRegion(ctx, "getRandomIndexIDSet")
		ticketIDs, err := s.store.GetRandomIndexedIDSet(ctx, queryLimit)
		if err != nil {
			return nil, fmt.Errorf("failed to get random indexed ids: %w", err)
		}
		r.End()

		r = trace.StartRegion(ctx, "fetchCached")
		tickets := make(map[string]*pb.Ticket, len(ticketIDs))

		var missingIDs []string
		for ticketID := range ticketIDs {
			if t, ok := s.batchCache.Get(ticketID); ok {
				tickets[ticketID] = t.(*pb.Ticket)
				continue
			}
			missingIDs = append(missingIDs, ticketID)
		}
		r.End()

		r = trace.StartRegion(ctx, "fetchMissing")
		if len(missingIDs) > 0 {
			missingTickets, err := s.store.GetTickets(ctx, missingIDs)
			if err != nil {
				return nil, fmt.Errorf("failed to get missing tickets: %w", err)
			}

			for _, ticket := range missingTickets {
				tickets[ticket.Id] = ticket
				s.batchCache.Add(ticket.Id, ticket)
			}
		}
		r.End()

		poolFilters := make(map[string]*filter.PoolFilter, len(req.Pools))
		poolTickets := make(map[string]*pb.BatchQueryTicketsResponse_PoolTickets, len(req.Pools))
		for _, pool := range req.Pools {
			poolTickets[pool.Name] = &pb.BatchQueryTicketsResponse_PoolTickets{}

			pf, err := filter.NewPoolFilter(pool)
			if err != nil {
				return nil, err
			}
			poolFilters[pool.Name] = pf
		}

		r = trace.StartRegion(ctx, "filterTickets")
		wg := &sync.WaitGroup{}
		for _, pool := range req.Pools {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for _, ticket := range tickets {
					if len(poolTickets[pool.Name].TicketIds) >= resultLimit {
						continue
					}

					if poolFilters[pool.Name].In(ticket) {
						poolTickets[pool.Name].TicketIds = append(poolTickets[pool.Name].TicketIds)
					}
				}
			}()
		}
		wg.Wait()
		r.End()

		return &pb.BatchQueryTicketsResponse{
			PoolTickets: poolTickets,
			Tickets:     tickets,
		}, nil
	})
	if err != nil {
		return nil, err
	}

	return v.(*pb.BatchQueryTicketsResponse), nil
}

func (s *queryService) QueryTicketIds(req *pb.QueryTicketIdsRequest, responseServer pb.QueryService_QueryTicketIdsServer) error {
	ctx := responseServer.Context()
	pool := req.GetPool()
	if pool == nil {
		return status.Error(codes.InvalidArgument, ".pool is required")
	}

	pf, err := filter.NewPoolFilter(pool)
	if err != nil {
		return err
	}

	var results []string
	err = s.tc.request(ctx, func(value interface{}) {
		tickets, ok := value.(map[string]*pb.Ticket)
		if !ok {
			logger.Errorf("expecting value type map[string]*pb.Ticket, but got: %T", value)
			return
		}

		for id, ticket := range tickets {
			if pf.In(ticket) {
				results = append(results, id)
			}
		}
	})
	if err != nil {
		err = errors.Wrap(err, "QueryTicketIds: failed to run request")
		return err
	}
	stats.Record(ctx, ticketsPerQuery.M(int64(len(results))))

	pSize := getPageSize(s.cfg)
	for start := 0; start < len(results); start += pSize {
		end := start + pSize
		if end > len(results) {
			end = len(results)
		}

		err := responseServer.Send(&pb.QueryTicketIdsResponse{
			Ids: results[start:end],
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *queryService) QueryBackfills(req *pb.QueryBackfillsRequest, responseServer pb.QueryService_QueryBackfillsServer) error {
	ctx := responseServer.Context()
	pool := req.GetPool()
	if pool == nil {
		return status.Error(codes.InvalidArgument, ".pool is required")
	}

	pf, err := filter.NewPoolFilter(pool)
	if err != nil {
		return err
	}

	var results []*pb.Backfill
	err = s.bc.request(ctx, func(value interface{}) {
		backfills, ok := value.(map[string]*pb.Backfill)
		if !ok {
			logger.Errorf("expecting value type map[string]*pb.Backfill, but got: %T", value)
			return
		}

		for _, backfill := range backfills {
			if pf.In(backfill) {
				results = append(results, backfill)
			}
		}
	})
	if err != nil {
		err = errors.Wrap(err, "QueryBackfills: failed to run request")
		return err
	}
	stats.Record(ctx, backfillsPerQuery.M(int64(len(results))))

	pSize := getPageSize(s.cfg)
	for start := 0; start < len(results); start += pSize {
		end := start + pSize
		if end > len(results) {
			end = len(results)
		}

		err := responseServer.Send(&pb.QueryBackfillsResponse{
			Backfills: results[start:end],
		})
		if err != nil {
			return err
		}
	}

	return nil
}

func getPageSize(cfg config.View) int {
	const (
		name = "queryPageSize"
		// Minimum number of tickets to be returned in a streamed response for QueryTickets. This value
		// will be used if page size is configured lower than the minimum value.
		minPageSize int = 10
		// Default number of tickets to be returned in a streamed response for QueryTickets.  This value
		// will be used if page size is not configured.
		defaultPageSize int = 1000
		// Maximum number of tickets to be returned in a streamed response for QueryTickets. This value
		// will be used if page size is configured higher than the maximum value.
		maxPageSize int = 10000
	)

	if !cfg.IsSet(name) {
		return defaultPageSize
	}

	pSize := cfg.GetInt(name)
	if pSize < minPageSize {
		logger.Infof("page size %v is lower than the minimum limit of %v", pSize, maxPageSize)
		pSize = minPageSize
	}

	if pSize > maxPageSize {
		logger.Infof("page size %v is higher than the maximum limit of %v", pSize, maxPageSize)
		return maxPageSize
	}

	return pSize
}

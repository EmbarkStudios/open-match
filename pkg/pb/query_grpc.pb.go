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

// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v4.24.0
// source: api/query.proto

package pb

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.64.0 or later.
const _ = grpc.SupportPackageIsVersion9

const (
	QueryService_QueryTickets_FullMethodName   = "/openmatch.QueryService/QueryTickets"
	QueryService_QueryTicketIds_FullMethodName = "/openmatch.QueryService/QueryTicketIds"
	QueryService_QueryBackfills_FullMethodName = "/openmatch.QueryService/QueryBackfills"
)

// QueryServiceClient is the client API for QueryService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
//
// The QueryService service implements helper APIs for Match Function to query Tickets from state storage.
type QueryServiceClient interface {
	// QueryTickets gets a list of Tickets that match all Filters of the input Pool.
	//   - If the Pool contains no Filters, QueryTickets will return all Tickets in the state storage.
	//
	// QueryTickets pages the Tickets by `queryPageSize` and stream back responses.
	//   - queryPageSize is default to 1000 if not set, and has a minimum of 10 and maximum of 10000.
	QueryTickets(ctx context.Context, in *QueryTicketsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[QueryTicketsResponse], error)
	// QueryTicketIds gets the list of TicketIDs that meet all the filtering criteria requested by the pool.
	//   - If the Pool contains no Filters, QueryTicketIds will return all TicketIDs in the state storage.
	//
	// QueryTicketIds pages the TicketIDs by `queryPageSize` and stream back responses.
	//   - queryPageSize is default to 1000 if not set, and has a minimum of 10 and maximum of 10000.
	QueryTicketIds(ctx context.Context, in *QueryTicketIdsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[QueryTicketIdsResponse], error)
	// QueryBackfills gets a list of Backfills.
	// BETA FEATURE WARNING:  This call and the associated Request and Response
	// messages are not finalized and still subject to possible change or removal.
	QueryBackfills(ctx context.Context, in *QueryBackfillsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[QueryBackfillsResponse], error)
}

type queryServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewQueryServiceClient(cc grpc.ClientConnInterface) QueryServiceClient {
	return &queryServiceClient{cc}
}

func (c *queryServiceClient) QueryTickets(ctx context.Context, in *QueryTicketsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[QueryTicketsResponse], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &QueryService_ServiceDesc.Streams[0], QueryService_QueryTickets_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[QueryTicketsRequest, QueryTicketsResponse]{ClientStream: stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type QueryService_QueryTicketsClient = grpc.ServerStreamingClient[QueryTicketsResponse]

func (c *queryServiceClient) QueryTicketIds(ctx context.Context, in *QueryTicketIdsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[QueryTicketIdsResponse], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &QueryService_ServiceDesc.Streams[1], QueryService_QueryTicketIds_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[QueryTicketIdsRequest, QueryTicketIdsResponse]{ClientStream: stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type QueryService_QueryTicketIdsClient = grpc.ServerStreamingClient[QueryTicketIdsResponse]

func (c *queryServiceClient) QueryBackfills(ctx context.Context, in *QueryBackfillsRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[QueryBackfillsResponse], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &QueryService_ServiceDesc.Streams[2], QueryService_QueryBackfills_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[QueryBackfillsRequest, QueryBackfillsResponse]{ClientStream: stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type QueryService_QueryBackfillsClient = grpc.ServerStreamingClient[QueryBackfillsResponse]

// QueryServiceServer is the server API for QueryService service.
// All implementations should embed UnimplementedQueryServiceServer
// for forward compatibility.
//
// The QueryService service implements helper APIs for Match Function to query Tickets from state storage.
type QueryServiceServer interface {
	// QueryTickets gets a list of Tickets that match all Filters of the input Pool.
	//   - If the Pool contains no Filters, QueryTickets will return all Tickets in the state storage.
	//
	// QueryTickets pages the Tickets by `queryPageSize` and stream back responses.
	//   - queryPageSize is default to 1000 if not set, and has a minimum of 10 and maximum of 10000.
	QueryTickets(*QueryTicketsRequest, grpc.ServerStreamingServer[QueryTicketsResponse]) error
	// QueryTicketIds gets the list of TicketIDs that meet all the filtering criteria requested by the pool.
	//   - If the Pool contains no Filters, QueryTicketIds will return all TicketIDs in the state storage.
	//
	// QueryTicketIds pages the TicketIDs by `queryPageSize` and stream back responses.
	//   - queryPageSize is default to 1000 if not set, and has a minimum of 10 and maximum of 10000.
	QueryTicketIds(*QueryTicketIdsRequest, grpc.ServerStreamingServer[QueryTicketIdsResponse]) error
	// QueryBackfills gets a list of Backfills.
	// BETA FEATURE WARNING:  This call and the associated Request and Response
	// messages are not finalized and still subject to possible change or removal.
	QueryBackfills(*QueryBackfillsRequest, grpc.ServerStreamingServer[QueryBackfillsResponse]) error
}

// UnimplementedQueryServiceServer should be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedQueryServiceServer struct{}

func (UnimplementedQueryServiceServer) QueryTickets(*QueryTicketsRequest, grpc.ServerStreamingServer[QueryTicketsResponse]) error {
	return status.Errorf(codes.Unimplemented, "method QueryTickets not implemented")
}
func (UnimplementedQueryServiceServer) QueryTicketIds(*QueryTicketIdsRequest, grpc.ServerStreamingServer[QueryTicketIdsResponse]) error {
	return status.Errorf(codes.Unimplemented, "method QueryTicketIds not implemented")
}
func (UnimplementedQueryServiceServer) QueryBackfills(*QueryBackfillsRequest, grpc.ServerStreamingServer[QueryBackfillsResponse]) error {
	return status.Errorf(codes.Unimplemented, "method QueryBackfills not implemented")
}
func (UnimplementedQueryServiceServer) testEmbeddedByValue() {}

// UnsafeQueryServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to QueryServiceServer will
// result in compilation errors.
type UnsafeQueryServiceServer interface {
	mustEmbedUnimplementedQueryServiceServer()
}

func RegisterQueryServiceServer(s grpc.ServiceRegistrar, srv QueryServiceServer) {
	// If the following call pancis, it indicates UnimplementedQueryServiceServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&QueryService_ServiceDesc, srv)
}

func _QueryService_QueryTickets_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(QueryTicketsRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(QueryServiceServer).QueryTickets(m, &grpc.GenericServerStream[QueryTicketsRequest, QueryTicketsResponse]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type QueryService_QueryTicketsServer = grpc.ServerStreamingServer[QueryTicketsResponse]

func _QueryService_QueryTicketIds_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(QueryTicketIdsRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(QueryServiceServer).QueryTicketIds(m, &grpc.GenericServerStream[QueryTicketIdsRequest, QueryTicketIdsResponse]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type QueryService_QueryTicketIdsServer = grpc.ServerStreamingServer[QueryTicketIdsResponse]

func _QueryService_QueryBackfills_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(QueryBackfillsRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(QueryServiceServer).QueryBackfills(m, &grpc.GenericServerStream[QueryBackfillsRequest, QueryBackfillsResponse]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type QueryService_QueryBackfillsServer = grpc.ServerStreamingServer[QueryBackfillsResponse]

// QueryService_ServiceDesc is the grpc.ServiceDesc for QueryService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var QueryService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "openmatch.QueryService",
	HandlerType: (*QueryServiceServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "QueryTickets",
			Handler:       _QueryService_QueryTickets_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "QueryTicketIds",
			Handler:       _QueryService_QueryTicketIds_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "QueryBackfills",
			Handler:       _QueryService_QueryBackfills_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "api/query.proto",
}

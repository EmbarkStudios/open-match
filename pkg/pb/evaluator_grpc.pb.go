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
// source: api/evaluator.proto

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
	Evaluator_Evaluate_FullMethodName = "/openmatch.Evaluator/Evaluate"
)

// EvaluatorClient is the client API for Evaluator service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
//
// The Evaluator service implements APIs used to evaluate and shortlist matches proposed by MMFs.
type EvaluatorClient interface {
	// Evaluate evaluates a list of proposed matches based on quality, collision status, and etc, then shortlist the matches and returns the final results.
	Evaluate(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[EvaluateRequest, EvaluateResponse], error)
}

type evaluatorClient struct {
	cc grpc.ClientConnInterface
}

func NewEvaluatorClient(cc grpc.ClientConnInterface) EvaluatorClient {
	return &evaluatorClient{cc}
}

func (c *evaluatorClient) Evaluate(ctx context.Context, opts ...grpc.CallOption) (grpc.BidiStreamingClient[EvaluateRequest, EvaluateResponse], error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	stream, err := c.cc.NewStream(ctx, &Evaluator_ServiceDesc.Streams[0], Evaluator_Evaluate_FullMethodName, cOpts...)
	if err != nil {
		return nil, err
	}
	x := &grpc.GenericClientStream[EvaluateRequest, EvaluateResponse]{ClientStream: stream}
	return x, nil
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type Evaluator_EvaluateClient = grpc.BidiStreamingClient[EvaluateRequest, EvaluateResponse]

// EvaluatorServer is the server API for Evaluator service.
// All implementations should embed UnimplementedEvaluatorServer
// for forward compatibility.
//
// The Evaluator service implements APIs used to evaluate and shortlist matches proposed by MMFs.
type EvaluatorServer interface {
	// Evaluate evaluates a list of proposed matches based on quality, collision status, and etc, then shortlist the matches and returns the final results.
	Evaluate(grpc.BidiStreamingServer[EvaluateRequest, EvaluateResponse]) error
}

// UnimplementedEvaluatorServer should be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedEvaluatorServer struct{}

func (UnimplementedEvaluatorServer) Evaluate(grpc.BidiStreamingServer[EvaluateRequest, EvaluateResponse]) error {
	return status.Errorf(codes.Unimplemented, "method Evaluate not implemented")
}
func (UnimplementedEvaluatorServer) testEmbeddedByValue() {}

// UnsafeEvaluatorServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to EvaluatorServer will
// result in compilation errors.
type UnsafeEvaluatorServer interface {
	mustEmbedUnimplementedEvaluatorServer()
}

func RegisterEvaluatorServer(s grpc.ServiceRegistrar, srv EvaluatorServer) {
	// If the following call pancis, it indicates UnimplementedEvaluatorServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&Evaluator_ServiceDesc, srv)
}

func _Evaluator_Evaluate_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(EvaluatorServer).Evaluate(&grpc.GenericServerStream[EvaluateRequest, EvaluateResponse]{ServerStream: stream})
}

// This type alias is provided for backwards compatibility with existing code that references the prior non-generic stream type by name.
type Evaluator_EvaluateServer = grpc.BidiStreamingServer[EvaluateRequest, EvaluateResponse]

// Evaluator_ServiceDesc is the grpc.ServiceDesc for Evaluator service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Evaluator_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "openmatch.Evaluator",
	HandlerType: (*EvaluatorServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "Evaluate",
			Handler:       _Evaluator_Evaluate_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "api/evaluator.proto",
}

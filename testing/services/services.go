// Copyright 2025 Google LLC
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

// Package services provides test helpers for starting all Open Match services
// in-process using an in-memory Redis backend. This is intended for use in
// integration tests that need real Open Match behaviour without Docker or
// Kubernetes.
//
// All four core services (frontend, backend, query, synchronizer) are started
// on a single gRPC/HTTP server via the minimatch pattern, so they share one
// TCP port. The evaluator and match function are real external gRPC servers
// started by the caller; their addresses are injected via evaluatorAddr and
// per-request FunctionConfig respectively.
package services

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Bose/minisentinel"
	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"open-match.dev/open-match/internal/app/minimatch"
	"open-match.dev/open-match/internal/appmain/apptest"
	"open-match.dev/open-match/internal/rpc"
	"open-match.dev/open-match/pkg/pb"
)

// Server holds the addresses and pre-created clients for the in-process
// Open Match services started by Start.
type Server struct {
	// GRPCAddr is the "host:port" address for all Open Match gRPC services
	// (frontend, backend, query, synchronizer).
	GRPCAddr string
	// HTTPAddr is the "host:port" address for all Open Match HTTP services.
	HTTPAddr string

	RedisAddr string

	// AdvanceTTLTime fast-forwards the in-memory Redis clock by the given
	// duration. Use this in tests that exercise ticket or backfill TTLs to
	// avoid real sleeps.
	AdvanceTTLTime func(time.Duration)

	// Pre-created gRPC clients. The underlying connection is closed
	// automatically when the test ends via t.Cleanup.
	Frontend pb.FrontendServiceClient
	Backend  pb.BackendServiceClient
	Query    pb.QueryServiceClient
}

// Start starts all four Open Match services (frontend, backend, query,
// synchronizer) in-process on random TCP ports using an in-memory Redis.
//
// evaluatorAddr is the "host:port" of the caller's evaluator gRPC server,
// which must already be listening before Start is called. The synchronizer
// will call this address during the match cycle. Pass "" if the evaluator
// will not be exercised by the test.
//
// The match function address is not configured here; pass it per-request
// inside FetchMatchesRequest.Config (a FunctionConfig with Host/Port/Type).
//
// All Open Match services stop automatically when the test ends via t.Cleanup.
func Start(t *testing.T, evaluatorAddr string) *Server {
	t.Helper()

	// In-memory Redis with Sentinel for Open Match state store.
	mredis := miniredis.NewMiniRedis()
	if err := mredis.StartAddr("localhost:0"); err != nil {
		t.Fatalf("services.Start: failed to start miniredis: %v", err)
	}
	t.Cleanup(mredis.Close)

	msentinel := minisentinel.NewSentinel(mredis)
	if err := msentinel.StartAddr("localhost:0"); err != nil {
		t.Fatalf("services.Start: failed to start minisentinel: %v", err)
	}
	t.Cleanup(msentinel.Close)

	// Create one gRPC listener and one HTTP listener; all services share them.
	grpcLis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("services.Start: failed to create gRPC listener: %v", err)
	}
	httpLis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("services.Start: failed to create HTTP listener: %v", err)
	}

	_, grpcPort, _ := net.SplitHostPort(grpcLis.Addr().String())
	_, httpPort, _ := net.SplitHostPort(httpLis.Addr().String())

	// Build Viper config: load defaults then override with in-memory addresses.
	cfg := viper.New()
	cfg.SetConfigType("yaml")
	if err := cfg.ReadConfig(strings.NewReader(defaultConfig)); err != nil {
		t.Fatalf("services.Start: failed to read default config: %v", err)
	}

	cfg.Set("redis.sentinelHostname", msentinel.Host())
	cfg.Set("redis.sentinelPort", msentinel.Port())
	cfg.Set("redis.sentinelMaster", msentinel.MasterInfo().Name)

	for _, name := range []string{apptest.ServiceName, "synchronizer", "backend", "frontend", "query"} {
		cfg.Set("api."+name+".hostname", "localhost")
		cfg.Set("api."+name+".grpcport", grpcPort)
		cfg.Set("api."+name+".httpport", httpPort)
	}

	// Point the synchronizer at the caller's external evaluator, if provided.
	if evaluatorAddr != "" {
		evalHost, evalPort, err := net.SplitHostPort(evaluatorAddr)
		if err != nil {
			t.Fatalf("services.Start: invalid evaluatorAddr %q: %v", evaluatorAddr, err)
		}
		cfg.Set("api.evaluator.hostname", evalHost)
		cfg.Set("api.evaluator.grpcport", evalPort)
	}

	cfg.Set(rpc.ConfigNameEnableRPCLogging, false)

	apptest.TestApp(t, cfg, []net.Listener{grpcLis, httpLis}, minimatch.BindService)

	// Dial a single shared connection; all three clients reuse it.
	conn, err := grpc.NewClient("localhost:"+grpcPort,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("services.Start: failed to dial Open Match gRPC: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return &Server{
		GRPCAddr:       "localhost:" + grpcPort,
		HTTPAddr:       "localhost:" + httpPort,
		AdvanceTTLTime: mredis.FastForward,
		Frontend:       pb.NewFrontendServiceClient(conn),
		Backend:        pb.NewBackendServiceClient(conn),
		Query:          pb.NewQueryServiceClient(conn),
		RedisAddr:      net.JoinHostPort(msentinel.Host(), msentinel.Port()),
	}
}

// defaultConfig is the minimal Open Match configuration for in-process tests.
// Adapted from testing/e2e/common.go.
const defaultConfig = `
registrationInterval: 200ms
proposalCollectionInterval: 200ms
pendingReleaseTimeout: 1s
assignedDeleteTimeout: 200ms
queryPageSize: 10
backfillLockTimeout: 1m

logging:
  level: warn
  format: text
  rpc: false

backoff:
  initialInterval: 100ms
  maxInterval: 500ms
  multiplier: 1.5
  randFactor: 0.5
  maxElapsedTime: 3000ms

api:
  backend:
    hostname: "open-match-backend"
    grpcport: "50505"
    httpport: "51505"
  frontend:
    hostname: "open-match-frontend"
    grpcport: "50504"
    httpport: "51504"
  query:
    hostname: "open-match-query"
    grpcport: "50503"
    httpport: "51503"
  synchronizer:
    hostname: "open-match-synchronizer"
    grpcport: "50506"
    httpport: "51506"
  evaluator:
    hostname: "open-match-test"
    grpcport: "50509"
    httpport: "51509"
  test:
    hostname: "open-match-test"
    grpcport: "50509"
    httpport: "51509"

redis:
  port: 6379
  usePassword: false
  passwordPath: /redis-password
  pool:
    maxIdle: 200
    maxActive: 0
    idleTimeout: 0
    healthCheckTimeout: 300ms

telemetry:
  reportingPeriod: "1m"
  traceSamplingFraction: "0.01"
  zpages:
    enable: "false"
  prometheus:
    enable: "false"
    endpoint: "/metrics"
    serviceDiscovery: "false"
  stackdriverMetrics:
    enable: "false"
    gcpProjectId: "intentionally-invalid-value"
    prefix: "open_match"
`

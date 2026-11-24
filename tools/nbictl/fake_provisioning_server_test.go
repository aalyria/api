// Copyright (c) Aalyria Technologies, Inc., and its affiliates.
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

package nbictl

import (
	"context"
	"net"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	provapipb "aalyria.com/spacetime/api/provisioning/v1alpha"
)

// fakeProvisioningServer stores geographic regions and emissions limits in
// memory. Like provisioningfe, it rejects an emissions limit whose
// geographic regions don't exist. Every other RPC is unimplemented, which
// sync treats as an empty remote list.
type fakeProvisioningServer struct {
	provapipb.UnimplementedProvisioningServer

	// writeDelay is how long each write of a geographic region takes, so an
	// emissions limit sent in parallel arrives before the region commits.
	writeDelay time.Duration

	mu        sync.Mutex
	regions   map[string]*provapipb.GeographicRegion
	limits    map[string]*provapipb.EmissionsLimit
	deleteLog []string
}

func newFakeProvisioningServer() *fakeProvisioningServer {
	return &fakeProvisioningServer{
		regions: map[string]*provapipb.GeographicRegion{},
		limits:  map[string]*provapipb.EmissionsLimit{},
	}
}

func startFakeProvisioningServer(ctx context.Context, g *errgroup.Group, lis net.Listener, srv *fakeProvisioningServer) {
	server := grpc.NewServer()
	provapipb.RegisterProvisioningServer(server, srv)
	g.Go(func() error { return server.Serve(lis) })
	g.Go(func() error {
		<-ctx.Done()
		server.GracefulStop()
		return nil
	})
}

func (s *fakeProvisioningServer) UpdateGeographicRegion(_ context.Context, req *provapipb.UpdateGeographicRegionRequest) (*provapipb.GeographicRegion, error) {
	time.Sleep(s.writeDelay)
	s.mu.Lock()
	defer s.mu.Unlock()
	region := req.GetGeographicRegion()
	s.regions[region.GetName()] = proto.CloneOf(region)
	return region, nil
}

func (s *fakeProvisioningServer) UpdateEmissionsLimit(_ context.Context, req *provapipb.UpdateEmissionsLimitRequest) (*provapipb.EmissionsLimit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	limit := req.GetEmissionsLimit()
	for _, region := range limit.GetGeographicRegions() {
		if _, ok := s.regions[region]; !ok {
			return nil, status.Errorf(codes.InvalidArgument, "no such geographic region: %q", region)
		}
	}
	s.limits[limit.GetName()] = proto.CloneOf(limit)
	return limit, nil
}

func (s *fakeProvisioningServer) ListGeographicRegions(context.Context, *provapipb.ListGeographicRegionsRequest) (*provapipb.ListGeographicRegionsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &provapipb.ListGeographicRegionsResponse{GeographicRegions: mapValues(s.regions)}, nil
}

func (s *fakeProvisioningServer) ListEmissionsLimits(context.Context, *provapipb.ListEmissionsLimitsRequest) (*provapipb.ListEmissionsLimitsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &provapipb.ListEmissionsLimitsResponse{EmissionsLimits: mapValues(s.limits)}, nil
}

func (s *fakeProvisioningServer) DeleteEmissionsLimit(_ context.Context, req *provapipb.DeleteEmissionsLimitRequest) (*emptypb.Empty, error) {
	return deleteFrom(s, s.limits, req.GetName())
}

func (s *fakeProvisioningServer) DeleteGeographicRegion(_ context.Context, req *provapipb.DeleteGeographicRegionRequest) (*emptypb.Empty, error) {
	return deleteFrom(s, s.regions, req.GetName())
}

// deleteFrom removes name from m and records the delete.
func deleteFrom[T any](s *fakeProvisioningServer, m map[string]T, name string) (*emptypb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := m[name]; !ok {
		return nil, status.Errorf(codes.NotFound, "no such resource %q", name)
	}
	delete(m, name)
	s.deleteLog = append(s.deleteLog, name)
	return &emptypb.Empty{}, nil
}

func mapValues[T proto.Message](m map[string]T) []T {
	values := make([]T, 0, len(m))
	for _, v := range m {
		values = append(values, proto.CloneOf(v))
	}
	return values
}

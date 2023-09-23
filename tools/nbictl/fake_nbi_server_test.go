// Copyright 2023 Aalyria Technologies, Inc., and its affiliates.
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
	"sync/atomic"

	nbi "aalyria.com/spacetime/api/nbi/v1alpha"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
)

type FakeNetOpsServer struct {
	listener net.Listener

	nbi.UnimplementedNetOpsServer
	IncomingMetadata     []metadata.MD
	NumCallsListEntities *atomic.Int64
	ListEntityResponse   *nbi.ListEntitiesResponse
}

func (s *FakeNetOpsServer) ListEntities(ctx context.Context, req *nbi.ListEntitiesRequest) (*nbi.ListEntitiesResponse, error) {
	md := make(metadata.MD)
	md, _ = metadata.FromIncomingContext(ctx)
	s.IncomingMetadata = append(s.IncomingMetadata, md)
	s.NumCallsListEntities.Add(1)
	return s.ListEntityResponse, nil
}

func startFakeNbiServer(ctx context.Context, g *errgroup.Group, listener net.Listener) (*FakeNetOpsServer, error) {
	fakeNbiServer := &FakeNetOpsServer{
		IncomingMetadata:     make([]metadata.MD, 0),
		NumCallsListEntities: &atomic.Int64{},
		listener:             listener,
		ListEntityResponse:   &nbi.ListEntitiesResponse{},
	}
	server := grpc.NewServer()
	nbi.RegisterNetOpsServer(server, fakeNbiServer)
	reflection.Register(server)

	g.Go(func() error {
		return server.Serve(listener)
	})

	g.Go(func() error {
		<-ctx.Done()
		server.GracefulStop()
		listener.Close()
		return nil
	})

	return fakeNbiServer, nil
}

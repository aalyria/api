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
	"sync"
	"sync/atomic"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/proto"

	nbi "aalyria.com/spacetime/api/nbi/v1alpha"
)

const DEFAULT_COMMIT_TIMESTAMP = int64(123456)

type FakeNetOpsServer struct {
	listener net.Listener

	nbi.UnimplementedNetOpsServer
	IncomingMetadata     []metadata.MD
	NumCallsListEntities *atomic.Int64
	ListEntityResponse   *nbi.ListEntitiesResponse

	EntityIDsModified map[string]struct{}
	// Synchronizes access to EntityIDsModified.
	mu sync.Mutex
}

func (s *FakeNetOpsServer) ListEntities(ctx context.Context, req *nbi.ListEntitiesRequest) (*nbi.ListEntitiesResponse, error) {
	md := make(metadata.MD)
	md, _ = metadata.FromIncomingContext(ctx)
	s.IncomingMetadata = append(s.IncomingMetadata, md)
	s.NumCallsListEntities.Add(1)
	return s.ListEntityResponse, nil
}

// Returns the Entity in the request, with the default commit timestamp.
// Assumes that the ID has been set in the Entity within the CreateEntityRequest.
func (s *FakeNetOpsServer) CreateEntity(ctx context.Context, req *nbi.CreateEntityRequest) (*nbi.Entity, error) {
	md := make(metadata.MD)
	md, _ = metadata.FromIncomingContext(ctx)
	s.IncomingMetadata = append(s.IncomingMetadata, md)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.EntityIDsModified[req.GetEntity().GetId()] = struct{}{}

	res := *req.GetEntity()
	res.CommitTimestamp = proto.Int64(DEFAULT_COMMIT_TIMESTAMP)
	return &res, nil
}

// Returns an Entity with the same type and ID as in the request,
// along with the default commit timestamp.
// This method does not increment EntityIDsModified.
func (s *FakeNetOpsServer) GetEntity(ctx context.Context, req *nbi.GetEntityRequest) (*nbi.Entity, error) {
	md := make(metadata.MD)
	md, _ = metadata.FromIncomingContext(ctx)
	s.IncomingMetadata = append(s.IncomingMetadata, md)

	res := &nbi.Entity{
		Id: req.Id,
		Group: &nbi.EntityGroup{
			Type: req.Type,
		},
		CommitTimestamp: proto.Int64(DEFAULT_COMMIT_TIMESTAMP),
	}
	return res, nil
}

// Returns the Entity in the request, with the default commit timestamp.
func (s *FakeNetOpsServer) UpdateEntity(ctx context.Context, req *nbi.UpdateEntityRequest) (*nbi.Entity, error) {
	md := make(metadata.MD)
	md, _ = metadata.FromIncomingContext(ctx)
	s.IncomingMetadata = append(s.IncomingMetadata, md)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.EntityIDsModified[req.GetEntity().GetId()] = struct{}{}

	res := *req.GetEntity()
	res.CommitTimestamp = proto.Int64(DEFAULT_COMMIT_TIMESTAMP)
	return &res, nil
}

// Returns a DeleteEntityResponse regardless of the request.
func (s *FakeNetOpsServer) DeleteEntity(ctx context.Context, req *nbi.DeleteEntityRequest) (*nbi.DeleteEntityResponse, error) {
	md := make(metadata.MD)
	md, _ = metadata.FromIncomingContext(ctx)
	s.IncomingMetadata = append(s.IncomingMetadata, md)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.EntityIDsModified[req.GetId()] = struct{}{}

	return &nbi.DeleteEntityResponse{}, nil
}

func startFakeNbiServer(ctx context.Context, g *errgroup.Group, listener net.Listener) (*FakeNetOpsServer, error) {
	fakeNbiServer := &FakeNetOpsServer{
		IncomingMetadata:     make([]metadata.MD, 0),
		NumCallsListEntities: &atomic.Int64{},
		EntityIDsModified:    make(map[string]struct{}, 0),
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

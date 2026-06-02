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
	"fmt"
	"net"
	"sync"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	modelpb "aalyria.com/spacetime/api/model/v1"
	nmtspb "outernetcouncil.org/nmts/v1/proto"
)

type relationshipKey struct {
	A    string
	Z    string
	Kind nmtspb.RK
}

type InMemoryModelServer struct {
	modelpb.UnimplementedModelServer

	mu            sync.RWMutex
	entities      map[string]*nmtspb.Entity
	relationships map[relationshipKey]*nmtspb.Relationship
	listener      net.Listener
}

func newInMemoryModelServer() *InMemoryModelServer {
	return &InMemoryModelServer{
		entities:      make(map[string]*nmtspb.Entity),
		relationships: make(map[relationshipKey]*nmtspb.Relationship),
	}
}

func (s *InMemoryModelServer) Seed(entities []*nmtspb.Entity, relationships []*nmtspb.Relationship) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, e := range entities {
		s.entities[e.GetId()] = proto.Clone(e).(*nmtspb.Entity)
	}
	for _, r := range relationships {
		key := relationshipKey{A: r.GetA(), Z: r.GetZ(), Kind: r.GetKind()}
		s.relationships[key] = proto.Clone(r).(*nmtspb.Relationship)
	}
}

func (s *InMemoryModelServer) EntityCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entities)
}

func (s *InMemoryModelServer) RelationshipCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.relationships)
}

func (s *InMemoryModelServer) CreateEntity(_ context.Context, req *modelpb.CreateEntityRequest) (*nmtspb.Entity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entity := req.GetEntity()
	id := entity.GetId()
	if id == "" {
		return nil, status.Errorf(codes.InvalidArgument, "entity id is required")
	}
	if _, exists := s.entities[id]; exists {
		return nil, status.Errorf(codes.AlreadyExists, "entity %q already exists", id)
	}

	stored := proto.Clone(entity).(*nmtspb.Entity)
	s.entities[id] = stored
	return proto.Clone(stored).(*nmtspb.Entity), nil
}

func (s *InMemoryModelServer) UpdateEntity(_ context.Context, req *modelpb.UpdateEntityRequest) (*nmtspb.Entity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entity := req.GetEntity()
	id := entity.GetId()
	if id == "" {
		return nil, status.Errorf(codes.InvalidArgument, "entity id is required")
	}

	if _, exists := s.entities[id]; !exists {
		if !req.GetAllowMissing() {
			return nil, status.Errorf(codes.NotFound, "entity %q not found", id)
		}
	}

	stored := proto.Clone(entity).(*nmtspb.Entity)
	s.entities[id] = stored
	return proto.Clone(stored).(*nmtspb.Entity), nil
}

func (s *InMemoryModelServer) DeleteEntity(_ context.Context, req *modelpb.DeleteEntityRequest) (*modelpb.DeleteEntityResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := req.GetEntityId()
	if _, exists := s.entities[id]; !exists {
		return nil, status.Errorf(codes.NotFound, "entity %q not found", id)
	}

	delete(s.entities, id)

	var deletedRels []*nmtspb.Relationship
	for key, rel := range s.relationships {
		if key.A == id || key.Z == id {
			deletedRels = append(deletedRels, proto.Clone(rel).(*nmtspb.Relationship))
			delete(s.relationships, key)
		}
	}

	return &modelpb.DeleteEntityResponse{
		DeletedRelationships: deletedRels,
	}, nil
}

func (s *InMemoryModelServer) CreateRelationship(_ context.Context, req *modelpb.CreateRelationshipRequest) (*nmtspb.Relationship, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rel := req.GetRelationship()
	key := relationshipKey{A: rel.GetA(), Z: rel.GetZ(), Kind: rel.GetKind()}

	stored := proto.Clone(rel).(*nmtspb.Relationship)
	s.relationships[key] = stored
	return proto.Clone(stored).(*nmtspb.Relationship), nil
}

func (s *InMemoryModelServer) DeleteRelationship(_ context.Context, req *modelpb.DeleteRelationshipRequest) (*emptypb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rel := req.GetRelationship()
	key := relationshipKey{A: rel.GetA(), Z: rel.GetZ(), Kind: rel.GetKind()}

	if _, exists := s.relationships[key]; !exists {
		return nil, status.Errorf(codes.NotFound, "relationship not found")
	}

	delete(s.relationships, key)
	return &emptypb.Empty{}, nil
}

func (s *InMemoryModelServer) GetEntity(_ context.Context, req *modelpb.GetEntityRequest) (*nmtspb.Entity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id := req.GetEntityId()
	entity, exists := s.entities[id]
	if !exists {
		return nil, status.Errorf(codes.NotFound, "entity %q not found", id)
	}

	return proto.Clone(entity).(*nmtspb.Entity), nil
}

func (s *InMemoryModelServer) ListEntities(_ context.Context, _ *modelpb.ListEntitiesRequest) (*modelpb.ListEntitiesResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entities := make([]*nmtspb.Entity, 0, len(s.entities))
	for _, e := range s.entities {
		entities = append(entities, proto.Clone(e).(*nmtspb.Entity))
	}

	return &modelpb.ListEntitiesResponse{Entities: entities}, nil
}

func (s *InMemoryModelServer) ListRelationships(_ context.Context, _ *modelpb.ListRelationshipsRequest) (*modelpb.ListRelationshipsResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rels := make([]*nmtspb.Relationship, 0, len(s.relationships))
	for _, r := range s.relationships {
		rels = append(rels, proto.Clone(r).(*nmtspb.Relationship))
	}

	return &modelpb.ListRelationshipsResponse{Relationships: rels}, nil
}

func startInMemoryModelServer(ctx context.Context, g *errgroup.Group, listener net.Listener) (*InMemoryModelServer, error) {
	srv := newInMemoryModelServer()
	srv.listener = listener

	server := grpc.NewServer()
	modelpb.RegisterModelServer(server, srv)
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

	return srv, nil
}

func generateSyntheticModel(numEntities int) ([]*nmtspb.Entity, []*nmtspb.Relationship) {
	entities := make([]*nmtspb.Entity, numEntities)
	for i := range numEntities {
		entities[i] = &nmtspb.Entity{
			Id: fmt.Sprintf("entity-%04d", i),
		}
	}

	var relationships []*nmtspb.Relationship
	if numEntities > 1 {
		relationships = make([]*nmtspb.Relationship, numEntities-1)
		for i := 1; i < numEntities; i++ {
			relationships[i-1] = &nmtspb.Relationship{
				A:    entities[0].GetId(),
				Z:    entities[i].GetId(),
				Kind: nmtspb.RK_RK_CONTAINS,
			}
		}
	}

	return entities, relationships
}

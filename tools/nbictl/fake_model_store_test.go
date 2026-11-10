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
	"cmp"
	"context"
	"fmt"
	"maps"
	"net"
	"slices"
	"strconv"
	"sync"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	"outernetcouncil.org/nmts/v1/lib/validation"
	nmtspb "outernetcouncil.org/nmts/v1/proto"
	nmtsphypb "outernetcouncil.org/nmts/v1/proto/ek/physical"

	modelpb "aalyria.com/spacetime/api/model/v1"
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
	// upsertBatches records the entity and relationship counts of each
	// accepted UpsertFragment request, in the order they were applied.
	upsertBatches []upsertBatch

	// maxPageSize, when positive, caps the page the List RPCs serve.
	maxPageSize             int
	entityPagesServed       int
	relationshipPagesServed int
	// failListAfterPages, when positive, makes each List RPC fail once it has
	// served that many pages.
	failListAfterPages int
}

// fakeDefaultPageSize matches the page the real ModelFE serves when a request
// names no page size.
const fakeDefaultPageSize = 1000

type upsertBatch struct {
	entities, relationships int
}

// UpsertBatches returns the counts of each accepted UpsertFragment request, in
// the order they were applied.
func (s *InMemoryModelServer) UpsertBatches() []upsertBatch {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.upsertBatches)
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

// SetMaxPageSize sets the largest page the List RPCs serve, clamping any larger
// page_size down to it the way the real ModelFE does.
func (s *InMemoryModelServer) SetMaxPageSize(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxPageSize = n
}

// FailListAfterPages makes each List RPC fail with a non-retryable error once
// it has served n pages.
func (s *InMemoryModelServer) FailListAfterPages(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failListAfterPages = n
}

// ListPageCounts returns how many ListEntities and ListRelationships requests
// have been served.
func (s *InMemoryModelServer) ListPageCounts() (entityPages, relationshipPages int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.entityPagesServed, s.relationshipPagesServed
}

// pageBounds resolves a page_size and page_token against total elements in a
// stable order, the way the real ModelFE does: a negative size is rejected, an
// unset one gets the default, and a size over the maximum is clamped. The token
// is the offset the page starts at, which is enough for a fake that is not
// mutated while it is being listed.
func (s *InMemoryModelServer) pageBounds(pageSize int32, pageToken string, total int) (start, end int, nextToken string, err error) {
	size := int(pageSize)
	switch {
	case size < 0:
		return 0, 0, "", status.Errorf(codes.InvalidArgument, "page_size must not be negative, got %d", size)
	case size == 0:
		size = fakeDefaultPageSize
	}
	if s.maxPageSize > 0 {
		size = min(size, s.maxPageSize)
	}

	if pageToken != "" {
		if start, err = strconv.Atoi(pageToken); err != nil || start < 0 || start > total {
			return 0, 0, "", status.Errorf(codes.InvalidArgument, "page_token %q was not issued by this service", pageToken)
		}
	}
	end = min(start+size, total)
	if end < total {
		nextToken = strconv.Itoa(end)
	}
	return start, end, nextToken, nil
}

func (s *InMemoryModelServer) ListEntities(_ context.Context, req *modelpb.ListEntitiesRequest) (*modelpb.ListEntitiesResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.failListAfterPages > 0 && s.entityPagesServed >= s.failListAfterPages {
		return nil, status.Errorf(codes.Internal, "injected failure after %d pages", s.entityPagesServed)
	}
	ids := slices.Sorted(maps.Keys(s.entities))
	start, end, nextToken, err := s.pageBounds(req.GetPageSize(), req.GetPageToken(), len(ids))
	if err != nil {
		return nil, err
	}
	s.entityPagesServed++

	entities := make([]*nmtspb.Entity, 0, end-start)
	for _, id := range ids[start:end] {
		entities = append(entities, proto.Clone(s.entities[id]).(*nmtspb.Entity))
	}

	return &modelpb.ListEntitiesResponse{Entities: entities, NextPageToken: nextToken}, nil
}

func (s *InMemoryModelServer) ListRelationships(_ context.Context, req *modelpb.ListRelationshipsRequest) (*modelpb.ListRelationshipsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.failListAfterPages > 0 && s.relationshipPagesServed >= s.failListAfterPages {
		return nil, status.Errorf(codes.Internal, "injected failure after %d pages", s.relationshipPagesServed)
	}
	keys := slices.SortedFunc(maps.Keys(s.relationships), func(a, b relationshipKey) int {
		return cmp.Or(cmp.Compare(a.A, b.A), cmp.Compare(a.Z, b.Z), cmp.Compare(a.Kind, b.Kind))
	})
	start, end, nextToken, err := s.pageBounds(req.GetPageSize(), req.GetPageToken(), len(keys))
	if err != nil {
		return nil, err
	}
	s.relationshipPagesServed++

	rels := make([]*nmtspb.Relationship, 0, end-start)
	for _, key := range keys[start:end] {
		rels = append(rels, proto.Clone(s.relationships[key]).(*nmtspb.Relationship))
	}

	return &modelpb.ListRelationshipsResponse{Relationships: rels, NextPageToken: nextToken}, nil
}

func (s *InMemoryModelServer) UpsertFragment(_ context.Context, req *modelpb.UpsertFragmentRequest) (*modelpb.UpsertFragmentResponse, error) {
	fragment := req.GetFragment()
	if len(fragment.GetEntity()) == 0 && len(fragment.GetRelationship()) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "fragment has no entities or relationships")
	}

	// Apply the same admission checks the real ModelFE runs before it touches
	// storage, so a fragment this fake accepts is one the service would accept.
	if err := validation.ValidateFragment(fragment); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid fragment: %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Like the real ModelFE, reject a relationship whose endpoint is neither
	// in the fragment nor already stored, and write nothing from the fragment.
	inFragment := make(map[string]bool, len(fragment.GetEntity()))
	for _, e := range fragment.GetEntity() {
		inFragment[e.GetId()] = true
	}
	for _, r := range fragment.GetRelationship() {
		for _, id := range []string{r.GetA(), r.GetZ()} {
			if _, stored := s.entities[id]; !stored && !inFragment[id] {
				return nil, status.Errorf(codes.InvalidArgument, "relationship endpoint %q does not exist", id)
			}
		}
	}

	resp := &modelpb.UpsertFragmentResponse{}
	for _, e := range fragment.GetEntity() {
		if _, exists := s.entities[e.GetId()]; exists {
			resp.EntitiesUpdated++
		} else {
			resp.EntitiesCreated++
		}
		s.entities[e.GetId()] = proto.Clone(e).(*nmtspb.Entity)
	}
	for _, r := range fragment.GetRelationship() {
		key := relationshipKey{A: r.GetA(), Z: r.GetZ(), Kind: r.GetKind()}
		if _, exists := s.relationships[key]; !exists {
			resp.RelationshipsCreated++
		}
		s.relationships[key] = proto.Clone(r).(*nmtspb.Relationship)
	}
	s.upsertBatches = append(s.upsertBatches, upsertBatch{
		entities:      len(fragment.GetEntity()),
		relationships: len(fragment.GetRelationship()),
	})
	return resp, nil
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
		entity := &nmtspb.Entity{Id: fmt.Sprintf("entity-%04d", i)}
		// The first entity contains every other one, a pairing the model permits
		// between a platform and its ports.
		if i == 0 {
			entity.Kind = &nmtspb.Entity_EkPlatform{EkPlatform: &nmtsphypb.Platform{}}
		} else {
			entity.Kind = &nmtspb.Entity_EkPort{EkPort: &nmtsphypb.Port{}}
		}
		entities[i] = entity
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

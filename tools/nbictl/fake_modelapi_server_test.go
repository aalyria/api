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

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	modelpb "aalyria.com/spacetime/api/model/v1alpha"
	nmtspb "outernetcouncil.org/nmts/v1/proto"
)

type FakeModelServer struct {
	listener net.Listener

	modelpb.UnimplementedModelServer
	ResponseError   error
	ResponseMessage proto.Message
	RequestMessage  proto.Message
}

func (s *FakeModelServer) Reset() {
	s.ResponseError = nil
	s.ResponseMessage = nil
	s.RequestMessage = nil
}

// Handle any of the gRPC calls for simplistic testing:
//
//   - store the request for inspecton by the test
//   - if a response error is present, return the error
//   - else return the response message
func handleCall[RespT proto.Message](s *FakeModelServer, ctx context.Context, req proto.Message) (RespT, error) {
	var nilRespT RespT

	s.RequestMessage = req
	if s.ResponseError != nil {
		return nilRespT, s.ResponseError
	} else {
		resp := s.ResponseMessage.(RespT)
		return resp, nil
	}
}

func (s *FakeModelServer) UpsertEntity(ctx context.Context, req *modelpb.UpsertEntityRequest) (*modelpb.UpsertEntityResponse, error) {
	s.RequestMessage = req
	if s.ResponseError != nil {
		return nil, s.ResponseError
	} else {
		resp := s.ResponseMessage.(*modelpb.UpsertEntityResponse)
		return resp, nil
	}
}

func (s *FakeModelServer) UpdateEntity(ctx context.Context, req *modelpb.UpdateEntityRequest) (*nmtspb.Entity, error) {
	return handleCall[*nmtspb.Entity](s, ctx, req)
}

func (s *FakeModelServer) DeleteEntity(ctx context.Context, req *modelpb.DeleteEntityRequest) (*modelpb.DeleteEntityResponse, error) {
	return handleCall[*modelpb.DeleteEntityResponse](s, ctx, req)
}

func (s *FakeModelServer) CreateRelationship(ctx context.Context, req *modelpb.CreateRelationshipRequest) (*nmtspb.Relationship, error) {
	return handleCall[*nmtspb.Relationship](s, ctx, req)
}

func (s *FakeModelServer) DeleteRelationship(ctx context.Context, req *modelpb.DeleteRelationshipRequest) (*emptypb.Empty, error) {
	return handleCall[*emptypb.Empty](s, ctx, req)
}

func (s *FakeModelServer) GetEntity(ctx context.Context, req *modelpb.GetEntityRequest) (*modelpb.GetEntityResponse, error) {
	return handleCall[*modelpb.GetEntityResponse](s, ctx, req)
}

func (s *FakeModelServer) ListElements(ctx context.Context, req *modelpb.ListElementsRequest) (*modelpb.ListElementsResponse, error) {
	return handleCall[*modelpb.ListElementsResponse](s, ctx, req)
}

func startFakeModelServer(ctx context.Context, g *errgroup.Group, listener net.Listener) (*FakeModelServer, error) {
	fakeModelServer := &FakeModelServer{
		listener: listener,
	}
	fakeModelServer.Reset()
	server := grpc.NewServer()
	modelpb.RegisterModelServer(server, fakeModelServer)
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

	return fakeModelServer, nil
}

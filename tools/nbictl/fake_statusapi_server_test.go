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

	statuspb "aalyria.com/spacetime/api/status/v1"
)

type FakeStatusServer struct {
	listener net.Listener

	statuspb.UnimplementedStatusServiceServer
	ResponseError   error
	ResponseMessage proto.Message
	RequestMessage  proto.Message
}

func (s *FakeStatusServer) Reset() {
	s.ResponseError = nil
	s.ResponseMessage = nil
	s.RequestMessage = nil
}

// Handle any of the gRPC calls for simplistic testing:
//
//   - store the request for inspecton by the test
//   - if a response error is present, return the error
//   - else return the response message
func handleStatusServerCall[RespT proto.Message](s *FakeStatusServer, ctx context.Context, req proto.Message) (RespT, error) {
	var nilRespT RespT

	s.RequestMessage = req
	if s.ResponseError != nil {
		return nilRespT, s.ResponseError
	} else {
		resp := s.ResponseMessage.(RespT)
		return resp, nil
	}
}

func (s *FakeStatusServer) GetVersion(ctx context.Context, req *statuspb.GetVersionRequest) (*statuspb.GetVersionResponse, error) {
	return handleStatusServerCall[*statuspb.GetVersionResponse](s, ctx, req)
}

func startFakeStatusServer(ctx context.Context, g *errgroup.Group, listener net.Listener) (*FakeStatusServer, error) {
	fakeStatusServer := &FakeStatusServer{
		listener: listener,
	}
	fakeStatusServer.Reset()
	server := grpc.NewServer()
	statuspb.RegisterStatusServiceServer(server, fakeStatusServer)
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

	return fakeStatusServer, nil
}

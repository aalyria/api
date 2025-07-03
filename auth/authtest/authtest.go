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

// Package authtest provides helpers for testing functionality that uses the
// auth package.
package authtest // import "aalyria.com/spacetime/auth/authtest"

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"
)

type OIDCServer struct {
	*httptest.Server

	numCalls *atomic.Int64
}

func NewOIDCServer(idToken string) *OIDCServer {
	numCalls := &atomic.Int64{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		numCalls.Add(1)
		json.NewEncoder(w).Encode(map[string]interface{}{"id_token": idToken})
	}))

	return &OIDCServer{
		Server:   ts,
		numCalls: numCalls,
	}
}

func (s *OIDCServer) NumberOfCalls() int64 { return s.numCalls.Load() }

func (s *OIDCServer) Client() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				if addr == "www.googleapis.com:443" {
					addr = s.Server.Listener.Addr().String()
				}
				return (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext(ctx, network, addr)
			},
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: time.Second,
		},
	}
}

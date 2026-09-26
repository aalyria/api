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

// Package quictransport provides a QUIC/HTTP3-based dialer adapter for gRPC
// clients. It bridges gRPC's HTTP/2 transport to HTTP/3 over QUIC, allowing
// gRPC to operate over QUIC/UDP while preserving all existing interceptors,
// auth, and observability.
package quictransport

import (
	"context"
	"crypto/tls"
	"net"
	"net/http/httputil"
	"net/url"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"golang.org/x/net/http2"
)

// bridgedConn wraps a net.Conn and waits for a background goroutine to
// finish when Close is called, ensuring deterministic cleanup.
type bridgedConn struct {
	net.Conn
	done <-chan struct{}
}

func (c *bridgedConn) Close() error {
	err := c.Conn.Close()
	<-c.done
	return err
}

// NewDialer returns a function compatible with grpc.WithContextDialer that
// establishes gRPC connections over QUIC/HTTP3 transport.
//
// The returned dialer creates an in-process HTTP/2-to-HTTP/3 bridge: gRPC
// speaks HTTP/2 to a local reverse proxy, which forwards requests over
// HTTP/3 (QUIC) to the remote server. Closing the returned connection
// tears down the bridge and waits for all resources to be released.
//
// The provided tls.Config should have ServerName set to the target hostname
// for proper TLS SNI and HTTP Host header handling.
func NewDialer(tlsConf *tls.Config) func(ctx context.Context, addr string) (net.Conn, error) {
	conf := tlsConf.Clone()
	conf.MinVersion = max(conf.MinVersion, tls.VersionTLS13)

	return func(ctx context.Context, addr string) (net.Conn, error) {
		clientConn, serverConn := net.Pipe()

		// Build the target host for HTTP/3 requests using the hostname
		// from the TLS config and the port from the resolved address.
		host := conf.ServerName
		if host == "" {
			host = addr
		} else if _, port, err := net.SplitHostPort(addr); err == nil {
			host = net.JoinHostPort(host, port)
		}

		h3rt := &http3.Transport{
			TLSClientConfig: conf,
			// Route QUIC connections to the already-resolved address
			// while keeping the hostname for TLS SNI.
			Dial: func(ctx context.Context, _ string, tlsCfg *tls.Config, quicCfg *quic.Config) (*quic.Conn, error) {
				return quic.DialAddr(ctx, addr, tlsCfg, quicCfg)
			},
		}

		proxy := &httputil.ReverseProxy{
			Rewrite: func(r *httputil.ProxyRequest) {
				r.SetURL(&url.URL{Scheme: "https", Host: host})
				r.Out.Host = host
			},
			Transport:     h3rt,
			FlushInterval: -1, // flush immediately for gRPC streaming
		}

		done := make(chan struct{})
		go func() {
			defer close(done)
			defer serverConn.Close()
			defer h3rt.Close()

			h2s := &http2.Server{}
			h2s.ServeConn(serverConn, &http2.ServeConnOpts{
				Handler: proxy,
			})
		}()

		return &bridgedConn{Conn: clientConn, done: done}, nil
	}
}

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

package quictransport

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"
)

func generateSelfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}
}

func TestQUICDialer(t *testing.T) {
	cert := generateSelfSignedCert(t)

	// Set up a gRPC server that can handle HTTP requests.
	grpcServer := grpc.NewServer()
	healthSrv := health.NewServer()
	healthSrv.SetServingStatus("", healthgrpc.HealthCheckResponse_SERVING)
	healthgrpc.RegisterHealthServer(grpcServer, healthSrv)

	// Wrap the gRPC handler so it accepts HTTP/3 requests. gRPC's
	// ServeHTTP rejects requests where ProtoMajor != 2, so we override
	// the proto version before dispatching.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ProtoMajor = 2
		r.Proto = "HTTP/2.0"
		grpcServer.ServeHTTP(w, r)
	})

	udpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udpConn.Close()

	tr := &quic.Transport{Conn: udpConn}
	defer tr.Close()

	serverTLS := http3.ConfigureTLSConfig(&tls.Config{
		Certificates: []tls.Certificate{cert},
	})

	ql, err := tr.ListenEarly(serverTLS, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ql.Close()

	h3Server := &http3.Server{Handler: handler}
	var wg sync.WaitGroup
	wg.Go(func() {
		h3Server.ServeListener(ql)
	})
	defer wg.Wait()
	defer h3Server.Close()

	certPool := x509.NewCertPool()
	leafCert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	certPool.AddCert(leafCert)

	clientTLS := &tls.Config{
		RootCAs:    certPool,
		ServerName: "127.0.0.1",
	}

	dialer := NewDialer(clientTLS)
	conn, err := grpc.NewClient(
		udpConn.LocalAddr().String(),
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	client := healthgrpc.NewHealthClient(conn)
	resp, err := client.Check(t.Context(), &healthgrpc.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("health check RPC failed: %v", err)
	}
	if resp.GetStatus() != healthgrpc.HealthCheckResponse_SERVING {
		t.Errorf("unexpected status: got %v, want SERVING", resp.GetStatus())
	}
}

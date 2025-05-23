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
	"crypto/tls"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bazelbuild/rules_go/go/tools/bazel"
	"golang.org/x/sync/errgroup"

	nbi "aalyria.com/spacetime/api/nbi/v1alpha"
	"aalyria.com/spacetime/auth/authtest"
	"aalyria.com/spacetime/tools/nbictl/nbictlpb"
)

const (
	authHeader      = "authorization"
	proxyAuthHeader = "proxy-authorization"
	oidcToken       = `eyJhbGciOiJSUzI1NiIsImtpZCI6IjcyMTk0YjI2MzU0YzIzYzBiYTU5YTZkNzUxZGZmYWEyNTg2NTkwNGUiLCJ0eXAiOiJKV1QifQ.eyJhdWQiOiJodHRwczovL3d3dy5nb29nbGVhcGlzLmNvbS9vYXV0aDIvdjQvdG9rZW4iLCJleHAiOjE2ODE3OTIzMTksImlhdCI6MTY4MTc4ODcxOSwiaXNzIjoiY2RwaS1hZ2VudEBhNWEtc3BhY2V0aW1lLWdrZS1iYWNrLWRldi5pYW0uZ3NlcnZpY2VhY2NvdW50LmNvbSIsInN1YiI6ImNkcGktYWdlbnRAYTVhLXNwYWNldGltZS1na2UtYmFjay1kZXYuaWFtLmdzZXJ2aWNlYWNjb3VudC5jb20iLCJ0YXJnZXRfYXVkaWVuY2UiOiI2MDI5MjQwMzEzOS1tZTY4dGpnYWpsNWRjZGJwbmxtMmVrODMwbHZzbnNscS5hcHBzLmdvb2dsZXVzZXJjb250ZW50LmNvbSJ9.QyOi7vkFCwdmjT4ChT3_yVY4ZObUJkZkYC0q7alF_thiotdJKRiSo1ZHp_XnS0nM4WSWcQYLGHUDdAMPS0R22brFGzCl8ndgNjqI38yp_LDL8QVTqnLBGUj-m3xB5wH17Q_Dt8riBB4IE-mSS8FB-R6sqSwn-seMfMDydScC0FrtOF3-2BCYpIAlf1AQKN083QdtKgNEVDi72npPr2MmsWV3tct6ydXHWNbxG423kfSD6vCZSUTvWXAuVjuOwnbc2LHZS04U-jiLpvHxu06OwHOQ5LoGVPyd69o8Ny_Bapd2m0YCX2xJr8_HH2nw1jH7EplFf-owbBYz9ZtQoQ2YTA`
)

// LocalhostCert is a PEM-encoded TLS cert with SAN IPs
// "127.0.0.1" and "[::1]", expiring at Jan 29 16:00:00 2084 GMT.
// generated from src/crypto/tls:
// go run generate_cert.go  --rsa-bits 2048 --host 127.0.0.1,::1,example.com --ca --start-date "Jan 1 00:00:00 1970" --duration=1000000h
var LocalhostCert = []byte(`-----BEGIN CERTIFICATE-----
MIIDOTCCAiGgAwIBAgIQSRJrEpBGFc7tNb1fb5pKFzANBgkqhkiG9w0BAQsFADAS
MRAwDgYDVQQKEwdBY21lIENvMCAXDTcwMDEwMTAwMDAwMFoYDzIwODQwMTI5MTYw
MDAwWjASMRAwDgYDVQQKEwdBY21lIENvMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8A
MIIBCgKCAQEA6Gba5tHV1dAKouAaXO3/ebDUU4rvwCUg/CNaJ2PT5xLD4N1Vcb8r
bFSW2HXKq+MPfVdwIKR/1DczEoAGf/JWQTW7EgzlXrCd3rlajEX2D73faWJekD0U
aUgz5vtrTXZ90BQL7WvRICd7FlEZ6FPOcPlumiyNmzUqtwGhO+9ad1W5BqJaRI6P
YfouNkwR6Na4TzSj5BrqUfP0FwDizKSJ0XXmh8g8G9mtwxOSN3Ru1QFc61Xyeluk
POGKBV/q6RBNklTNe0gI8usUMlYyoC7ytppNMW7X2vodAelSu25jgx2anj9fDVZu
h7AXF5+4nJS4AAt0n1lNY7nGSsdZas8PbQIDAQABo4GIMIGFMA4GA1UdDwEB/wQE
AwICpDATBgNVHSUEDDAKBggrBgEFBQcDATAPBgNVHRMBAf8EBTADAQH/MB0GA1Ud
DgQWBBStsdjh3/JCXXYlQryOrL4Sh7BW5TAuBgNVHREEJzAlggtleGFtcGxlLmNv
bYcEfwAAAYcQAAAAAAAAAAAAAAAAAAAAATANBgkqhkiG9w0BAQsFAAOCAQEAxWGI
5NhpF3nwwy/4yB4i/CwwSpLrWUa70NyhvprUBC50PxiXav1TeDzwzLx/o5HyNwsv
cxv3HdkLW59i/0SlJSrNnWdfZ19oTcS+6PtLoVyISgtyN6DpkKpdG1cOkW3Cy2P2
+tK/tKHRP1Y/Ra0RiDpOAmqn0gCOFGz8+lqDIor/T7MTpibL3IxqWfPrvfVRHL3B
grw/ZQTTIVjjh4JBSW3WyWgNo/ikC1lrVxzl4iPUGptxT36Cr7Zk2Bsg0XqwbOvK
5d+NTDREkSnUbie4GeutujmX3Dsx88UiV6UY/4lHJa6I5leHUNOHahRbpbWeOfs/
WkBKOclmOV2xlTVuPw==
-----END CERTIFICATE-----`)

// LocalhostKey is the private key for LocalhostCert.
var LocalhostKey = []byte(testingKey(`-----BEGIN RSA TESTING KEY-----
MIIEvAIBADANBgkqhkiG9w0BAQEFAASCBKYwggSiAgEAAoIBAQDoZtrm0dXV0Aqi
4Bpc7f95sNRTiu/AJSD8I1onY9PnEsPg3VVxvytsVJbYdcqr4w99V3AgpH/UNzMS
gAZ/8lZBNbsSDOVesJ3euVqMRfYPvd9pYl6QPRRpSDPm+2tNdn3QFAvta9EgJ3sW
URnoU85w+W6aLI2bNSq3AaE771p3VbkGolpEjo9h+i42TBHo1rhPNKPkGupR8/QX
AOLMpInRdeaHyDwb2a3DE5I3dG7VAVzrVfJ6W6Q84YoFX+rpEE2SVM17SAjy6xQy
VjKgLvK2mk0xbtfa+h0B6VK7bmODHZqeP18NVm6HsBcXn7iclLgAC3SfWU1jucZK
x1lqzw9tAgMBAAECggEABWzxS1Y2wckblnXY57Z+sl6YdmLV+gxj2r8Qib7g4ZIk
lIlWR1OJNfw7kU4eryib4fc6nOh6O4AWZyYqAK6tqNQSS/eVG0LQTLTTEldHyVJL
dvBe+MsUQOj4nTndZW+QvFzbcm2D8lY5n2nBSxU5ypVoKZ1EqQzytFcLZpTN7d89
EPj0qDyrV4NZlWAwL1AygCwnlwhMQjXEalVF1ylXwU3QzyZ/6MgvF6d3SSUlh+sq
XefuyigXw484cQQgbzopv6niMOmGP3of+yV4JQqUSb3IDmmT68XjGd2Dkxl4iPki
6ZwXf3CCi+c+i/zVEcufgZ3SLf8D99kUGE7v7fZ6AQKBgQD1ZX3RAla9hIhxCf+O
3D+I1j2LMrdjAh0ZKKqwMR4JnHX3mjQI6LwqIctPWTU8wYFECSh9klEclSdCa64s
uI/GNpcqPXejd0cAAdqHEEeG5sHMDt0oFSurL4lyud0GtZvwlzLuwEweuDtvT9cJ
Wfvl86uyO36IW8JdvUprYDctrQKBgQDycZ697qutBieZlGkHpnYWUAeImVA878sJ
w44NuXHvMxBPz+lbJGAg8Cn8fcxNAPqHIraK+kx3po8cZGQywKHUWsxi23ozHoxo
+bGqeQb9U661TnfdDspIXia+xilZt3mm5BPzOUuRqlh4Y9SOBpSWRmEhyw76w4ZP
OPxjWYAgwQKBgA/FehSYxeJgRjSdo+MWnK66tjHgDJE8bYpUZsP0JC4R9DL5oiaA
brd2fI6Y+SbyeNBallObt8LSgzdtnEAbjIH8uDJqyOmknNePRvAvR6mP4xyuR+Bv
m+Lgp0DMWTw5J9CKpydZDItc49T/mJ5tPhdFVd+am0NAQnmr1MCZ6nHxAoGABS3Y
LkaC9FdFUUqSU8+Chkd/YbOkuyiENdkvl6t2e52jo5DVc1T7mLiIrRQi4SI8N9bN
/3oJWCT+uaSLX2ouCtNFunblzWHBrhxnZzTeqVq4SLc8aESAnbslKL4i8/+vYZlN
s8xtiNcSvL+lMsOBORSXzpj/4Ot8WwTkn1qyGgECgYBKNTypzAHeLE6yVadFp3nQ
Ckq9yzvP/ib05rvgbvrne00YeOxqJ9gtTrzgh7koqJyX1L4NwdkEza4ilDWpucn0
xiUZS4SoaJq6ZvcBYS62Yr1t8n09iG47YL8ibgtmH3L+svaotvpVxVK+d7BLevA/
ZboOWVe3icTy64BT3OQhmg==
-----END RSA TESTING KEY-----`))

func TestDial_insecure(t *testing.T) {
	t.Parallel()

	// Start fake NBI server and a timeout for the test
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	g, ctx := errgroup.WithContext(ctx)
	defer func() { checkErr(t, g.Wait()) }()
	defer cancel()

	srv := startInsecureServer(ctx, t, g)
	// Invoke OpenConnection
	nbiConf := &nbictlpb.Config{
		Url: srv.listener.Addr().String(),
		TransportSecurity: &nbictlpb.Config_TransportSecurity{
			Type: &nbictlpb.Config_TransportSecurity_Insecure{},
		},
	}
	conn, err := dial(ctx, nbiConf, nil)
	checkErr(t, err)
	defer conn.Close()

	// Test a gRPC method invocation
	client := nbi.NewNetOpsClient(conn)
	_, err = client.ListEntities(ctx, &nbi.ListEntitiesRequest{Type: nbi.EntityType_ANTENNA_PATTERN.Enum()})
	checkErr(t, err)

	// Verify that the method invocation reached the server
	if srv.NumCallsListEntities.Load() != 1 {
		t.Fatal("ListEntities has not been invoked correctly")
	}
	// Verify that when transportSecurity = insecure, the gRPC headers
	// authHeader and proxyAuthHeader are NOT transmitted to the server.
	if len(srv.IncomingMetadata[0].Get(authHeader)) > 0 {
		t.Fatal("Unexpected Incoming Metadata: ", authHeader)
	}
	if len(srv.IncomingMetadata[0].Get(proxyAuthHeader)) > 0 {
		t.Fatal("Unexpected Incoming Metadata: ", proxyAuthHeader)
	}
}

func TestDial_serverCertificate(t *testing.T) {
	t.Parallel()

	// Store on filesystem the localhost certificate and the user priv/pub key
	tmpDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)
	serverCertPath := filepath.Join(tmpDir, "localhost.crt.tls")
	checkErr(t, os.WriteFile(serverCertPath, LocalhostCert, 0o644))

	userKeys := generateKeysForTesting(t, tmpDir, "--org", "user.organization")

	// Start fake NBI server WITH TLS and a timeout for the test
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	g, ctx := errgroup.WithContext(ctx)
	defer func() { checkErr(t, g.Wait()) }()
	defer cancel()
	cert, _ := tls.X509KeyPair(LocalhostCert, LocalhostKey)
	lis, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"h2"}})
	checkErr(t, err)
	fakeGrpcServer, err := startFakeNbiServer(ctx, g, lis)
	checkErr(t, err)

	// Start a fake OIDCServer
	ts := authtest.NewOIDCServer(oidcToken)
	defer ts.Close()

	// Invoke OpenConnection
	nbiConf := &nbictlpb.Config{
		Url:     lis.Addr().String(),
		PrivKey: userKeys.key,
		Name:    "test",
		KeyId:   "1",
		Email:   "some@example.com",
		TransportSecurity: &nbictlpb.Config_TransportSecurity{
			Type: &nbictlpb.Config_TransportSecurity_ServerCertificate_{
				ServerCertificate: &nbictlpb.Config_TransportSecurity_ServerCertificate{
					CertFilePath: serverCertPath,
				},
			},
		},
	}
	conn, err := dial(ctx, nbiConf, ts.Client())
	checkErr(t, err)
	defer conn.Close()

	// Test a gRPC method invocation
	client := nbi.NewNetOpsClient(conn)
	_, err = client.ListEntities(ctx, &nbi.ListEntitiesRequest{Type: nbi.EntityType_ANTENNA_PATTERN.Enum()})
	checkErr(t, err)

	// Verify that the method invocation reached the server
	if fakeGrpcServer.NumCallsListEntities.Load() != 1 {
		t.Fatal("ListEntities has not been invoked correctly")
	}
	// Verify that when transportSecurity != insecure, the gRPC headers
	// authHeader and proxyAuthHeader are transmitted to the server.
	if len(fakeGrpcServer.IncomingMetadata[0].Get(authHeader)) != 1 {
		t.Fatal("Missing incoming metadata: ", authHeader)
	}
	if len(fakeGrpcServer.IncomingMetadata[0].Get(proxyAuthHeader)) != 1 {
		t.Fatal("Missing incoming metadata: ", proxyAuthHeader)
	}
	if want, got := "Bearer "+oidcToken, fakeGrpcServer.IncomingMetadata[0].Get(proxyAuthHeader)[0]; want != got {
		t.Fatalf("fakeGrpcServer received the wrong proxyAuthHeader header: got %+v, wanted %+v ", got, want)
	}
}

func testingKey(s string) string {
	return strings.ReplaceAll(s, "TESTING KEY", "PRIVATE KEY")
}

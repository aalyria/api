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
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/jonboulle/clockwork"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding/gzip"
	_ "google.golang.org/grpc/encoding/gzip" // Install the gzip compressor

	"aalyria.com/spacetime/auth"
	"aalyria.com/spacetime/common/quictransport"
	"aalyria.com/spacetime/tools/nbictl/nbictlpb"
)

func readPrivateKeyFromSigningStrategy(ss *nbictlpb.Config_SigningStrategy) (*bytes.Buffer, error) {
	switch s := ss.GetType().(type) {
	case *nbictlpb.Config_SigningStrategy_PrivateKeyFile:
		pkeyBytes, err := os.ReadFile(s.PrivateKeyFile)
		if err != nil {
			return nil, fmt.Errorf("unable to read private key file: %w", err)
		}
		return bytes.NewBuffer(pkeyBytes), nil
	case *nbictlpb.Config_SigningStrategy_PrivateKeyBytes:
		return bytes.NewBuffer(s.PrivateKeyBytes), nil
	default:
		return nil, errors.New("no signing strategy configured")
	}
}

func resolvePerRPCCredentials(ctx context.Context, setting *nbictlpb.Config, isInsecure, useQUIC bool) (credentials.PerRPCCredentials, error) {
	clock := clockwork.NewRealClock()

	switch t := setting.GetAuthStrategy().GetType().(type) {
	case *nbictlpb.Config_AuthStrategy_None:
		return nil, nil

	case *nbictlpb.Config_AuthStrategy_Jwt_:
		jwt := t.Jwt
		privateKey, err := readPrivateKeyFromSigningStrategy(jwt.GetSigningStrategy())
		if err != nil {
			return nil, err
		}
		config := auth.Config{
			Clock:                 clock,
			PrivateKey:            privateKey,
			PrivateKeyID:          jwt.GetPrivateKeyId(),
			Email:                 jwt.GetEmail(),
			SkipTransportSecurity: useQUIC || isInsecure,
		}
		creds, err := auth.NewCredentials(ctx, config)
		if err != nil {
			return nil, fmt.Errorf("unable to get new credentials with provided information: %w", err)
		}
		return creds, nil

	case nil:
		// Backward compatibility: auth_strategy not set, use deprecated fields.
		if isInsecure {
			return nil, nil
		}
		if setting.GetPrivKey() == "" {
			return nil, errors.New("no private key set for chosen context")
		}
		pkeyBytes, err := os.ReadFile(setting.GetPrivKey())
		if err != nil {
			return nil, fmt.Errorf("unable to read the file: %w", err)
		}
		config := auth.Config{
			Clock:                 clock,
			PrivateKey:            bytes.NewBuffer(pkeyBytes),
			PrivateKeyID:          setting.GetKeyId(),
			Email:                 setting.GetEmail(),
			SkipTransportSecurity: useQUIC,
		}
		creds, err := auth.NewCredentials(ctx, config)
		if err != nil {
			return nil, fmt.Errorf("unable to get new credentials with provided information: %w", err)
		}
		return creds, nil

	default:
		return nil, fmt.Errorf("unexpected auth strategy type: %T", t)
	}
}

func openAPIConnection(appCtx *cli.Context, apiSubDomain string) (*grpc.ClientConn, error) {
	profileName := appCtx.String("profile")

	appConfDir, err := getAppConfDir(appCtx)
	if err != nil {
		return nil, err
	}
	setting, err := readConfig(profileName, filepath.Join(appConfDir, confFileName))
	if err != nil {
		return nil, fmt.Errorf("unable to obtain context information: %w", err)
	}
	url := setting.GetUrl()
	originalURL := url
	var containsDnsSchema bool
	url, containsDnsSchema = strings.CutPrefix(url, "dns:///")
	if containsDnsSchema {
		return nil, fmt.Errorf("URL (%s) with dns:/// prefix is unsupported. Please provide only host[:port].", originalURL)
	}
	url, err = adjustURLForAPISubDomain(url, apiSubDomain)
	if err != nil {
		return nil, err
	}

	setting.Url = url
	return dial(appCtx.Context, setting)
}

func adjustURLForAPISubDomain(url string, apiSubDomain string) (string, error) {
	// Unexpectedly empty arguments or already the subdomain sought.
	if url == "" || apiSubDomain == "" || strings.HasPrefix(url, apiSubDomain+".") {
		return url, nil
	}

	// If the |url| is an ip:port then best to leave it alone.
	if host, _, err := net.SplitHostPort(url); err == nil {
		if net.ParseIP(host) != nil {
			return url, nil
		}
	}

	// TODO: Remove error after a release, so that customers do not depend on unsupported configuration.
	if strings.HasPrefix(url, "nbi.") {
		err := fmt.Errorf("URL (%s) with nbi subdomain is unsupported. Please remove the nbi subdomain.", url)
		return "", err
	}

	return apiSubDomain + "." + url, nil
}

func dial(ctx context.Context, setting *nbictlpb.Config) (*grpc.ClientConn, error) {
	dialOpts, err := getDialOpts(ctx, setting)
	if err != nil {
		return nil, fmt.Errorf("unable to construct dial options: %w", err)
	}
	conn, err := grpc.NewClient(setting.GetUrl(), dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to the server: %w", err)
	}
	return conn, nil
}

func getDialOpts(ctx context.Context, setting *nbictlpb.Config) ([]grpc.DialOption, error) {
	dialOpts := []grpc.DialOption{
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(1024*1024*256), grpc.UseCompressor(gzip.Name)),
	}

	var cp *x509.CertPool
	var transportCreds credentials.TransportCredentials
	switch t := setting.GetTransportSecurity().GetType().(type) {
	case *nbictlpb.Config_TransportSecurity_Insecure:
		transportCreds = insecure.NewCredentials()

	case *nbictlpb.Config_TransportSecurity_ServerCertificate_:
		certFile := t.ServerCertificate.GetCertFilePath()
		clientTLSFromFile, err := credentials.NewClientTLSFromFile(certFile, "")
		if err != nil {
			return nil, fmt.Errorf("creating TLS credentials from certificate file: %w", err)
		}
		transportCreds = clientTLSFromFile
		certPEM, err := os.ReadFile(certFile)
		if err != nil {
			return nil, fmt.Errorf("reading certificate file for QUIC: %w", err)
		}
		cp = x509.NewCertPool()
		cp.AppendCertsFromPEM(certPEM)

	// SystemCertPool is the default option in case transport_security is not set (nil).
	case nil, *nbictlpb.Config_TransportSecurity_SystemCertPool:
		var err error
		cp, err = x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("reading system tls cert pool: %w", err)
		}
		transportCreds = credentials.NewClientTLSFromCert(cp, "")

	default:
		return nil, fmt.Errorf("unexpected transport security selection: %T", t)
	}

	var useQUIC bool
	switch setting.GetTransport().GetType().(type) {
	case *nbictlpb.Config_Transport_Quic:
		if _, isInsecure := setting.GetTransportSecurity().GetType().(*nbictlpb.Config_TransportSecurity_Insecure); isInsecure {
			return nil, errors.New("QUIC transport requires TLS; incompatible with insecure transport_security")
		}
		host := setting.GetUrl()
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		tlsConf := &tls.Config{
			RootCAs:    cp,
			ServerName: host,
			MinVersion: tls.VersionTLS13,
		}
		transportCreds = insecure.NewCredentials()
		dialOpts = append(dialOpts, grpc.WithContextDialer(quictransport.NewDialer(tlsConf)))
		useQUIC = true
	case nil, *nbictlpb.Config_Transport_Tcp:
		// Default: TCP, no changes needed.
	}

	dialOpts = append(dialOpts, grpc.WithTransportCredentials(transportCreds))

	_, isInsecure := setting.GetTransportSecurity().GetType().(*nbictlpb.Config_TransportSecurity_Insecure)
	perRPCCreds, err := resolvePerRPCCredentials(ctx, setting, isInsecure, useQUIC)
	if err != nil {
		return nil, err
	}
	if perRPCCreds != nil {
		dialOpts = append(dialOpts, grpc.WithPerRPCCredentials(perRPCCreds))
	}

	return dialOpts, nil
}

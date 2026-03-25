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
	"aalyria.com/spacetime/tools/nbictl/nbictlpb"
)

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

	switch t := setting.GetTransportSecurity().GetType().(type) {
	case *nbictlpb.Config_TransportSecurity_Insecure:
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))

	case *nbictlpb.Config_TransportSecurity_ServerCertificate_:
		clientTLSFromFile, err := credentials.NewClientTLSFromFile(t.ServerCertificate.GetCertFilePath(), "")
		if err != nil {
			return nil, fmt.Errorf("creating TLS credentials from certificate file: %w", err)
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(clientTLSFromFile))

	// SystemCertPoll is the default option in case transport_security is not set (nil).
	case nil, *nbictlpb.Config_TransportSecurity_SystemCertPool:
		cp, err := x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("reading system tls cert pool: %w", err)
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(cp, "")))

	default:
		return nil, fmt.Errorf("unexpected transport security selection: %T", t)
	}

	// Unless transport-security is set to Insecure, add Spacetime PerRPCCredentials.
	if _, insecure := setting.GetTransportSecurity().GetType().(*nbictlpb.Config_TransportSecurity_Insecure); !insecure {
		if setting.GetPrivKey() == "" {
			return nil, errors.New("no private key set for chosen context")
		}
		pkeyBytes, err := os.ReadFile(setting.GetPrivKey())
		if err != nil {
			return nil, fmt.Errorf("unable to read the file: %w", err)
		}
		privateKey := bytes.NewBuffer(pkeyBytes)
		clock := clockwork.NewRealClock()

		config := auth.Config{
			Clock:        clock,
			PrivateKey:   privateKey,
			PrivateKeyID: setting.GetKeyId(),
			Email:        setting.GetEmail(),
		}

		creds, err := auth.NewCredentials(ctx, config)
		if err != nil {
			return nil, fmt.Errorf("unable to get new credentials with provided information: %w", err)
		}

		dialOpts = append(dialOpts, grpc.WithPerRPCCredentials(creds))
	}

	return dialOpts, nil
}

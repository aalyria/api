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

package main

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"

	"aalyria.com/spacetime/cdpi_agent/internal/auth"

	"github.com/jonboulle/clockwork"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	pb "aalyria.com/spacetime/api/nbi/v1alpha"
	nbi "aalyria.com/spacetime/github/tools/nbictl"
)

var (
	privKey = flag.String("priv_key", "", "path to your private key for authentication to NBI API")
	keyID   = flag.String("key_id", "", "key id associated with the provate key provided by Aalyria")
	userID  = flag.String("user_id", "", "user id address associated with the private key provided by Aalyria")
	url     = flag.String("url", "", "url of NBI endpoint")
	subCmds = map[string]func(context.Context, pb.NetOpsClient, []string) error{
		"list":          nbi.List,
		"create":        nbi.Create,
		"update":        nbi.Update,
		"delete":        nbi.Delete,
		"generate-keys": nbi.GenerateKeys,
	}
)

const (
	linkToAuthGuide = "https://docs.spacetime.aalyria.com/authentication"
	clientName      = "nbictl"
)

func getDialOpts(ctx context.Context) ([]grpc.DialOption, error) {
	dialOpts := []grpc.DialOption{grpc.WithBlock()}
	file, err := os.ReadFile(*privKey)
	if err != nil {
		return nil, fmt.Errorf("unable to open the file: %w", err)
	}
	privateKey := bytes.NewBuffer(file)

	clock := clockwork.NewRealClock()
	creds, err := auth.NewCredentials(ctx, auth.Config{
		Clock:        clock,
		PrivateKey:   privateKey,
		PrivateKeyID: *keyID,
		Email:        *userID,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to get new credentials with provided information: %w", err)
	}

	dialOpts = append(dialOpts, grpc.WithPerRPCCredentials(creds))
	cp, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("reading system tls cert pool: %w", err)
	}
	dialOpts = append(dialOpts, grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(cp, "")))

	return dialOpts, nil
}

func getSubcommandNames() []string {
	var cmdList []string
	for cmd := range subCmds {
		cmdList = append(cmdList, cmd)
	}
	return cmdList
}

func getConnection(ctx context.Context) (*grpc.ClientConn, error) {
	switch {
	case *privKey == "":
		return nil, fmt.Errorf("missing required flag --priv_key. For more information please refer to: %s", linkToAuthGuide)
	case *keyID == "":
		return nil, errors.New("missing required flag --key_id (key ID associated with the private key provided by Aalyria)")
	case *userID == "":
		return nil, errors.New("missing required flag --user_id (user ID associated with the private key provided by Aalyria)")
	case *url == "":
		return nil, errors.New("missing required flag --url (url of NBI endpoint)")
	}
	dialOpts, err := getDialOpts(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to construct dial options: %w", err)
	}

	conn, err := grpc.DialContext(ctx, *url, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to the server: %w", err)
	}
	return conn, nil
}

func run() error {
	ctx := context.Background()

	conn, err := getConnection(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	args := flag.Args()
	if flag.NArg() == 0 {
		return errors.New("Please specify a subcommand")
	}

	client := pb.NewNetOpsClient(conn)
	cmd, args := args[0], args[1:]

	cmdToPerform, ok := subCmds[cmd]
	if !ok {
		return fmt.Errorf("invalid command: %s. must be one of %v", cmd, getSubcommandNames())
	}
	return cmdToPerform(ctx, client, args)
}

func main() {
	flag.Parse()
	if err := run(); err != nil {
		log.Fatalf("fatal error: %v", err)
	}
}

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

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"time"

	"github.com/jonboulle/clockwork"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	nmts "outernetcouncil.org/nmts/v1/proto"

	model "aalyria.com/spacetime/api/model/v1"
	"aalyria.com/spacetime/auth"
)

const (
	maxMessageSize    = 256 * 1024 * 1024
	keepAliveTime     = 30 * time.Second
	keepAliveTimeout  = 10 * time.Second
	keepAliveInterval = 5 * time.Second
)

// NewCredentialsFromPrivateKey creates gRPC credentials from a private key string.
func NewCredentialsFromPrivateKey(email, privateKeyID, privateKey string) (credentials.PerRPCCredentials, error) {
	// Create auth config using the auth package
	authConfig := auth.Config{
		Clock:        clockwork.NewRealClock(),
		PrivateKey:   strings.NewReader(privateKey),
		PrivateKeyID: privateKeyID,
		Email:        email,
	}

	// Create credentials using the auth package
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return auth.NewCredentials(ctx, authConfig)
}

// listPageTimeout bounds one page of a walk. A whole model takes as many
// requests as it has pages, so a deadline covering the walk would have to grow
// with the model; one that covers a single page does not.
const listPageTimeout = 30 * time.Second

// listEntities calls the Model API to list entities, walking every page and returning the entities from all of them.
// The page size comes from the -page_size flag; 0 lets the server choose.
func listEntities(ctx context.Context, client model.ModelClient, pageSize int32) ([]*nmts.Entity, error) {
	var entities []*nmts.Entity

	req := &model.ListEntitiesRequest{PageSize: pageSize}

	for {
		pageCtx, cancel := context.WithTimeout(ctx, listPageTimeout)
		resp, err := client.ListEntities(pageCtx, req)
		cancel()
		if err != nil {
			return nil, fmt.Errorf("failed to list entities: %w", err)
		}
		entities = append(entities, resp.GetEntities()...)
		if resp.GetNextPageToken() == "" {
			return entities, nil
		}
		// A cursor that fails to advance would walk the same page forever.
		// Returning the entities read so far would look like the whole model
		// to a caller that acts on it, so this fails instead.
		if resp.GetNextPageToken() == req.PageToken {
			return nil, fmt.Errorf("ListEntities repeated the page token it was given (%d entities read); the walk cannot advance", len(entities))
		}
		req.PageToken = resp.GetNextPageToken()
	}
}

// establishConnection creates a gRPC connection to the Model API with authentication.
func establishConnection(target, email, keyID, privateKey string) (*grpc.ClientConn, error) {
	// Create call credentials
	callCreds, err := NewCredentialsFromPrivateKey(email, keyID, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create call credentials: %w", err)
	}

	// Create composite credentials with TLS
	creds := credentials.NewTLS(nil)
	compositeCreds := grpc.WithPerRPCCredentials(callCreds)

	// Build connection options
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
		compositeCreds,
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxMessageSize)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                keepAliveTime,
			Timeout:             keepAliveTimeout,
			PermitWithoutStream: true,
		}),
	}

	// Connect to the server
	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", target, err)
	}

	return conn, nil
}

func run() error {
	// Parse command line arguments
	var (
		target         = flag.String("target", "", "The target URL of the Spacetime Model API (e.g., 'api.example.com' or 'api.example.com:8080')")
		email          = flag.String("email", "", "Client email for Spacetime authentication")
		keyID          = flag.String("key_id", "", "Client key ID for Spacetime authentication")
		privateKeyPath = flag.String("private_key_path", "", "Path to the private key file")
		pageSize       = flag.Int("page_size", 1000, "Page size to limit query to; default = 1000")
	)
	flag.Parse()

	// Validate required arguments
	if *target == "" || *email == "" || *keyID == "" || *privateKeyPath == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s -target <target> -email <email> -key_id <key_id> -private_key_path <private_key_path>\n", os.Args[0])
		flag.PrintDefaults()
		return fmt.Errorf("missing required arguments")
	}

	// Validate page size argument
	if *pageSize < 0 || *pageSize > math.MaxInt32 {
		return fmt.Errorf("page size is expected to be between 0 and %v, received: %v", math.MaxInt32, *pageSize)
	}

	// Read private key
	privateKeyBytes, err := os.ReadFile(*privateKeyPath)
	if err != nil {
		return fmt.Errorf("error reading private key file: %w", err)
	}
	privateKey := string(privateKeyBytes)

	// Establish connection
	conn, err := establishConnection(*target, *email, *keyID, privateKey)
	if err != nil {
		return fmt.Errorf("error establishing connection: %w", err)
	}
	defer conn.Close()

	// Create client
	client := model.NewModelClient(conn)

	// Each page of the walk below is bounded by its own deadline, so the walk
	// itself runs for as long as the model needs.
	ctx := context.Background()

	// List entities
	entities, err := listEntities(ctx, client, int32(*pageSize))
	if err != nil {
		return fmt.Errorf("error listing entities: %w", err)
	}

	// Print results
	fmt.Printf("ListEntitiesResponse received:\n%v", entities)
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatalf("%v", err)
	}
}

// Copyright (c) Aalyria Technologies, Inc., and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package agentcli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jonboulle/clockwork"
	"google.golang.org/grpc/credentials"

	"aalyria.com/spacetime/agent/internal/configpb"
	"aalyria.com/spacetime/auth"
)

// serviceAccountKey holds the fields of an identity provider key file that
// the agent uses.
type serviceAccountKey struct {
	KeyID  string `json:"keyId"`
	Key    string `json:"key"`
	UserID string `json:"userId"`
}

// maxServiceAccountKeyBytes caps the key file. A key file is about two
// kilobytes, so anything near this size names the wrong file.
const maxServiceAccountKeyBytes = 1 << 20

// readServiceAccountKey reads and checks the key file at path. The key
// material never goes into an error.
func readServiceAccountKey(path string) (*serviceAccountKey, error) {
	// Stat before Open: opening a named pipe blocks until a writer appears,
	// and the agent would hang at startup with no diagnostic.
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("reading service account key file %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("reading service account key file %s: not a regular file", path)
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("reading service account key file %s: %w", path, err)
	}
	defer f.Close()

	// One byte past the cap tells a file at the cap from one over it.
	data, err := io.ReadAll(io.LimitReader(f, maxServiceAccountKeyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading service account key file %s: %w", path, err)
	}
	if len(data) > maxServiceAccountKeyBytes {
		return nil, fmt.Errorf("reading service account key file %s: larger than %d bytes", path, maxServiceAccountKeyBytes)
	}

	var key serviceAccountKey
	if err := json.Unmarshal(data, &key); err != nil {
		return nil, fmt.Errorf("parsing service account key file %s: %w", path, err)
	}

	errs := []error{}
	if key.KeyID == "" {
		errs = append(errs, fmt.Errorf("service account key file %s: missing required field 'keyId'", path))
	}
	if key.Key == "" {
		errs = append(errs, fmt.Errorf("service account key file %s: missing required field 'key'", path))
	}
	if key.UserID == "" {
		// An application key has the same shape but no userId, so this is the
		// field that tells the two apart.
		errs = append(errs, fmt.Errorf("service account key file %s: missing required field 'userId'; the file can be an application key and not a service account key", path))
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return &key, nil
}

// oidcServiceAccountConfig maps the agent config and the key file to the
// auth config. It sets no TokenStore: the agent is a long-running process,
// and the in-process token cache in auth is sufficient.
func oidcServiceAccountConfig(spec *configpb.AuthStrategy_OidcServiceAccount, key *serviceAccountKey, clock clockwork.Clock, skipTransportSecurity bool) auth.OIDCServiceAccountConfig {
	return auth.OIDCServiceAccountConfig{
		Clock:                 clock,
		Issuer:                spec.GetIssuer(),
		ProjectID:             spec.GetProjectId(),
		KeyID:                 key.KeyID,
		UserID:                key.UserID,
		PrivateKey:            strings.NewReader(key.Key),
		SkipTransportSecurity: skipTransportSecurity,
	}
}

// newOIDCServiceAccountCredentials checks spec, reads the key file, and makes
// the credentials.
func newOIDCServiceAccountCredentials(ctx context.Context, spec *configpb.AuthStrategy_OidcServiceAccount, clock clockwork.Clock, skipTransportSecurity bool) (credentials.PerRPCCredentials, error) {
	// Report every missing field at once, so that an operator fixes the
	// config in one round.
	errs := []error{}
	if spec.GetIssuer() == "" {
		errs = append(errs, errors.New("missing required field 'issuer'"))
	}
	if spec.GetProjectId() == "" {
		errs = append(errs, errors.New("missing required field 'project_id'"))
	}
	if spec.GetServiceAccountKeyFile() == "" {
		errs = append(errs, errors.New("missing required field 'service_account_key_file'"))
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	if !strings.HasPrefix(spec.GetIssuer(), "https://") {
		return nil, fmt.Errorf("field 'issuer' must start with https://, got %q", spec.GetIssuer())
	}

	path := spec.GetServiceAccountKeyFile()
	key, err := readServiceAccountKey(path)
	if err != nil {
		return nil, err
	}
	creds, err := auth.NewOIDCServiceAccountCredentials(ctx, oidcServiceAccountConfig(spec, key, clock, skipTransportSecurity))
	if err != nil {
		return nil, fmt.Errorf("service account key file %s: %w", path, err)
	}
	return creds, nil
}

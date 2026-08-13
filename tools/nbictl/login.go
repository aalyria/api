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
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/urfave/cli/v2"

	"aalyria.com/spacetime/auth"
	"aalyria.com/spacetime/tools/nbictl/nbictlpb"
)

const tokenDirName = "tokens"

// unsafeProfileChars matches every character that must not reach a file
// name. A profile name is user-chosen, so it can hold a path separator.
var unsafeProfileChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// fileTokenStore holds the tokens of one profile in a JSON file with mode
// 0600. It implements [auth.TokenStore].
type fileTokenStore struct {
	path string
}

func newFileTokenStore(confDir, profileName string) *fileTokenStore {
	// Replacing the unsafe characters is not injective: "prod/x" and "prod_x"
	// both reduce to "prod_x". The two profiles can name different issuers and
	// different NBI hosts, so a shared file would send one profile's ID token
	// to the other profile's endpoint. A digest of the original name keeps
	// them apart.
	digest := sha256.Sum256([]byte(profileName))
	name := unsafeProfileChars.ReplaceAllString(profileName, "_")
	fileName := fmt.Sprintf("%s-%x.json", name, digest[:4])
	return &fileTokenStore{path: filepath.Join(confDir, tokenDirName, fileName)}
}

// storedTokens is the on-disk form of [auth.Tokens]. It restates the fields
// on purpose: auth/ is exported to the public API repository, so putting JSON
// tags on [auth.Tokens] would make this file format part of that public
// interface and freeze it. Email is deliberately absent, because a stored
// email would go stale against the ID token beside it.
type storedTokens struct {
	IDToken      string    `json:"id_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry"`
}

func (s *fileTokenStore) Load() (*auth.Tokens, error) {
	contents, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("reading the token file %s: %w", s.path, err)
	}

	stored := storedTokens{}
	if err := json.Unmarshal(contents, &stored); err != nil {
		return nil, fmt.Errorf("parsing the token file %s: %w", s.path, err)
	}

	return &auth.Tokens{
		IDToken:      stored.IDToken,
		RefreshToken: stored.RefreshToken,
		Expiry:       stored.Expiry,
	}, nil
}

func (s *fileTokenStore) Save(tokens *auth.Tokens) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating the token directory %s: %w", dir, err)
	}

	contents, err := json.Marshal(storedTokens{
		IDToken:      tokens.IDToken,
		RefreshToken: tokens.RefreshToken,
		Expiry:       tokens.Expiry,
	})
	if err != nil {
		return fmt.Errorf("encoding the tokens: %w", err)
	}

	// os.CreateTemp makes the file with mode 0600. Write to it and then
	// rename it, so that a concurrent reader never sees a partial file.
	tmp, err := os.CreateTemp(dir, ".tokens-*")
	if err != nil {
		return fmt.Errorf("creating a temporary token file in %s: %w", dir, err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(contents); err != nil {
		tmp.Close()
		return fmt.Errorf("writing %s: %w", tmp.Name(), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", tmp.Name(), err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("renaming %s to %s: %w", tmp.Name(), s.path, err)
	}
	return nil
}

// Delete removes the token file. It succeeds when there is no file.
func (s *fileTokenStore) Delete() error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("deleting the token file %s: %w", s.path, err)
	}
	return nil
}

// httpClientFromMetadata returns the HTTP client that a test injects through
// the app metadata. It returns nil in production, which selects
// [http.DefaultClient].
func httpClientFromMetadata(appCtx *cli.Context) *http.Client {
	client, _ := appCtx.App.Metadata["oidcHTTPClient"].(*http.Client)
	return client
}

// oidcUserStrategy returns the oidc_user strategy of the profile. When the
// profile uses another strategy, the error gives the command that corrects
// it.
func oidcUserStrategy(setting *nbictlpb.Config) (*nbictlpb.Config_AuthStrategy_OidcUser, error) {
	strategy, ok := setting.GetAuthStrategy().GetType().(*nbictlpb.Config_AuthStrategy_OidcUser_)
	if !ok {
		return nil, fmt.Errorf("the profile %q does not use the oidc_user auth strategy. Run:\n  %s --profile=%s config set --auth_strategy oidc_user --issuer <ISSUER_URL> --client_id <CLIENT_ID>",
			setting.GetName(), appName, setting.GetName())
	}
	return strategy.OidcUser, nil
}

// profileStore reads the profile that the --profile flag selects, together
// with its token store. It does not require the oidc_user strategy, so that
// logout can still clean up a profile that has moved to another one.
func profileStore(appCtx *cli.Context) (*nbictlpb.Config, *fileTokenStore, error) {
	confDir, err := getAppConfDir(appCtx)
	if err != nil {
		return nil, nil, err
	}

	setting, err := readConfig(appCtx.String("profile"), filepath.Join(confDir, confFileName))
	if err != nil {
		return nil, nil, fmt.Errorf("unable to obtain context information: %w", err)
	}
	return setting, newFileTokenStore(confDir, setting.GetName()), nil
}

// oidcUserConfig builds the auth package's configuration for a profile's
// oidc_user strategy. It takes no *cli.Context, so that the dial path, which
// has none, builds the config the same way the login command does.
func oidcUserConfig(setting *nbictlpb.Config, oidcUser *nbictlpb.Config_AuthStrategy_OidcUser, httpClient *http.Client) auth.OIDCUserConfig {
	return auth.OIDCUserConfig{
		Clock:      clockwork.NewRealClock(),
		Issuer:     oidcUser.GetIssuer(),
		ClientID:   oidcUser.GetClientId(),
		LoginHint:  fmt.Sprintf("%s --profile=%s login", appName, setting.GetName()),
		HTTPClient: httpClient,
	}
}

// profileConfig reads the profile that the --profile flag selects, together
// with its oidc_user strategy and its token store.
func profileConfig(appCtx *cli.Context) (auth.OIDCUserConfig, *fileTokenStore, error) {
	setting, store, err := profileStore(appCtx)
	if err != nil {
		return auth.OIDCUserConfig{}, nil, err
	}

	oidcUser, err := oidcUserStrategy(setting)
	if err != nil {
		return auth.OIDCUserConfig{}, nil, err
	}
	return oidcUserConfig(setting, oidcUser, httpClientFromMetadata(appCtx)), store, nil
}

// Logout revokes the refresh token at the identity provider and then deletes
// the local token file. The deletion always runs, because local cleanup must
// not depend on the network.
func Logout(appCtx *cli.Context) error {
	setting, store, err := profileStore(appCtx)
	if err != nil {
		return err
	}

	tokens, err := store.Load()
	if err != nil {
		return err
	}
	if tokens == nil {
		fmt.Fprintln(appCtx.App.Writer, "Not logged in")
		return nil
	}

	errWriter := appCtx.App.ErrWriter

	// A profile that has moved to another strategy no longer holds the issuer
	// and the client ID that a revocation needs. Report that, and still delete
	// the file below, so that logout never leaves a live refresh token behind.
	var revokeErr error
	oidcUser, strategyErr := oidcUserStrategy(setting)
	switch {
	case tokens.RefreshToken == "":
		// There is nothing to revoke.
	case strategyErr != nil:
		fmt.Fprintf(errWriter, "Warning: the profile %q no longer uses the oidc_user auth strategy, so the refresh token was not revoked. End the session in the identity provider's console.\n", setting.GetName())
	default:
		revokeErr = auth.Revoke(appCtx.Context, oidcUserConfig(setting, oidcUser, httpClientFromMetadata(appCtx)), tokens.RefreshToken)
	}

	if err := store.Delete(); err != nil {
		return err
	}

	switch {
	case errors.Is(revokeErr, auth.ErrNoRevocationEndpoint):
		fmt.Fprintln(errWriter, "Warning: the identity provider advertises no revocation endpoint, so the refresh token stays valid. End the session in the identity provider's console.")
	case revokeErr != nil:
		fmt.Fprintf(errWriter, "Warning: the token revocation failed (%v), so the refresh token can still be valid. End the session in the identity provider's console.\n", revokeErr)
	}

	fmt.Fprintln(appCtx.App.Writer, "Logged out")
	return nil
}

// Login runs the OIDC device login for the selected profile and stores the
// tokens.
func Login(appCtx *cli.Context) error {
	oidcConfig, store, err := profileConfig(appCtx)
	if err != nil {
		return err
	}

	errWriter := appCtx.App.ErrWriter
	tokens, err := auth.DeviceLogin(appCtx.Context, oidcConfig, func(p auth.DevicePrompt) error {
		fmt.Fprintf(errWriter, "Open %s in a browser and enter the code %s\n", p.VerificationURI, p.UserCode)
		if p.VerificationURIComplete != "" {
			fmt.Fprintf(errWriter, "This link holds the code already: %s\n", p.VerificationURIComplete)
		}
		if !p.ExpiresAt.IsZero() {
			fmt.Fprintf(errWriter, "The code expires at %s\n", p.ExpiresAt.Format(time.RFC3339))
		}
		fmt.Fprintln(errWriter, "Waiting for the login to complete...")
		return nil
	})
	if err != nil {
		return err
	}

	if err := store.Save(tokens); err != nil {
		return err
	}

	if tokens.RefreshToken == "" {
		fmt.Fprintln(errWriter, "Warning: the identity provider returned no refresh token, so this session ends when the ID token expires. Grant the offline_access scope to keep the session.")
	}

	fmt.Fprintf(appCtx.App.Writer, "Logged in as %s, valid until %s\n", tokens.Email, tokens.Expiry.Format(time.RFC3339))
	return nil
}

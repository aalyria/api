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
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"

	"github.com/jonboulle/clockwork"
	"github.com/urfave/cli/v2"

	"aalyria.com/spacetime/auth"
	"aalyria.com/spacetime/tools/devicelogin"
	"aalyria.com/spacetime/tools/nbictl/nbictlpb"
	"aalyria.com/spacetime/tools/tokenfile"
)

const tokenDirName = "tokens"

// unsafeProfileChars matches every character that must not reach a file
// name. A profile name is user-chosen, so it can hold a path separator.
var unsafeProfileChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// newFileTokenStore returns the store of one profile's OIDC user tokens.
func newFileTokenStore(confDir, profileName string) *tokenfile.Store {
	return tokenfile.New(filepath.Join(confDir, tokenDirName, profileFileName(profileName, ".json")))
}

// profileFileName builds the name of a per-profile file in the token
// directory.
//
// Replacing the unsafe characters is not injective: "prod/x" and "prod_x" both
// reduce to "prod_x". The two profiles can name different issuers and
// different NBI hosts, so a shared file would send one profile's token to the
// other profile's endpoint. A digest of the original name keeps them apart.
//
// The suffix separates the kinds of token that one profile can hold. They are
// not interchangeable, so neither must ever be parsed as the other.
func profileFileName(profileName, suffix string) string {
	digest := sha256.Sum256([]byte(profileName))
	name := unsafeProfileChars.ReplaceAllString(profileName, "_")
	return fmt.Sprintf("%s-%x%s", name, digest[:4], suffix)
}

// httpClientFromMetadata returns the HTTP client that a test injects through
// the app metadata. In production the metadata holds none, and it returns
// [http.DefaultClient]. It never returns nil: the result reaches an interface
// field of the auth package, and a nil *http.Client in an interface is not a
// nil interface.
func httpClientFromMetadata(appCtx *cli.Context) *http.Client {
	client, _ := appCtx.App.Metadata["oidcHTTPClient"].(*http.Client)
	if client == nil {
		return http.DefaultClient
	}
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
func profileStore(appCtx *cli.Context) (*nbictlpb.Config, *tokenfile.Store, error) {
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
func profileConfig(appCtx *cli.Context) (auth.OIDCUserConfig, *tokenfile.Store, error) {
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
	return devicelogin.Login(appCtx.Context, oidcConfig, store, appCtx.App.Writer, appCtx.App.ErrWriter)
}

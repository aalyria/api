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
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/urfave/cli/v2"

	"aalyria.com/spacetime/auth"
	"aalyria.com/spacetime/tools/nbictl/nbictlpb"
)

func GenerateAuthToken(appCtx *cli.Context) error {
	ctxName := appCtx.String("context")
	confFile, err := getConfFileForContext(appCtx)
	if err != nil {
		return err
	}
	c, err := readConfig(ctxName, confFile)
	if err != nil {
		return fmt.Errorf("unable to obtain context information: %w", err)
	}

	var email, keyID string
	var pkeyBytes []byte

	switch t := c.GetAuthStrategy().GetType().(type) {
	case *nbictlpb.Config_AuthStrategy_None:
		return errors.New("auth_strategy is set to 'none'; cannot generate auth token")

	case *nbictlpb.Config_AuthStrategy_Jwt_:
		jwt := t.Jwt
		email = jwt.GetEmail()
		keyID = jwt.GetPrivateKeyId()
		if email == "" {
			return errors.New("no user_id set for chosen context")
		}
		if keyID == "" {
			return errors.New("no key_id set for chosen context")
		}
		if jwt.GetSigningStrategy() == nil {
			return errors.New("no priv_key set for chosen context")
		}
		switch s := jwt.GetSigningStrategy().GetType().(type) {
		case *nbictlpb.Config_SigningStrategy_PrivateKeyFile:
			pkeyBytes, err = os.ReadFile(s.PrivateKeyFile)
			if err != nil {
				return fmt.Errorf("unable to read the private key file: %w", err)
			}
		case *nbictlpb.Config_SigningStrategy_PrivateKeyBytes:
			pkeyBytes = s.PrivateKeyBytes
		default:
			return errors.New("no signing strategy (private key source) configured for JWT auth")
		}

	case *nbictlpb.Config_AuthStrategy_OidcClientCredentials_:
		oidc := t.OidcClientCredentials
		return generateOIDCToken(appCtx, oidc)

	default:
		// Backward compatibility: use deprecated fields.
		email = c.GetEmail()
		keyID = c.GetKeyId()
		if email == "" {
			return errors.New("no user_id set for chosen context")
		}
		if keyID == "" {
			return errors.New("no key_id set for chosen context")
		}
		if c.GetPrivKey() == "" {
			return errors.New("no priv_key set for chosen context")
		}
		pkeyBytes, err = os.ReadFile(c.GetPrivKey())
		if err != nil {
			return fmt.Errorf("unable to read the priv_key file: %w", err)
		}
	}

	pkeyBlock, _ := pem.Decode(pkeyBytes)
	if pkeyBlock == nil {
		return errors.New("PrivateKey not PEM-encoded")
	}
	pkey, err := auth.ParsePrivateKey(pkeyBlock.Bytes)
	if err != nil {
		return fmt.Errorf("error while parsing private key: %w", err)
	}

	now := time.Now()
	opts := auth.JWTOptions{
		Email:        email,
		PrivateKeyID: keyID,
		Audience:     appCtx.String("audience"),
		ExpiresAt:    now.Add(appCtx.Duration("expiration")),
		IssuedAt:     now,
	}

	tokenString, err := auth.CreateJWT(opts, pkey)
	if err != nil {
		return err
	}
	fmt.Fprintln(appCtx.App.Writer, tokenString)
	return nil
}

func generateOIDCToken(appCtx *cli.Context, oidc *nbictlpb.Config_AuthStrategy_OidcClientCredentials) error {
	if oidc.GetClientId() == "" {
		return errors.New("no client_id set for chosen context")
	}
	if oidc.GetTokenUrl() == "" {
		return errors.New("no token_url set for chosen context")
	}

	pkeyReader, err := readPrivateKeyFromSigningStrategy(oidc.GetSigningStrategy())
	if err != nil {
		return err
	}
	pkeyBytes, err := io.ReadAll(pkeyReader)
	if err != nil {
		return fmt.Errorf("reading private key: %w", err)
	}

	pkeyBlock, _ := pem.Decode(pkeyBytes)
	if pkeyBlock == nil {
		return errors.New("PrivateKey not PEM-encoded")
	}
	pkey, err := auth.ParsePrivateKey(pkeyBlock.Bytes)
	if err != nil {
		return fmt.Errorf("error while parsing private key: %w", err)
	}

	now := time.Now()
	assertion, err := auth.CreateJWT(auth.JWTOptions{
		Issuer:       oidc.GetClientId(),
		Subject:      oidc.GetClientId(),
		Audience:     oidc.GetTokenUrl(),
		PrivateKeyID: oidc.GetPrivateKeyId(),
		IssuedAt:     now,
		ExpiresAt:    now.Add(1 * time.Hour),
		JTI:          uuid.NewString(),
	}, pkey)
	if err != nil {
		return fmt.Errorf("creating OIDC assertion: %w", err)
	}

	form := url.Values{
		"grant_type":            {"client_credentials"},
		"client_id":             {oidc.GetClientId()},
		"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
		"client_assertion":      {assertion},
	}

	resp, err := http.Post(oidc.GetTokenUrl(), "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token endpoint returned status %d: %s", resp.StatusCode, body)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return fmt.Errorf("parsing token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return fmt.Errorf("token response missing access_token: %s", body)
	}

	fmt.Fprintln(appCtx.App.Writer, tokenResp.AccessToken)
	return nil
}

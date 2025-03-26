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
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/urfave/cli/v2"
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
	if c.GetEmail() == "" {
		return errors.New("no user_id set for chosen context")
	}
	if c.GetKeyId() == "" {
		return errors.New("no key_id set for chosen context")
	}
	if c.GetPrivKey() == "" {
		return errors.New("no priv_key set for chosen context")
	}
	pkeyBytes, err := os.ReadFile(c.GetPrivKey())
	if err != nil {
		return fmt.Errorf("unable to read the priv_key file: %w", err)
	}
	pkeyBlock, _ := pem.Decode(pkeyBytes)
	if pkeyBlock == nil {
		return errors.New("PrivateKey not PEM-encoded")
	}
	pkey, err := parsePrivateKey(pkeyBlock.Bytes)
	if err != nil {
		return fmt.Errorf("error while parsing priv_key file (%s): %w", c.GetPrivKey(), err)
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iss": c.GetEmail(),
		"sub": c.GetEmail(),
		"iat": jwt.NewNumericDate(now),
		"exp": jwt.NewNumericDate(now.Add(appCtx.Duration("expiration"))),
	}
	if aud := appCtx.String("audience"); aud != "" {
		claims["aud"] = aud
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = c.GetKeyId()
	tokenString, err := token.SignedString(pkey)
	if err != nil {
		return fmt.Errorf("signing auth token: %w", err)
	}
	fmt.Fprintln(appCtx.App.Writer, tokenString)
	return nil
}

func parsePrivateKey(data []byte) (any, error) {
	var pkey any
	ok := false
	parseErrs := []error{}
	for algName, parse := range map[string]func([]byte) (any, error){
		"pkcs1": func(d []byte) (any, error) {
			k, err := x509.ParsePKCS1PrivateKey(d)
			return any(k), err
		},
		"pkcs8": x509.ParsePKCS8PrivateKey,
	} {
		k, err := parse(data)
		if err != nil {
			parseErrs = append(parseErrs, fmt.Errorf("%s: %w", algName, err))
			continue
		}

		pkey = k
		ok = true
	}

	if !ok {
		return nil, errors.Join(parseErrs...)
	}
	return pkey, nil
}

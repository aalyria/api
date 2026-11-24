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

// Package devicelogin signs a user in to an OIDC identity provider from a
// command-line tool, with the OAuth 2.0 device authorization grant.
package devicelogin // import "aalyria.com/spacetime/tools/devicelogin"

import (
	"context"
	"fmt"
	"io"
	"time"

	"aalyria.com/spacetime/auth"
)

// Login runs the device login that config describes, and saves the tokens
// to store. It writes the instructions the user must follow, and any
// warning, to errOut, and the outcome to out, so that a user who redirects
// the output still sees the prompt.
func Login(ctx context.Context, config auth.OIDCUserConfig, store auth.TokenStore, out, errOut io.Writer) error {
	tokens, err := auth.DeviceLogin(ctx, config, prompt(errOut))
	if err != nil {
		return err
	}
	if err := store.Save(tokens); err != nil {
		return err
	}
	report(out, errOut, tokens)
	return nil
}

// prompt returns a function that writes the instructions for a device login
// to w, for [auth.DeviceLogin] to call.
func prompt(w io.Writer) func(auth.DevicePrompt) error {
	return func(p auth.DevicePrompt) error {
		fmt.Fprintf(w, "Open %s in a browser and enter the code %s\n", p.VerificationURI, p.UserCode)
		if p.VerificationURIComplete != "" {
			fmt.Fprintf(w, "This link holds the code already: %s\n", p.VerificationURIComplete)
		}
		if !p.ExpiresAt.IsZero() {
			fmt.Fprintf(w, "The code expires at %s\n", p.ExpiresAt.Format(time.RFC3339))
		}
		fmt.Fprintln(w, "Waiting for the login to complete...")
		return nil
	}
}

// report writes who logged in, and until when, to out. It warns on errOut
// when the identity provider returned no refresh token.
func report(out, errOut io.Writer, tokens *auth.Tokens) {
	if tokens.RefreshToken == "" {
		fmt.Fprintln(errOut, "Warning: the identity provider returned no refresh token, so this session ends when the ID token expires. Grant the offline_access scope to keep the session.")
	}
	fmt.Fprintf(out, "Logged in as %s, valid until %s\n", tokens.Email, tokens.Expiry.Format(time.RFC3339))
}

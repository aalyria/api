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

package devicelogin

import (
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"aalyria.com/spacetime/auth"
)

func TestPrompt(t *testing.T) {
	t.Parallel()

	expiresAt := time.Date(2026, time.November, 10, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		desc         string
		devicePrompt auth.DevicePrompt
		want         []string
	}{
		{
			desc: "every field",
			devicePrompt: auth.DevicePrompt{
				VerificationURI:         "https://issuer.example.com/device",
				VerificationURIComplete: "https://issuer.example.com/device?user_code=WXYZ-1234",
				UserCode:                "WXYZ-1234",
				ExpiresAt:               expiresAt,
			},
			want: []string{
				"Open https://issuer.example.com/device in a browser and enter the code WXYZ-1234",
				"This link holds the code already: https://issuer.example.com/device?user_code=WXYZ-1234",
				"The code expires at 2026-11-10T12:00:00Z",
				"Waiting for the login to complete...",
			},
		},
		{
			desc: "no complete URI and no expiry",
			devicePrompt: auth.DevicePrompt{
				VerificationURI: "https://issuer.example.com/device",
				UserCode:        "WXYZ-1234",
			},
			want: []string{
				"Open https://issuer.example.com/device in a browser and enter the code WXYZ-1234",
				"Waiting for the login to complete...",
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()

			var w strings.Builder
			if err := prompt(&w)(tc.devicePrompt); err != nil {
				t.Fatalf("prompt returned an unexpected error: %v", err)
			}
			if diff := cmp.Diff(tc.want, strings.Split(strings.TrimSuffix(w.String(), "\n"), "\n")); diff != "" {
				t.Errorf("prompt lines mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReport(t *testing.T) {
	t.Parallel()

	expiry := time.Date(2026, time.November, 10, 13, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		desc         string
		refreshToken string
		wantWarning  bool
	}{
		{desc: "with a refresh token", refreshToken: "refresh-token-1", wantWarning: false},
		{desc: "without a refresh token", refreshToken: "", wantWarning: true},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()

			var out, errOut strings.Builder
			report(&out, &errOut, &auth.Tokens{Email: "user@example.com", RefreshToken: tc.refreshToken, Expiry: expiry})

			if want := "Logged in as user@example.com, valid until 2026-11-10T13:00:00Z\n"; out.String() != want {
				t.Errorf("out = %q, want %q", out.String(), want)
			}
			if got := strings.Contains(errOut.String(), "no refresh token"); got != tc.wantWarning {
				t.Errorf("errOut = %q; warning present = %v, want %v", errOut.String(), got, tc.wantWarning)
			}
		})
	}
}

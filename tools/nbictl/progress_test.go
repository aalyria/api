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
	"testing"

	"github.com/urfave/cli/v2"
	nmtspb "outernetcouncil.org/nmts/v1/proto"
)

func TestShouldShowProgress(t *testing.T) {
	tests := []struct {
		name    string
		flagVal string
		want    bool
	}{
		{name: "on", flagVal: "on", want: true},
		{name: "off", flagVal: "off", want: false},
		{name: "auto_non_tty", flagVal: "auto", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got bool
			app := &cli.App{
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "progress", Value: "auto"},
				},
				Action: func(ctx *cli.Context) error {
					got = shouldShowProgress(ctx)
					return nil
				},
			}

			if err := app.Run([]string{"test", "--progress", tt.flagVal}); err != nil {
				t.Fatalf("app.Run: %v", err)
			}
			if got != tt.want {
				t.Errorf("shouldShowProgress(%q) = %v, want %v", tt.flagVal, got, tt.want)
			}
		})
	}
}

func TestModelSync_WithProgressOn(t *testing.T) {
	env := setupSyncTestEnv(t)

	env.writeEntityFile(t, "platforms.txtpb", `
entity { id: "platform-1" ek_platform {} }
entity { id: "platform-2" ek_platform {} }
relationship { a: "platform-1" z: "platform-2" kind: RK_CONTAINS }
`)

	if err := newTestApp().Run(env.syncArgs("--progress", "on")); err != nil {
		t.Fatalf("sync with --progress=on failed: %v", err)
	}

	if got := env.srv.EntityCount(); got != 2 {
		t.Errorf("EntityCount = %d, want 2", got)
	}
	if got := env.srv.RelationshipCount(); got != 1 {
		t.Errorf("RelationshipCount = %d, want 1", got)
	}
}

func TestModelSync_DeleteWithProgressOn(t *testing.T) {
	env := setupSyncTestEnv(t)

	env.srv.Seed(
		[]*nmtspb.Entity{
			{Id: "platform-1"},
			{Id: "platform-2"},
			{Id: "platform-3"},
		},
		[]*nmtspb.Relationship{
			{A: "platform-1", Z: "platform-2", Kind: nmtspb.RK_RK_CONTAINS},
		},
	)

	env.writeEntityFile(t, "platforms.txtpb", `
entity { id: "platform-1" ek_platform {} }
`)

	if err := newTestApp().Run(env.syncArgs("--delete", "--progress", "on")); err != nil {
		t.Fatalf("sync with --delete --progress=on failed: %v", err)
	}

	if got := env.srv.EntityCount(); got != 1 {
		t.Errorf("EntityCount = %d, want 1", got)
	}
	if got := env.srv.RelationshipCount(); got != 0 {
		t.Errorf("RelationshipCount = %d, want 0", got)
	}
}

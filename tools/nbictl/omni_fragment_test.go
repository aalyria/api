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
	"cmp"
	"context"
	"net"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/bazelbuild/rules_go/go/tools/bazel"
	gcmp "github.com/google/go-cmp/cmp"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	nmtspb "outernetcouncil.org/nmts/v1/proto"
	nmtsphypb "outernetcouncil.org/nmts/v1/proto/ek/physical"

	modelpb "aalyria.com/spacetime/api/model/v1"
	omnipb "aalyria.com/spacetime/tools/nbictl/omnipb"
)

func platformEntity(id string) *nmtspb.Entity {
	return &nmtspb.Entity{
		Id:   id,
		Kind: &nmtspb.Entity_EkPlatform{EkPlatform: &nmtsphypb.Platform{}},
	}
}

func sortEntities(entities []*nmtspb.Entity) {
	slices.SortFunc(entities, func(l, r *nmtspb.Entity) int { return cmp.Compare(l.GetId(), r.GetId()) })
}

// TestOmniFragmentAcceptsSingularAndPluralFields proves that the OmniFragment
// parsing used on every nbictl input accepts both the singular (`entity`,
// `relationship`) and pluralized (`entities`, `relationships`) field spellings,
// even when both appear within the same file, for every supported --format.
func TestOmniFragmentAcceptsSingularAndPluralFields(t *testing.T) {
	t.Parallel()

	want := &nmtspb.Fragment{
		// normalizeOmniFragment concatenates the singular list first, then
		// the pluralized list.
		Entity: []*nmtspb.Entity{
			platformEntity("ent-singular"),
			platformEntity("ent-plural"),
		},
		Relationship: []*nmtspb.Relationship{
			{A: "ent-singular", Kind: nmtspb.RK_RK_CONTAINS, Z: "ent-plural"},
			{A: "ent-plural", Kind: nmtspb.RK_RK_CONTAINS, Z: "ent-singular"},
		},
	}

	textContent := []byte(`
entity {
  id: "ent-singular"
  ek_platform {}
}
entities {
  id: "ent-plural"
  ek_platform {}
}
relationship {
  a: "ent-singular"
  kind: RK_CONTAINS
  z: "ent-plural"
}
relationships {
  a: "ent-plural"
  kind: RK_CONTAINS
  z: "ent-singular"
}
`)

	jsonContent := []byte(`{
  "entity": [{"id": "ent-singular", "ekPlatform": {}}],
  "entities": [{"id": "ent-plural", "ekPlatform": {}}],
  "relationship": [{"a": "ent-singular", "kind": "RK_CONTAINS", "z": "ent-plural"}],
  "relationships": [{"a": "ent-plural", "kind": "RK_CONTAINS", "z": "ent-singular"}]
}`)

	binaryContent, err := proto.Marshal(&omnipb.OmniFragment{
		Entity:        []*nmtspb.Entity{platformEntity("ent-singular")},
		Entities:      []*nmtspb.Entity{platformEntity("ent-plural")},
		Relationship:  []*nmtspb.Relationship{{A: "ent-singular", Kind: nmtspb.RK_RK_CONTAINS, Z: "ent-plural"}},
		Relationships: []*nmtspb.Relationship{{A: "ent-plural", Kind: nmtspb.RK_RK_CONTAINS, Z: "ent-singular"}},
	})
	checkErr(t, err)

	cases := []struct {
		format  string
		content []byte
	}{
		{"text", textContent},
		{"json", jsonContent},
		{"binary", binaryContent},
	}
	for _, tc := range cases {
		t.Run(tc.format, func(t *testing.T) {
			marshaller, err := marshallerForFormat(tc.format)
			checkErr(t, err)

			got, err := readNormalizedFragment(marshaller, tc.content)
			checkErr(t, err)

			if diff := gcmp.Diff(want, got, protocmp.Transform()); diff != "" {
				t.Errorf("[%s] normalized fragment mismatch (-want +got):\n%s", tc.format, diff)
			}
		})
	}
}

// configureModelTest creates a working directory, starts a fake Model server,
// and configures an nbictl context pointed at it. It returns the fake server
// and the config directory shared across the commands under test.
func configureModelTest(t *testing.T, ctx context.Context, g *errgroup.Group) (*FakeModelServer, string) {
	t.Helper()

	tmpDir, err := bazel.NewTmpDir("nbictl")
	checkErr(t, err)

	lis, err := net.Listen("tcp", ":0")
	checkErr(t, err)
	srv, err := startFakeModelServer(ctx, g, lis)
	checkErr(t, err)

	keys := generateKeysForTesting(t, tmpDir, "--org", "example org")
	checkErr(t, newTestApp().Run([]string{
		"nbictl", "--config_dir", tmpDir, "--context", "DEFAULT",
		"config", "set",
		"--transport_security", "insecure",
		"--user_id", "usr1",
		"--key_id", "key1",
		"--priv_key", keys.key,
		"--url", srv.listener.Addr().String(),
	}))

	return srv, tmpDir
}

// TestListEntitiesToSyncRoundTrip demonstrates that, for every --format, the
// raw output of `nbictl model-v1 list-entities` can be fed directly into
// `nbictl model-v1 sync` without any intermediate transformation of the file.
func TestListEntitiesToSyncRoundTrip(t *testing.T) {
	t.Parallel()

	for _, format := range []string{"text", "json", "binary"} {
		t.Run(format, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			g, ctx := errgroup.WithContext(ctx)
			defer func() { checkErr(t, g.Wait()) }()
			defer cancel()

			srv, tmpDir := configureModelTest(t, ctx, g)

			entities := []*nmtspb.Entity{
				platformEntity("uuid-1"),
				platformEntity("uuid-2"),
				platformEntity("uuid-3"),
			}

			argsPrefix := []string{"nbictl", "--config_dir", tmpDir, "--context", "DEFAULT"}

			marshaller, err := marshallerForFormat(format)
			checkErr(t, err)

			// Step 1: GetEntities (list-entities) and capture the output exactly
			// as it is written to stdout.
			srv.Reset()
			srv.ResponseMessage = &modelpb.ListEntitiesResponse{Entities: entities}
			listApp := newTestApp()
			checkErr(t, listApp.Run(append(argsPrefix, "model-v1", "list-entities", "--format", format)))
			listing := listApp.stdout.Bytes()

			// Persist the listing verbatim, with the extension `sync` discovers.
			listingFile := filepath.Join(tmpDir, "listing."+marshaller.fileExt)
			checkErr(t, os.WriteFile(listingFile, listing, 0o644))

			// Step 2: sync the captured file directly (no transformation). An
			// empty remote model means every listed entity should be upserted.
			srv.Reset()
			srv.ListEntitiesResp = &modelpb.ListEntitiesResponse{}
			srv.ListRelationshipsResp = &modelpb.ListRelationshipsResponse{}
			syncApp := newTestApp()
			checkErr(t, syncApp.Run(append(argsPrefix, "model-v1", "sync", "--format", format, listingFile)))

			srv.mu.Lock()
			got := append([]*nmtspb.Entity{}, srv.UpsertedEntities...)
			srv.mu.Unlock()

			want := append([]*nmtspb.Entity{}, entities...)
			sortEntities(got)
			sortEntities(want)
			if diff := gcmp.Diff(want, got, protocmp.Transform()); diff != "" {
				t.Errorf("[%s] synced entities mismatch (-want +got):\n%s", format, diff)
			}
		})
	}
}

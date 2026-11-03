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
	"slices"
	"strconv"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"

	nmtspb "outernetcouncil.org/nmts/v1/proto"
)

// pagedModelSize is chosen so that it does not divide evenly by
// pagedModelMaxPageSize, leaving a short last page.
const (
	pagedModelSize        = 25
	pagedModelMaxPageSize = 10
	pagedModelNumPages    = 3
)

// setupPagedModel seeds the remote with a model that only comes back whole if
// every page of it is read.
func setupPagedModel(t *testing.T) *syncTestEnv {
	t.Helper()

	env := setupSyncTestEnv(t)
	env.srv.SetMaxPageSize(pagedModelMaxPageSize)
	env.srv.Seed(generateSyntheticModel(pagedModelSize))
	return env
}

func TestModelListEntities_ReadsEveryPage(t *testing.T) {
	t.Parallel()

	for _, pageSize := range []int{0, pagedModelMaxPageSize, defaultCLIPageSize} {
		t.Run("page-size="+strconv.Itoa(pageSize), func(t *testing.T) {
			t.Parallel()

			env := setupPagedModel(t)
			app := newTestApp()
			args := slices.Concat(env.argsPrefix, []string{"model-v1", "list-entities", "--page-size", strconv.Itoa(pageSize)})
			if err := app.Run(args); err != nil {
				t.Fatalf("list-entities failed: %v", err)
			}

			got := &nmtspb.Fragment{}
			if err := prototext.Unmarshal(app.stdout.Bytes(), got); err != nil {
				t.Fatalf("parsing list-entities output: %v", err)
			}
			if n := len(got.GetEntity()); n != pagedModelSize {
				t.Errorf("listed %d entities, want %d", n, pagedModelSize)
			}
			if entityPages, _ := env.srv.ListPageCounts(); entityPages != pagedModelNumPages {
				t.Errorf("served %d ListEntities pages, want %d", entityPages, pagedModelNumPages)
			}
		})
	}
}

func TestModelListRelationships_ReadsEveryPage(t *testing.T) {
	t.Parallel()

	env := setupPagedModel(t)
	app := newTestApp()
	args := slices.Concat(env.argsPrefix, []string{"model-v1", "list-relationships"})
	if err := app.Run(args); err != nil {
		t.Fatalf("list-relationships failed: %v", err)
	}

	got := &nmtspb.Fragment{}
	if err := prototext.Unmarshal(app.stdout.Bytes(), got); err != nil {
		t.Fatalf("parsing list-relationships output: %v", err)
	}
	if n, want := len(got.GetRelationship()), pagedModelSize-1; n != want {
		t.Errorf("listed %d relationships, want %d", n, want)
	}
	if _, relationshipPages := env.srv.ListPageCounts(); relationshipPages != pagedModelNumPages {
		t.Errorf("served %d ListRelationships pages, want %d", relationshipPages, pagedModelNumPages)
	}
}

// A sync with --delete removes whatever it did not see remotely, so an element
// on a page it skipped would survive the sync.
func TestModelSync_DeleteMode_ReadsEveryPage(t *testing.T) {
	t.Parallel()

	env := setupPagedModel(t)
	// Keep only the platform, which sorts first and so is on the first page.
	env.writeEntityFile(t, "model.txtpb", `entity { id: "entity-0000" ek_platform {} }`)

	if err := newTestApp().Run(env.syncArgs("--delete")); err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if got := env.srv.EntityCount(); got != 1 {
		t.Errorf("EntityCount = %d, want 1", got)
	}
	if got := env.srv.RelationshipCount(); got != 0 {
		t.Errorf("RelationshipCount = %d, want 0", got)
	}
	if entityPages, relationshipPages := env.srv.ListPageCounts(); entityPages != pagedModelNumPages || relationshipPages != pagedModelNumPages {
		t.Errorf("served %d ListEntities and %d ListRelationships pages, want %d of each",
			entityPages, relationshipPages, pagedModelNumPages)
	}
}

// An unchanged remote must not be rewritten: a relationship on a skipped page
// would read as missing and be upserted again.
func TestModelSync_UnchangedModelAddsNoRelationships(t *testing.T) {
	t.Parallel()

	env := setupPagedModel(t)
	entities, relationships := generateSyntheticModel(pagedModelSize)
	if err := writeFragmentFiles(env.dataDir, entities, relationships, 1); err != nil {
		t.Fatalf("writing fragment files: %v", err)
	}

	if err := newTestApp().Run(env.syncArgs()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	for _, batch := range env.srv.UpsertBatches() {
		if batch.relationships != 0 {
			t.Errorf("sync upserted %d relationships the remote already had", batch.relationships)
		}
	}
}

func TestModelDeleteAll_ReadsEveryPage(t *testing.T) {
	t.Parallel()

	env := setupPagedModel(t)
	args := slices.Concat(env.argsPrefix, []string{"model-v1", "delete-all", "--execute"})
	if err := newTestApp().Run(args); err != nil {
		t.Fatalf("delete-all failed: %v", err)
	}
	if got := env.srv.EntityCount(); got != 0 {
		t.Errorf("EntityCount = %d, want 0", got)
	}
	if got := env.srv.RelationshipCount(); got != 0 {
		t.Errorf("RelationshipCount = %d, want 0", got)
	}
}

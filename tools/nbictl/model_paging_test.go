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
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/testing/protocmp"

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

// listCommand is a command that streams a model fragment to stdout.
type listCommand struct {
	name string
	// want returns the part of the synthetic model the command lists.
	want func(entities []*nmtspb.Entity, relationships []*nmtspb.Relationship) *nmtspb.Fragment
	// pages returns how many pages the command was served.
	pages func(srv *InMemoryModelServer) int
}

// listCommands are the commands that stream a model fragment to stdout, each
// with the part of the synthetic model it lists.
var listCommands = []listCommand{
	{
		name: "list-entities",
		want: func(entities []*nmtspb.Entity, _ []*nmtspb.Relationship) *nmtspb.Fragment {
			return &nmtspb.Fragment{Entity: entities}
		},
		pages: func(srv *InMemoryModelServer) int { n, _ := srv.ListPageCounts(); return n },
	},
	{
		name: "list-relationships",
		want: func(_ []*nmtspb.Entity, relationships []*nmtspb.Relationship) *nmtspb.Fragment {
			return &nmtspb.Fragment{Relationship: relationships}
		},
		pages: func(srv *InMemoryModelServer) int { _, n := srv.ListPageCounts(); return n },
	},
}

// listFormats are the --format values the list commands write.
var listFormats = []string{"text", "json", "binary"}

// forEachListCommand runs test as a parallel subtest for every list command in
// every format.
func forEachListCommand(t *testing.T, test func(t *testing.T, cmd listCommand, format string)) {
	t.Helper()

	for _, cmd := range listCommands {
		for _, format := range listFormats {
			t.Run(cmd.name+"/"+format, func(t *testing.T) {
				t.Parallel()
				test(t, cmd, format)
			})
		}
	}
}

// runListCommand runs a list command against env in the given format.
func runListCommand(app testApp, env *syncTestEnv, command, format string) error {
	return app.Run(slices.Concat(env.argsPrefix, []string{"model-v1", command, "--format", format}))
}

// parseListOutput reads list output back the way `sync` and `upsert-fragment`
// do.
func parseListOutput(format string, output []byte) (*nmtspb.Fragment, error) {
	marshaller, err := marshallerForFormat(format)
	if err != nil {
		return nil, err
	}
	return readNormalizedFragment(marshaller, output)
}

// listAndParse runs a list command against env, which must succeed, and
// returns its output read back as a Fragment.
func listAndParse(t *testing.T, env *syncTestEnv, command, format string) *nmtspb.Fragment {
	t.Helper()

	app := newTestApp()
	if err := runListCommand(app, env, command, format); err != nil {
		t.Fatalf("%s failed: %v", command, err)
	}
	got, err := parseListOutput(format, app.stdout.Bytes())
	if err != nil {
		t.Fatalf("parsing %s output: %v\n%s", command, err, app.stdout.String())
	}
	return got
}

// A page boundary falls between two elements of the same repeated field, and the
// last page is short, so each format has to join pages back into one Fragment.
func TestModelList_MultiPageOutputRoundTrips(t *testing.T) {
	t.Parallel()

	entities, relationships := generateSyntheticModel(pagedModelSize)
	forEachListCommand(t, func(t *testing.T, cmd listCommand, format string) {
		env := setupPagedModel(t)
		got := listAndParse(t, env, cmd.name, format)
		if diff := cmp.Diff(cmd.want(entities, relationships), got, protocmp.Transform()); diff != "" {
			t.Errorf("%s output mismatch (-want +got):\n%s", cmd.name, diff)
		}
		if pages := cmd.pages(env.srv); pages != pagedModelNumPages {
			t.Errorf("served %d pages, want %d", pages, pagedModelNumPages)
		}
	})
}

func TestModelList_EmptyModelRoundTrips(t *testing.T) {
	t.Parallel()

	forEachListCommand(t, func(t *testing.T, cmd listCommand, format string) {
		got := listAndParse(t, setupSyncTestEnv(t), cmd.name, format)
		if diff := cmp.Diff(&nmtspb.Fragment{}, got, protocmp.Transform()); diff != "" {
			t.Errorf("%s output mismatch (-want +got):\n%s", cmd.name, diff)
		}
	})
}

// A walk that fails after some pages have been written must not read as a
// smaller model: the command fails, says the output is incomplete, and the
// output written so far does not parse.
func TestModelList_LaterPageFailure(t *testing.T) {
	t.Parallel()

	forEachListCommand(t, func(t *testing.T, cmd listCommand, format string) {
		env := setupPagedModel(t)
		env.srv.FailListAfterPages(1)
		app := newTestApp()
		err := runListCommand(app, env, cmd.name, format)
		if err == nil {
			t.Fatalf("%s succeeded, want an error", cmd.name)
		}
		if !strings.Contains(err.Error(), "incomplete") {
			t.Errorf("%s error = %q, want it to say the output is incomplete", cmd.name, err)
		}
		// The first page was written before the failure.
		if !strings.Contains(app.stdout.String(), "entity-0001") {
			t.Errorf("%s output is missing the first page:\n%s", cmd.name, app.stdout.String())
		}
		if _, err := parseListOutput(format, app.stdout.Bytes()); err == nil {
			t.Errorf("%s output parsed, want truncated output to fail to parse:\n%s", cmd.name, app.stdout.String())
		}
	})
}

var errWriteFailed = errors.New("write failed")

// failingWriter fails every write, like a closed pipe.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errWriteFailed }

func TestModelList_WriteErrorStopsWalk(t *testing.T) {
	t.Parallel()

	forEachListCommand(t, func(t *testing.T, cmd listCommand, format string) {
		env := setupPagedModel(t)
		app := newTestApp()
		app.Writer = failingWriter{}
		err := runListCommand(app, env, cmd.name, format)
		if !errors.Is(err, errWriteFailed) {
			t.Fatalf("%s error = %v, want %v", cmd.name, err, errWriteFailed)
		}
		if pages := cmd.pages(env.srv); pages >= pagedModelNumPages {
			t.Errorf("served %d pages after the first write failed, want fewer than %d", pages, pagedModelNumPages)
		}
	})
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

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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	er "outernetcouncil.org/nmts/v1/lib/entityrelationship"
	nmtspb "outernetcouncil.org/nmts/v1/proto"
	nmtsphypb "outernetcouncil.org/nmts/v1/proto/ek/physical"
)

func (env *syncTestEnv) upsertScenarioArgs(extraFlags ...string) []string {
	return slices.Concat(env.argsPrefix, []string{"model-v1", "upsert-scenario", "-r"}, extraFlags, []string{env.dataDir})
}

func TestModelUpsertScenario_RelationshipsAcrossFiles(t *testing.T) {
	t.Parallel()

	env := setupSyncTestEnv(t)
	env.writeEntityFile(t, "platform.txtpb", `entity { id: "platform-1" ek_platform {} }`)
	env.writeEntityFile(t, "ports.txtpb", `
entity { id: "port-1" ek_port {} }
entities { id: "port-2" ek_port {} }
`)
	// Relationships can come before the entities they reference.
	env.writeEntityFile(t, "a-relationships.txtpb", `
relationship { a: "platform-1" z: "port-1" kind: RK_CONTAINS }
relationships { a: "platform-1" z: "port-2" kind: RK_CONTAINS }
`)
	env.writeEntityFile(t, "ignored.json", `{}`)

	app := newTestApp()
	if err := app.Run(env.upsertScenarioArgs()); err != nil {
		t.Fatalf("upsert-scenario failed: %v\nstderr:\n%s", err, app.stderr)
	}

	if got := env.srv.EntityCount(); got != 3 {
		t.Errorf("EntityCount = %d, want 3", got)
	}
	if got := env.srv.RelationshipCount(); got != 2 {
		t.Errorf("RelationshipCount = %d, want 2", got)
	}
	if want := "# OK: upserted 3 entities and 2 relationships from 3 files"; !strings.Contains(app.stderr.String(), want) {
		t.Errorf("stderr = %q, want it to contain %q", app.stderr, want)
	}
}

func TestModelUpsertScenario_LargeScenarioSpansMultipleBatches(t *testing.T) {
	t.Parallel()

	env := setupSyncTestEnv(t)

	const numEntities = 2500
	entities, relationships := generateSyntheticModel(numEntities)
	// Five files of 500 entities each, so entity batches are bounded by the
	// file, and relationships are batched across files.
	if err := writeFragmentFiles(env.dataDir, entities, relationships, 5); err != nil {
		t.Fatalf("writing fragment files: %v", err)
	}

	if err := newTestApp().Run(env.upsertScenarioArgs()); err != nil {
		t.Fatalf("upsert-scenario failed: %v", err)
	}

	if got := env.srv.EntityCount(); got != numEntities {
		t.Errorf("EntityCount = %d, want %d", got, numEntities)
	}
	if got := env.srv.RelationshipCount(); got != numEntities-1 {
		t.Errorf("RelationshipCount = %d, want %d", got, numEntities-1)
	}

	batches := env.srv.UpsertBatches()
	// 5 entity batches, then 2499 relationships at 1000 per request.
	if got, want := len(batches), 8; got != want {
		t.Fatalf("got %d UpsertFragment requests, want %d: %+v", got, want, batches)
	}
	seenRelationships := false
	for i, b := range batches {
		if b.entities > 0 && b.relationships > 0 {
			t.Errorf("batch %d mixes entities and relationships: %+v", i, b)
		}
		if b.entities+b.relationships > int(er.MaxFragmentUpsertElements) {
			t.Errorf("batch %d has %d elements, want at most %d", i, b.entities+b.relationships, er.MaxFragmentUpsertElements)
		}
		if b.relationships > 0 {
			seenRelationships = true
		} else if seenRelationships {
			t.Errorf("entity batch %d was applied after a relationship batch: %+v", i, batches)
		}
	}
}

func TestModelUpsertScenario_CheckOnlyUploadsNothing(t *testing.T) {
	t.Parallel()

	env := setupSyncTestEnv(t)
	env.writeEntityFile(t, "scenario.txtpb", `
entity { id: "platform-1" ek_platform {} }
entity { id: "port-1" ek_port {} }
relationship { a: "platform-1" z: "port-1" kind: RK_CONTAINS }
`)

	app := newTestApp()
	if err := app.Run(env.upsertScenarioArgs("--check-only")); err != nil {
		t.Fatalf("upsert-scenario --check-only failed: %v", err)
	}

	if got := env.srv.UpsertBatches(); len(got) != 0 {
		t.Errorf("UpsertBatches = %+v, want none", got)
	}
	if want := "# checked 1 files: 2 entities, 1 relationships"; !strings.Contains(app.stderr.String(), want) {
		t.Errorf("stderr = %q, want it to contain %q", app.stderr, want)
	}
}

func TestModelUpsertScenario_ProblemsUploadNothing(t *testing.T) {
	t.Parallel()

	env := setupSyncTestEnv(t)
	env.writeEntityFile(t, "a.txtpb", `
entity { id: "platform-1" ek_platform {} }
entity { id: "port-1" ek_port {} }
entity { id: "no-kind" }
relationship { a: "platform-1" z: "port-1" kind: RK_CONTAINS }
relationship { a: "platform-1" z: "missing" kind: RK_CONTAINS }
relationship { a: "port-1" z: "platform-1" kind: RK_CONTAINS }
`)
	env.writeEntityFile(t, "b.txtpb", `
entity { id: "port-1" ek_port {} }
relationship { a: "platform-1" z: "port-1" kind: RK_CONTAINS }
`)
	env.writeEntityFile(t, "c.txtpb", `entity { id: "broken" ek_port {} `)

	app := newTestApp()
	err := app.Run(env.upsertScenarioArgs())
	if err == nil {
		t.Fatal("upsert-scenario succeeded, want problems reported")
	}
	if want := "scenario check found 6 problem(s); nothing was uploaded"; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}

	stderr := app.stderr.String()
	for _, want := range []string{
		`a.txtpb: invalid entity "no-kind"`,
		`a.txtpb: invalid relationship {platform-1 missing RK_CONTAINS}`, // endpoint is not in the scenario
		`a.txtpb: invalid relationship {port-1 platform-1 RK_CONTAINS}`,  // port-1 cannot contain platform-1
		`b.txtpb: duplicate entity id "port-1" (also defined in `,
		`b.txtpb: duplicate relationship {platform-1 port-1 RK_CONTAINS}`,
		`c.txtpb: proto:`,
	} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr does not contain %q:\n%s", want, stderr)
		}
	}
	if got := env.srv.UpsertBatches(); len(got) != 0 {
		t.Errorf("UpsertBatches = %+v, want none", got)
	}
}

func TestModelUpsertScenario_JSONAndBinaryFormats(t *testing.T) {
	t.Parallel()

	fragment := &nmtspb.Fragment{
		Entity: []*nmtspb.Entity{
			{Id: "platform-1", Kind: &nmtspb.Entity_EkPlatform{EkPlatform: &nmtsphypb.Platform{}}},
			{Id: "port-1", Kind: &nmtspb.Entity_EkPort{EkPort: &nmtsphypb.Port{}}},
		},
		Relationship: []*nmtspb.Relationship{{A: "platform-1", Z: "port-1", Kind: nmtspb.RK_RK_CONTAINS}},
	}
	for _, tc := range []struct {
		format, filename string
		marshal          func(proto.Message) ([]byte, error)
	}{
		{format: "json", filename: "scenario.json", marshal: protojson.Marshal},
		{format: "binary", filename: "scenario.binpb", marshal: proto.Marshal},
	} {
		t.Run(tc.format, func(t *testing.T) {
			t.Parallel()

			env := setupSyncTestEnv(t)
			data, err := tc.marshal(fragment)
			if err != nil {
				t.Fatalf("marshalling fragment: %v", err)
			}
			env.writeEntityFile(t, tc.filename, string(data))
			// Files in other formats are ignored.
			env.writeEntityFile(t, "ignored.txtpb", `entity { id: "ignored" ek_port {} }`)

			app := newTestApp()
			if err := app.Run(env.upsertScenarioArgs("--format", tc.format)); err != nil {
				t.Fatalf("upsert-scenario failed: %v\nstderr:\n%s", err, app.stderr)
			}
			if got := env.srv.EntityCount(); got != 2 {
				t.Errorf("EntityCount = %d, want 2", got)
			}
			if got := env.srv.RelationshipCount(); got != 1 {
				t.Errorf("RelationshipCount = %d, want 1", got)
			}
		})
	}
}

func TestScenarioReader_MemoryBudgetLimitsFilesInFlight(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	var files []string
	for i := range 8 {
		path := filepath.Join(dir, fmt.Sprintf("%d.txtpb", i))
		if err := os.WriteFile(path, []byte(fmt.Sprintf(`entity { id: "port-%d" ek_port {} }`, i)), 0o644); err != nil {
			t.Fatalf("writing %s: %v", path, err)
		}
		files = append(files, path)
	}
	format, err := marshallerForFormat("text")
	if err != nil {
		t.Fatal(err)
	}

	// Every file is over a 1 byte budget, so each must be read on its own
	// despite there being 4 readers.
	reader := newScenarioReader(format, 1, 4)
	var inFlight, maxInFlight, numRead atomic.Int32
	err = reader.forEachFile(context.Background(), files, func(_ int, fragment *nmtspb.Fragment, readErr error) error {
		if readErr != nil {
			return readErr
		}
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			m := maxInFlight.Load()
			if n <= m || maxInFlight.CompareAndSwap(m, n) {
				break
			}
		}
		numRead.Add(int32(len(fragment.GetEntity())))
		return nil
	})
	if err != nil {
		t.Fatalf("forEachFile failed: %v", err)
	}
	if got := numRead.Load(); got != int32(len(files)) {
		t.Errorf("read %d entities, want %d", got, len(files))
	}
	if got := maxInFlight.Load(); got != 1 {
		t.Errorf("at most %d files were in flight at once, want 1", got)
	}
}

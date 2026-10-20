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
	"path/filepath"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	er "outernetcouncil.org/nmts/v1/lib/entityrelationship"
)

func (env *syncTestEnv) upsertFragmentArgs(filename string) []string {
	return slices.Concat(env.argsPrefix, []string{"model-v1", "upsert-fragment", filepath.Join(env.dataDir, filename)})
}

func TestModelUpsertFragment_SmallFragmentIsOneRequest(t *testing.T) {
	t.Parallel()

	env := setupSyncTestEnv(t)
	env.writeEntityFile(t, "fragment.txtpb", `
entity { id: "platform-1" ek_platform {} }
entity { id: "port-1" ek_port {} }
relationship { a: "platform-1" z: "port-1" kind: RK_CONTAINS }
`)

	if err := newTestApp().Run(env.upsertFragmentArgs("fragment.txtpb")); err != nil {
		t.Fatalf("upsert-fragment failed: %v", err)
	}

	if got := env.srv.EntityCount(); got != 2 {
		t.Errorf("EntityCount = %d, want 2", got)
	}
	if got := env.srv.RelationshipCount(); got != 1 {
		t.Errorf("RelationshipCount = %d, want 1", got)
	}
	want := []upsertBatch{{entities: 2, relationships: 1}}
	if diff := cmp.Diff(want, env.srv.UpsertBatches(), cmp.AllowUnexported(upsertBatch{})); diff != "" {
		t.Errorf("UpsertBatches mismatch (-want +got):\n%s", diff)
	}
}

func TestModelUpsertFragment_RejectedSmallFragmentWritesNothing(t *testing.T) {
	t.Parallel()

	env := setupSyncTestEnv(t)
	// The fragment is well formed, so only the server can reject it: the
	// relationship's z endpoint is in neither the fragment nor the remote model.
	env.writeEntityFile(t, "fragment.txtpb", `
entity { id: "platform-1" ek_platform {} }
relationship { a: "platform-1" z: "port-1" kind: RK_CONTAINS }
`)

	if err := newTestApp().Run(env.upsertFragmentArgs("fragment.txtpb")); err == nil {
		t.Fatal("upsert-fragment succeeded, want the server to reject the fragment")
	}

	if got := env.srv.EntityCount(); got != 0 {
		t.Errorf("EntityCount = %d, want 0", got)
	}
	if got := env.srv.UpsertBatches(); len(got) != 0 {
		t.Errorf("UpsertBatches = %+v, want none", got)
	}
}

func TestModelUpsertFragment_LargeFragmentSpansMultipleBatches(t *testing.T) {
	t.Parallel()

	env := setupSyncTestEnv(t)

	const numEntities = 2500
	entities, relationships := generateSyntheticModel(numEntities)
	if err := writeFragmentFiles(env.dataDir, entities, relationships, 1); err != nil {
		t.Fatalf("writing fragment file: %v", err)
	}

	if err := newTestApp().Run(env.upsertFragmentArgs("fragment-000.txtpb")); err != nil {
		t.Fatalf("upsert-fragment failed: %v", err)
	}

	if got := env.srv.EntityCount(); got != numEntities {
		t.Errorf("EntityCount = %d, want %d", got, numEntities)
	}
	if got := env.srv.RelationshipCount(); got != numEntities-1 {
		t.Errorf("RelationshipCount = %d, want %d", got, numEntities-1)
	}

	batches := env.srv.UpsertBatches()
	// 2500 entities and 2499 relationships at 1000 elements per request.
	if got, want := len(batches), 6; got != want {
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

func TestModelUpsertFragment_InvalidFragmentWritesNothing(t *testing.T) {
	t.Parallel()

	env := setupSyncTestEnv(t)

	entities, _ := generateSyntheticModel(1500)
	// A duplicate id that lands in a different batch from the original, so
	// only whole-fragment validation can catch it.
	entities = append(entities, entities[0])
	if err := writeFragmentFiles(env.dataDir, entities, nil, 1); err != nil {
		t.Fatalf("writing fragment file: %v", err)
	}

	if err := newTestApp().Run(env.upsertFragmentArgs("fragment-000.txtpb")); err == nil {
		t.Fatal("upsert-fragment succeeded, want a duplicate entity id error")
	}

	if got := env.srv.EntityCount(); got != 0 {
		t.Errorf("EntityCount = %d, want 0", got)
	}
	if got := env.srv.UpsertBatches(); len(got) != 0 {
		t.Errorf("UpsertBatches = %+v, want none", got)
	}
}

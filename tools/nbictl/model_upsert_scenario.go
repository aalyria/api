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
	"fmt"
	"os"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
	er "outernetcouncil.org/nmts/v1/lib/entityrelationship"
	"outernetcouncil.org/nmts/v1/lib/graph"
	"outernetcouncil.org/nmts/v1/lib/validation"
	nmtspb "outernetcouncil.org/nmts/v1/proto"

	modelpb "aalyria.com/spacetime/api/model/v1"
)

// maxReportedScenarioProblems caps how many problems the check pass prints; a
// systematic mistake in a large scenario can otherwise produce millions.
const maxReportedScenarioProblems = 100

// scenarioFileMemoryFactor is how many times its size on disk a fragment file
// is assumed to take in memory while it is read and parsed: the raw bytes, the
// parsed fragment, and garbage collector headroom. Measured peaks were 2.4x to
// 3.2x for text and JSON files and about 4x for binary files.
const scenarioFileMemoryFactor = 4

// ModelUpsertScenario upserts every fragment file found in the command line
// arguments as one scenario. Each file is read and parsed whole, but only as
// many files as fit in the --max-read-memory-mib budget are held at once, so
// the scenario as a whole never has to fit in memory. It runs in three phases:
//
//  1. Check: read every file and apply the same checks the server applies to
//     UpsertFragment, treating the scenario as self-contained. Only entity ids
//     and kinds and a compact form of every relationship are kept. Any problem
//     stops the command before anything is uploaded.
//  2. Read every file again and upsert its entities in size-bounded batches.
//  3. Upsert the relationships kept from the check phase, once every entity
//     batch has committed.
func ModelUpsertScenario(appCtx *cli.Context) error {
	maxConcurrency := appCtx.Int("max-concurrency")
	maxReadMemoryMiB := appCtx.Int64("max-read-memory-mib")
	showProgress := shouldShowProgress(appCtx)
	checkOnly := appCtx.Bool("check-only")
	if maxConcurrency < 1 {
		return fmt.Errorf("--max-concurrency must be at least 1, got %d", maxConcurrency)
	}
	if maxReadMemoryMiB < 1 {
		return fmt.Errorf("--max-read-memory-mib must be at least 1, got %d", maxReadMemoryMiB)
	}
	if !appCtx.Args().Present() {
		return fmt.Errorf("upsert-scenario needs at least one directory or filename argument")
	}

	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}
	files, err := findAllFilesWithExtension(marshaller.fileExt, appCtx.Bool("recursive"), appCtx.Args().Slice()...)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no *.%s files found in %v", marshaller.fileExt, appCtx.Args().Slice())
	}
	reader := newScenarioReader(marshaller, maxReadMemoryMiB<<20, min(runtime.NumCPU(), len(files)))

	checkProgress := newSyncProgress(showProgress)
	checkBar := checkProgress.AddBar("checking files", len(files))
	checkProgress.Start()
	index, err := checkScenario(appCtx.Context, files, reader, checkBar)
	checkProgress.Stop()
	if err != nil {
		return err
	}

	if index.problemCount > 0 {
		slices.Sort(index.reportedProblems)
		for _, p := range index.reportedProblems {
			fmt.Fprintln(appCtx.App.ErrWriter, p)
		}
		if extra := index.problemCount - len(index.reportedProblems); extra > 0 {
			fmt.Fprintf(appCtx.App.ErrWriter, "... and %d more\n", extra)
		}
		return fmt.Errorf("scenario check found %d problem(s); nothing was uploaded", index.problemCount)
	}
	fmt.Fprintf(appCtx.App.ErrWriter, "# checked %d files: %d entities, %d relationships\n",
		len(files), index.numEntities, len(index.relationships))
	if checkOnly {
		return nil
	}
	if index.numEntities == 0 && len(index.relationships) == 0 {
		return fmt.Errorf("scenario has no entities or relationships")
	}

	target, dialOpts, err := resolveAPIDialOpts(appCtx, serviceModel)
	if err != nil {
		return err
	}
	clients := make([]modelpb.ModelClient, maxConcurrency)
	for i := range maxConcurrency {
		conn, err := grpc.NewClient(target, dialOpts...)
		if err != nil {
			return fmt.Errorf("unable to connect to the server: %w", err)
		}
		defer conn.Close()
		clients[i] = modelpb.NewModelClient(conn)
	}
	var clientIdx atomic.Int64
	nextClient := func() modelpb.ModelClient {
		return clients[clientIdx.Add(1)%int64(maxConcurrency)]
	}

	applyProgress := newSyncProgress(showProgress)
	upsertEntitiesBar := applyProgress.AddBar("upserting entities     ", index.numEntities)
	upsertRelsBar := applyProgress.AddBar("upserting relationships", len(index.relationships))
	applyProgress.Start()

	err = upsertInBatches(appCtx.Context, maxConcurrency, nextClient, upsertEntitiesBar,
		func(ctx context.Context, send func(*nmtspb.Fragment) error) error {
			return readScenarioEntities(ctx, files, reader, send)
		})
	// Relationships are only sent once every entity batch has committed.
	if err == nil {
		err = upsertInBatches(appCtx.Context, maxConcurrency, nextClient, upsertRelsBar, index.sendRelationships)
	}
	applyProgress.Stop()
	if err != nil {
		return fmt.Errorf("upload failed partway; the remote model may be partially loaded (re-running is safe): %w", err)
	}

	fmt.Fprintf(appCtx.App.ErrWriter, "# OK: upserted %d entities and %d relationships from %d files\n",
		index.numEntities, len(index.relationships), len(files))
	return nil
}

// upsertInBatches runs produce, which passes size-bounded batches to send, and
// upserts each batch with up to maxConcurrency requests in flight. send blocks
// while every uploader is busy, so a producer never gets more than one batch
// ahead. The first failure cancels the producer and the other uploaders.
func upsertInBatches(
	ctx context.Context,
	maxConcurrency int,
	nextClient func() modelpb.ModelClient,
	bar *progressBar,
	produce func(ctx context.Context, send func(*nmtspb.Fragment) error) error,
) error {
	// ctx is canceled as soon as any goroutine below returns an error.
	g, ctx := errgroup.WithContext(ctx)
	// Unbuffered, so a send waits until an uploader is free to take the batch.
	batches := make(chan *nmtspb.Fragment)

	// Uploaders: each takes batches off the channel until it is closed, and
	// upserts them one at a time.
	for range maxConcurrency {
		g.Go(func() error {
			for batch := range batches {
				err := withRetryTimeout(ctx, upsertFragmentTimeout, func(ctx context.Context) error {
					_, err := nextClient().UpsertFragment(ctx, &modelpb.UpsertFragmentRequest{Fragment: batch})
					return err
				})
				if err != nil {
					return err
				}
				bar.IncrBy(len(batch.GetEntity()) + len(batch.GetRelationship()))
			}
			return nil
		})
	}

	// Producer: produce calls this send function with each batch. Closing
	// the channel when produce returns lets the uploaders finish.
	g.Go(func() error {
		defer close(batches)
		return produce(ctx, func(batch *nmtspb.Fragment) error {
			// Give up waiting for an uploader if one has already failed.
			select {
			case batches <- batch:
				return nil
			case <-ctx.Done():
				return context.Cause(ctx)
			}
		})
	})

	// Wait for the producer and every uploader; returns the first error.
	return g.Wait()
}

// scenarioReader reads and parses fragment files in parallel while keeping the
// memory the files being parsed take within a budget.
type scenarioReader struct {
	marshaller   protoFormat
	memory       *semaphore.Weighted
	memoryBudget int64
	numReaders   int
}

func newScenarioReader(marshaller protoFormat, memoryBudget int64, numReaders int) *scenarioReader {
	return &scenarioReader{
		marshaller:   marshaller,
		memory:       semaphore.NewWeighted(memoryBudget),
		memoryBudget: memoryBudget,
		numReaders:   numReaders,
	}
}

// forEachFile reads and parses files, up to numReaders at a time and in order,
// calling fn with the index of each file and either its fragment or the error
// reading it. Each file is charged scenarioFileMemoryFactor times its size
// against the memory budget until fn returns; a file over the whole budget is
// read on its own. The first error fn returns stops the remaining files.
func (r *scenarioReader) forEachFile(
	ctx context.Context,
	files []string,
	fn func(i int, fragment *nmtspb.Fragment, readErr error) error,
) error {
	// gctx is canceled as soon as any fn returns an error. g.Go blocks once
	// numReaders goroutines are running, which caps parallel parsing.
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(r.numReaders)
	for i, path := range files {
		info, err := os.Stat(path)
		if err != nil {
			// Let fn decide whether an unreadable file is fatal.
			g.Go(func() error { return fn(i, nil, err) })
			continue
		}
		// Wait here, before starting the goroutine, until this file fits in
		// the memory budget. Files are admitted in order, so a large file is
		// not passed over indefinitely by smaller ones. Acquire fails only if
		// gctx is canceled, so stop starting files.
		weight := min(info.Size()*scenarioFileMemoryFactor, r.memoryBudget)
		if err := r.memory.Acquire(gctx, weight); err != nil {
			break
		}
		g.Go(func() error {
			// Hold the file's share of the budget until fn is done with the
			// fragment, since that is when the file's memory can be freed.
			defer r.memory.Release(weight)
			data, err := os.ReadFile(path)
			if err != nil {
				return fn(i, nil, err)
			}
			fragment, err := readNormalizedFragment(r.marshaller, data)
			return fn(i, fragment, err)
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	// If no fn failed but the loop stopped early, the caller's ctx was
	// canceled. Check ctx, not gctx, which Wait always cancels.
	return context.Cause(ctx)
}

// readScenarioEntities reads every file and passes its entities to send in
// size-bounded batches. A batch never spans files.
func readScenarioEntities(ctx context.Context, files []string, reader *scenarioReader, send func(*nmtspb.Fragment) error) error {
	return reader.forEachFile(ctx, files, func(i int, fragment *nmtspb.Fragment, readErr error) error {
		if readErr != nil {
			return fmt.Errorf("%s: %w", files[i], readErr)
		}
		entities := &nmtspb.Fragment{Entity: fragment.GetEntity()}
		for _, batch := range er.SplitBySize(entities, er.MaxFragmentUpsertBytes, er.MaxFragmentUpsertElements) {
			if err := send(batch); err != nil {
				return err
			}
		}
		return nil
	})
}

// scenarioRelationship is a relationship held by the check pass. Endpoints are
// indexes into scenarioIndex.ids, and file is the index of the file it is in,
// for error messages.
type scenarioRelationship struct {
	a, z uint32
	kind nmtspb.RK
	file uint32
}

// scenarioIndex is everything the check pass keeps about a scenario: each
// entity id with its kind, and every relationship in compact form.
type scenarioIndex struct {
	files []string

	mu sync.Mutex
	// idIndexes and ids map entity ids to dense indexes and back. An id gets
	// an index the first time it is seen, whether as an entity or as a
	// relationship endpoint.
	idIndexes map[string]uint32
	ids       []string
	// kinds holds the field number of the Entity kind oneof field set on each
	// id's entity, or 0 if no entity has that id (so far).
	kinds []protoreflect.FieldNumber
	// entityFiles holds the file each id's entity is in. When an id is defined
	// in more than one file, it keeps the first file, so the duplicate
	// reported does not depend on the order files are read in.
	entityFiles      []uint32
	numEntities      int
	relationships    []scenarioRelationship
	reportedProblems []string
	problemCount     int
}

var entityKindOneof = (&nmtspb.Entity{}).ProtoReflect().Descriptor().Oneofs().ByName("kind")

func entityKindField(entity *nmtspb.Entity) protoreflect.FieldNumber {
	field := entity.ProtoReflect().WhichOneof(entityKindOneof)
	if field == nil {
		return 0
	}
	return field.Number()
}

// checkScenario reads every file and returns the index of the scenario, with
// any problems found recorded in it. A file that fails to read or parse is
// recorded as a problem rather than ending the check, so one run reports
// problems from every file.
func checkScenario(ctx context.Context, files []string, reader *scenarioReader, bar *progressBar) (*scenarioIndex, error) {
	index := &scenarioIndex{files: files, idIndexes: map[string]uint32{}}
	err := reader.forEachFile(ctx, files, func(i int, fragment *nmtspb.Fragment, readErr error) error {
		defer bar.Incr()
		if readErr != nil {
			index.mu.Lock()
			defer index.mu.Unlock()
			index.addProblem(fmt.Sprintf("%s: %v", files[i], readErr))
			return nil
		}
		for _, entity := range fragment.GetEntity() {
			index.addEntity(uint32(i), entity)
		}
		for _, relationship := range fragment.GetRelationship() {
			index.addRelationship(uint32(i), relationship)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	index.checkRelationships()
	return index, nil
}

// addProblem records a problem. index.mu must be held.
func (index *scenarioIndex) addProblem(problem string) {
	index.problemCount++
	if len(index.reportedProblems) < maxReportedScenarioProblems {
		index.reportedProblems = append(index.reportedProblems, problem)
	}
}

// intern returns the index for id, assigning one if it is new. index.mu must
// be held.
func (index *scenarioIndex) intern(id string) uint32 {
	if i, ok := index.idIndexes[id]; ok {
		return i
	}
	i := uint32(len(index.ids))
	index.idIndexes[id] = i
	index.ids = append(index.ids, id)
	index.kinds = append(index.kinds, 0)
	index.entityFiles = append(index.entityFiles, 0)
	return i
}

// addEntity applies the server's per-entity checks to entity and records its
// id and kind, reporting a duplicate if another entity already has the id.
func (index *scenarioIndex) addEntity(file uint32, entity *nmtspb.Entity) {
	wellFormedErr := validation.IsEntityMinimallyWellFormed(entity)
	antennaErr := validation.ValidateAntenna(entity)
	kind := entityKindField(entity)

	index.mu.Lock()
	defer index.mu.Unlock()
	location := index.files[file]
	if wellFormedErr != nil {
		index.addProblem(fmt.Sprintf("%s: invalid entity %q: %v", location, entity.GetId(), wellFormedErr))
		return
	}
	if antennaErr != nil {
		index.addProblem(fmt.Sprintf("%s: invalid entity %q: %v", location, entity.GetId(), antennaErr))
	}

	i := index.intern(entity.GetId())
	if index.kinds[i] == 0 {
		index.numEntities++
	} else {
		first := index.files[index.entityFiles[i]]
		if file >= index.entityFiles[i] {
			index.addProblem(fmt.Sprintf("%s: duplicate entity id %q (also defined in %s)", location, entity.GetId(), first))
			return
		}
		index.addProblem(fmt.Sprintf("%s: duplicate entity id %q (also defined in %s)", first, entity.GetId(), location))
	}
	index.kinds[i] = kind
	index.entityFiles[i] = file
}

// addRelationship applies the server's per-relationship checks to
// relationship and keeps it for checkRelationships and the upload.
func (index *scenarioIndex) addRelationship(file uint32, relationship *nmtspb.Relationship) {
	wellFormedErr := validation.IsRelationshipMinimallyWellFormed(relationship)

	index.mu.Lock()
	defer index.mu.Unlock()
	if wellFormedErr != nil {
		index.addProblem(fmt.Sprintf("%s: invalid relationship %s: %v",
			index.files[file], er.RelationshipFromProto(relationship), wellFormedErr))
		return
	}
	index.relationships = append(index.relationships, scenarioRelationship{
		a:    index.intern(relationship.GetA()),
		z:    index.intern(relationship.GetZ()),
		kind: relationship.GetKind(),
		file: file,
	})
}

func (index *scenarioIndex) relationshipProto(r scenarioRelationship) *nmtspb.Relationship {
	return &nmtspb.Relationship{A: index.ids[r.a], Z: index.ids[r.z], Kind: r.kind}
}

// relationshipRule is the input to the server's relationship check: the kinds
// of both endpoints and the relationship kind.
type relationshipRule struct {
	a    protoreflect.FieldNumber
	kind nmtspb.RK
	z    protoreflect.FieldNumber
}

// checkRelationships runs once every file has been read. It reports duplicate
// relationships, endpoints that no entity in the scenario defines, and
// relationship kinds the server does not permit between the endpoints' entity
// kinds. Of duplicate relationships, the ones in later files are reported. It
// sorts index.relationships, which also gives the upload a stable order.
func (index *scenarioIndex) checkRelationships() {
	slices.SortFunc(index.relationships, func(x, y scenarioRelationship) int {
		return cmp.Or(cmp.Compare(x.a, y.a), cmp.Compare(x.z, y.z), cmp.Compare(x.kind, y.kind),
			cmp.Compare(x.file, y.file))
	})

	// The server's check depends only on the endpoints' entity kinds and the
	// relationship kind, and a scenario has few distinct combinations, so
	// each permitted combination is only checked once.
	permitted := map[relationshipRule]bool{}
	validator := validation.DefaultGraphValidator{}

	for i, r := range index.relationships {
		location := index.files[r.file]
		relationship := er.RelationshipFromProto(index.relationshipProto(r))

		if i > 0 {
			prev := index.relationships[i-1]
			if prev.a == r.a && prev.z == r.z && prev.kind == r.kind {
				index.addProblem(fmt.Sprintf("%s: duplicate relationship %s", location, relationship))
				continue
			}
		}

		rule := relationshipRule{a: index.kinds[r.a], kind: r.kind, z: index.kinds[r.z]}
		if rule.a != 0 && rule.z != 0 && permitted[rule] {
			continue
		}

		// Check against a graph holding just the two endpoints, each a stub
		// entity of the right kind.
		g := graph.New()
		var err error
		for _, endpoint := range []uint32{r.a, r.z} {
			if err == nil && index.kinds[endpoint] != 0 {
				_, err = g.UpsertEntity(stubEntity(index.ids[endpoint], index.kinds[endpoint]))
			}
		}
		if err == nil {
			err = validator.ValidateRelationship(g, relationship)
		}
		if err != nil {
			index.addProblem(fmt.Sprintf("%s: invalid relationship %s: %v", location, relationship, err))
			continue
		}
		permitted[rule] = true
	}
}

// stubEntity returns an entity with the given id whose kind oneof is set to an
// empty message in the given field.
func stubEntity(id string, kind protoreflect.FieldNumber) *nmtspb.Entity {
	entity := &nmtspb.Entity{Id: id}
	m := entity.ProtoReflect()
	field := m.Descriptor().Fields().ByNumber(kind)
	m.Set(field, m.NewField(field))
	return entity
}

// sendRelationships passes every relationship to send in size-bounded
// batches. Relationships are converted back to protos one batch's worth at a
// time rather than all at once.
func (index *scenarioIndex) sendRelationships(_ context.Context, send func(*nmtspb.Fragment) error) error {
	for chunk := range slices.Chunk(index.relationships, int(er.MaxFragmentUpsertElements)) {
		relationships := &nmtspb.Fragment{Relationship: make([]*nmtspb.Relationship, len(chunk))}
		for i, r := range chunk {
			relationships.Relationship[i] = index.relationshipProto(r)
		}
		for _, batch := range er.SplitBySize(relationships, er.MaxFragmentUpsertBytes, er.MaxFragmentUpsertElements) {
			if err := send(batch); err != nil {
				return err
			}
		}
	}
	return nil
}

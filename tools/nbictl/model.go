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
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	set "github.com/deckarep/golang-set/v2"
	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	er "outernetcouncil.org/nmts/v1/lib/entityrelationship"
	nmtspb "outernetcouncil.org/nmts/v1/proto"

	modelpb "aalyria.com/spacetime/api/model/v1"
	omnipb "aalyria.com/spacetime/tools/nbictl/omnipb"
)

// upsertFragmentTimeout is larger than defaultRPCTimeout because each request
// carries a batch of entities and relationships.
const upsertFragmentTimeout = 90 * time.Second

// listPageTimeout is larger than defaultRPCTimeout because a single page can
// carry up to the server's maximum page size worth of entities.
const listPageTimeout = 10 * time.Minute

// listMaxRetryAttempts applies to listAllEntities and listAllRelationships, this
// dictates how many times a query for a specific page from storage can be retried
// before the query is aborted.
const listMaxRetryAttempts = 3

// normalizeOmniFragment collapses an OmniFragment, which permissively accepts
// both the singular (`entity`/`relationship`) and pluralized
// (`entities`/`relationships`) field spellings, into a single normalized native
// nmts.v1.Fragment. The singular and pluralized lists are concatenated so that
// inputs mixing both spellings - even within the same file - are accepted.
func normalizeOmniFragment(omni *omnipb.OmniFragment) *nmtspb.Fragment {
	fragment := &nmtspb.Fragment{}
	fragment.Entity = append(fragment.Entity, omni.GetEntity()...)
	fragment.Entity = append(fragment.Entity, omni.GetEntities()...)
	fragment.Relationship = append(fragment.Relationship, omni.GetRelationship()...)
	fragment.Relationship = append(fragment.Relationship, omni.GetRelationships()...)
	return fragment
}

// readNormalizedFragment reads an OmniFragment from the provided bytes using the
// given marshaller and returns the normalized native nmts.v1.Fragment.
func readNormalizedFragment(marshaller protoFormat, data []byte) (*nmtspb.Fragment, error) {
	omni := &omnipb.OmniFragment{}
	if err := marshaller.unmarshal(data, omni); err != nil {
		return nil, err
	}
	return normalizeOmniFragment(omni), nil
}

func readDataFromCommandLineFilenameArgument(appCtx *cli.Context) ([]byte, error) {
	if appCtx.Args().Len() != 1 {
		return nil, fmt.Errorf("need one and only one filename argument ('-' reads from stdin)")
	}

	fileName := appCtx.Args().First()
	if fileName == "-" {
		return io.ReadAll(appCtx.App.Reader)
	} else {
		return os.ReadFile(fileName)
	}
}

func readProtoFromCommandLineFilenameArgument[ProtoT proto.Message](appCtx *cli.Context, marshaller protoFormat, msg ProtoT) error {
	data, err := readDataFromCommandLineFilenameArgument(appCtx)
	if err != nil {
		return err
	}
	return marshaller.unmarshal(data, msg)
}

func ModelCreateEntity(appCtx *cli.Context) error {
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	nmtsEntity := &nmtspb.Entity{}
	if err := readProtoFromCommandLineFilenameArgument(appCtx, marshaller, nmtsEntity); err != nil {
		return err
	}

	conn, err := openAPIConnection(appCtx, serviceModel)
	if err != nil {
		return err
	}
	defer conn.Close()
	modelClient := modelpb.NewModelClient(conn)

	_, err = modelClient.CreateEntity(
		appCtx.Context,
		&modelpb.CreateEntityRequest{
			Entity: nmtsEntity,
		})
	if err == nil {
		fmt.Fprintln(appCtx.App.ErrWriter, "# OK")
	}
	return err
}

func ModelUpdateEntity(appCtx *cli.Context) error {
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	nmtsEntity := &nmtspb.Entity{}
	if err := readProtoFromCommandLineFilenameArgument(appCtx, marshaller, nmtsEntity); err != nil {
		return err
	}

	conn, err := openAPIConnection(appCtx, serviceModel)
	if err != nil {
		return err
	}
	defer conn.Close()
	modelClient := modelpb.NewModelClient(conn)

	_, err = modelClient.UpdateEntity(
		appCtx.Context,
		&modelpb.UpdateEntityRequest{
			Entity: nmtsEntity,
		})
	if err == nil {
		fmt.Fprintln(appCtx.App.ErrWriter, "# OK")
	}
	return err
}

func ModelDeleteEntity(appCtx *cli.Context) error {
	if appCtx.Args().Len() != 1 {
		return fmt.Errorf("need one and only one Entity ID argument")
	}

	conn, err := openAPIConnection(appCtx, serviceModel)
	if err != nil {
		return err
	}
	defer conn.Close()
	modelClient := modelpb.NewModelClient(conn)

	resp, err := modelClient.DeleteEntity(
		appCtx.Context,
		&modelpb.DeleteEntityRequest{
			EntityId: appCtx.Args().First(),
		})
	if err == nil {
		fmt.Fprintf(appCtx.App.ErrWriter, "# also deleted %d relationship/s\n", len(resp.DeletedRelationships))
	}
	return err
}

func ModelCreateRelationship(appCtx *cli.Context) error {
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	nmtsRelationship := &nmtspb.Relationship{}
	if err := readProtoFromCommandLineFilenameArgument(appCtx, marshaller, nmtsRelationship); err != nil {
		return err
	}

	conn, err := openAPIConnection(appCtx, serviceModel)
	if err != nil {
		return err
	}
	defer conn.Close()
	modelClient := modelpb.NewModelClient(conn)

	_, err = modelClient.CreateRelationship(
		appCtx.Context,
		&modelpb.CreateRelationshipRequest{
			Relationship: nmtsRelationship,
		})
	if err == nil {
		fmt.Fprintln(appCtx.App.ErrWriter, "# OK")
	}
	return err
}

func ModelDeleteRelationship(appCtx *cli.Context) error {
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	nmtsRelationship := &nmtspb.Relationship{}
	if err := readProtoFromCommandLineFilenameArgument(appCtx, marshaller, nmtsRelationship); err != nil {
		return err
	}

	conn, err := openAPIConnection(appCtx, serviceModel)
	if err != nil {
		return err
	}
	defer conn.Close()
	modelClient := modelpb.NewModelClient(conn)

	_, err = modelClient.DeleteRelationship(
		appCtx.Context,
		&modelpb.DeleteRelationshipRequest{
			Relationship: nmtsRelationship,
		})
	if err == nil {
		fmt.Fprintln(appCtx.App.ErrWriter, "# OK")
	}
	return err
}

func ModelUpsertFragment(appCtx *cli.Context) error {
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}
	data, err := readDataFromCommandLineFilenameArgument(appCtx)
	if err != nil {
		return err
	}
	nmtsFragment, err := readNormalizedFragment(marshaller, data)
	if err != nil {
		return err
	}

	conn, err := openAPIConnection(appCtx, serviceModel)
	if err != nil {
		return err
	}
	defer conn.Close()
	modelClient := modelpb.NewModelClient(conn)

	if _, err := modelClient.UpsertFragment(
		appCtx.Context,
		&modelpb.UpsertFragmentRequest{Fragment: nmtsFragment},
	); err != nil {
		return err
	}

	fmt.Fprintln(appCtx.App.ErrWriter, "# OK")
	return nil
}

func ModelGetEntity(appCtx *cli.Context) error {
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	if appCtx.Args().Len() != 1 {
		return fmt.Errorf("need one and only one Entity ID argument")
	}

	conn, err := openAPIConnection(appCtx, serviceModel)
	if err != nil {
		return err
	}
	defer conn.Close()
	modelClient := modelpb.NewModelClient(conn)

	response, err := modelClient.GetEntity(
		appCtx.Context,
		&modelpb.GetEntityRequest{
			EntityId: appCtx.Args().First(),
		})
	if err != nil {
		return err
	}

	marshalled, err := marshaller.marshal(response)
	if err != nil {
		return err
	}
	fmt.Fprint(appCtx.App.Writer, string(marshalled))
	return nil
}

// verboseLogf returns a progress logger for the paging walks. It writes to w,
// which must not be the stream carrying the marshalled fragment, and does
// nothing at all unless verbose is set.
func verboseLogf(w io.Writer, verbose bool) func(string, ...any) {
	if !verbose {
		return func(string, ...any) {}
	}
	var mu sync.Mutex
	return func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		fmt.Fprintf(w, format, args...)
	}
}

// listAllEntities walks through all pages from ListEntities and returns the entire model set. The service paginates its
// list endpoints, so the caller should read through all pages rather than read the entire model in a single call.
func listAllEntities(ctx context.Context, client modelpb.ModelClient, pageSize int32, logf func(string, ...any)) ([]*nmtspb.Entity, error) {
	var entities []*nmtspb.Entity

	logf("ListEntities returning up to %d entities per page\n", pageSize)
	request := &modelpb.ListEntitiesRequest{PageSize: pageSize}
	for pageNumber := 0; ; pageNumber++ {
		logf("Querying page number %d\n", pageNumber)
		var resp *modelpb.ListEntitiesResponse
		err := withRetryAttempts(ctx, listMaxRetryAttempts+1, defaultRetryBackoff, listPageTimeout,
			func(ctx context.Context) error {
				var err error
				resp, err = client.ListEntities(ctx, request)
				return err
			})
		if err != nil {
			return nil, err
		}

		entities = append(entities, resp.GetEntities()...)
		logf("Page %d returned %d entities.\n",
			pageNumber, len(resp.GetEntities()))
		if resp.GetNextPageToken() == "" {
			return entities, nil
		}

		// A cursor that fails to advance would walk the same page forever.
		// Returning what has been read so far would look like the whole model
		// to a caller that acts on it, so this fails instead.
		if resp.GetNextPageToken() == request.PageToken {
			return nil, fmt.Errorf(
				"ListEntities repeated the page token it was given after page %d (%d entities read); the walk cannot advance",
				pageNumber, len(entities))
		}
		request.PageToken = resp.GetNextPageToken()
	}
}

// listAllRelationships walks through all pages provided by ListRelationships and returns the entire model set. The
// service paginates its list endpoints, so the caller should read through all pages rather than read the entire model
// in a single call.
func listAllRelationships(ctx context.Context, client modelpb.ModelClient, filter string, pageSize int32, logf func(string, ...any)) ([]*nmtspb.Relationship, error) {
	var relationships []*nmtspb.Relationship

	logf("ListRelationships returning up to %d relationships per page\n", pageSize)
	request := &modelpb.ListRelationshipsRequest{Filter: filter, PageSize: pageSize}
	for pageNumber := 0; ; pageNumber++ {
		logf("Querying page number %d\n", pageNumber)
		var resp *modelpb.ListRelationshipsResponse
		err := withRetryAttempts(ctx, listMaxRetryAttempts+1, defaultRetryBackoff, listPageTimeout,
			func(ctx context.Context) error {
				var err error
				resp, err = client.ListRelationships(ctx, request)
				return err
			})
		if err != nil {
			return nil, err
		}

		relationships = append(relationships, resp.GetRelationships()...)
		logf("Page %d returned %d relationships.\n",
			pageNumber, len(resp.GetRelationships()))
		if resp.GetNextPageToken() == "" {
			return relationships, nil
		}
		// See listAllEntities: a cursor that fails to advance is an error
		// rather than a short result that reads as the whole model.
		if resp.GetNextPageToken() == request.PageToken {
			return nil, fmt.Errorf(
				"ListRelationships repeated the page token it was given after page %d (%d relationships read); the walk cannot advance",
				pageNumber, len(relationships))
		}
		request.PageToken = resp.GetNextPageToken()
	}
}

func ModelListEntities(appCtx *cli.Context) error {
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	conn, err := openAPIConnection(appCtx, serviceModel)
	if err != nil {
		return err
	}
	defer conn.Close()
	modelClient := modelpb.NewModelClient(conn)

	entities, err := listAllEntities(appCtx.Context, modelClient, int32(appCtx.Int("page-size")),
		verboseLogf(appCtx.App.ErrWriter, appCtx.Bool("verbose")))
	if err != nil {
		return err
	}

	// Emit a normalized native nmts.v1.Fragment so the output can be fed
	// directly back into `sync` or `upsert-fragment`.
	fragment := &nmtspb.Fragment{Entity: entities}
	marshalled, err := marshaller.marshal(fragment)
	if err != nil {
		return err
	}
	fmt.Fprint(appCtx.App.Writer, string(marshalled))
	return nil
}

func ModelListRelationships(appCtx *cli.Context) error {
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	conn, err := openAPIConnection(appCtx, serviceModel)
	if err != nil {
		return err
	}
	defer conn.Close()
	modelClient := modelpb.NewModelClient(conn)

	relationships, err := listAllRelationships(appCtx.Context, modelClient, "", int32(appCtx.Int("page-size")),
		verboseLogf(appCtx.App.ErrWriter, appCtx.Bool("verbose")))
	if err != nil {
		return err
	}

	// Emit a normalized native nmts.v1.Fragment so the output can be fed
	// directly back into `sync` or `upsert-fragment`.
	fragment := &nmtspb.Fragment{Relationship: relationships}
	marshalled, err := marshaller.marshal(fragment)
	if err != nil {
		return err
	}
	fmt.Fprint(appCtx.App.Writer, string(marshalled))
	return nil
}

func ModelDeleteAll(appCtx *cli.Context) error {
	dryRunMode := !appCtx.Bool("execute")
	verboseMode := appCtx.Bool("verbose")
	printMode := dryRunMode || verboseMode
	maxConcurrency := appCtx.Int("max-concurrency")
	showProgress := shouldShowProgress(appCtx)
	pageSize := int32(appCtx.Int("page-size"))

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

	listProgress := newSyncProgress(showProgress)
	listEntBar := listProgress.AddBar("listing remote entities     ", 1)
	listRelBar := listProgress.AddBar("listing remote relationships", 1)
	logf := verboseLogf(listProgress.Writer(), verboseMode)

	listProgress.Start()

	listPool := pool.New().WithErrors()

	var entityIDs []string
	listPool.Go(func() error {
		entities, err := listAllEntities(appCtx.Context, clients[0], pageSize, logf)
		if err != nil {
			return err
		}

		entityIDs = lo.Map(entities, func(item *nmtspb.Entity, _ int) string {
			return item.GetId()
		})

		listEntBar.Incr()
		return nil
	})
	var relationships []*nmtspb.Relationship
	listPool.Go(func() error {
		var err error
		relationships, err = listAllRelationships(appCtx.Context, clients[1%maxConcurrency], "", pageSize, logf)
		if err != nil {
			return err
		}

		listRelBar.Incr()
		return nil
	})

	errs := listPool.Wait()
	if errs != nil {
		listProgress.Stop()
		return errors.Join(errs)
	}
	listProgress.Stop()

	deleteProgress := newSyncProgress(showProgress)
	deleteRelsBar := deleteProgress.AddBar("deleting relationships", len(relationships))
	deleteEntsBar := deleteProgress.AddBar("deleting entities", len(entityIDs))

	deleteProgress.Start()
	defer deleteProgress.Stop()

	w := deleteProgress.Writer()

	refCount := make(map[string]*atomic.Int32, len(entityIDs))
	for _, id := range entityIDs {
		refCount[id] = &atomic.Int32{}
	}
	for _, rel := range relationships {
		if c, ok := refCount[rel.GetA()]; ok {
			c.Add(1)
		}
		if c, ok := refCount[rel.GetZ()]; ok {
			c.Add(1)
		}
	}

	// Buffer sized for every entity, since each is sent at most once. Sends
	// from relationship workers therefore never block, which is what prevents
	// the self-feeding deadlock the single-pool design had.
	entityCh := make(chan string, len(entityIDs))
	for id, refs := range refCount {
		if refs.Load() == 0 {
			entityCh <- id
		}
	}

	deleteEntity := func(id string) error {
		if printMode {
			fmt.Fprintf(w, "delete entity: %s\n", id)
		}
		var err error
		if !dryRunMode {
			err = withRetry(appCtx.Context, func(ctx context.Context) error {
				_, err := nextClient().DeleteEntity(ctx, &modelpb.DeleteEntityRequest{
					EntityId: id,
				})
				return err
			})
			if err != nil && isNotFoundError(err) {
				err = nil
			}
		}
		deleteEntsBar.Incr()
		return err
	}

	entityPool := pool.New().WithErrors()
	for range maxConcurrency {
		entityPool.Go(func() error {
			var errs []error
			for id := range entityCh {
				if err := deleteEntity(id); err != nil {
					errs = append(errs, err)
				}
			}
			return errors.Join(errs...)
		})
	}

	maybeDeleteEntity := func(entityId string) {
		c, ok := refCount[entityId]
		if !ok {
			return
		}
		if c.Add(-1) > 0 {
			return
		}
		entityCh <- entityId
	}

	relPool := pool.New().WithErrors().WithMaxGoroutines(maxConcurrency)
	for _, relationship := range relationships {
		relPool.Go(func() error {
			if printMode {
				fmt.Fprintf(w, "delete relationship: %s\n", relationship)
			}
			var err error
			if !dryRunMode {
				err = withRetry(appCtx.Context, func(ctx context.Context) error {
					_, err := nextClient().DeleteRelationship(ctx, &modelpb.DeleteRelationshipRequest{
						Relationship: relationship,
					})
					return err
				})
				if err != nil && isNotFoundError(err) {
					err = nil
				}
			}
			deleteRelsBar.Incr()
			if err == nil {
				maybeDeleteEntity(relationship.GetA())
				maybeDeleteEntity(relationship.GetZ())
			}
			return err
		})
	}

	relErr := relPool.Wait()
	close(entityCh)
	entityErr := entityPool.Wait()
	return errors.Join(relErr, entityErr)
}

func ModelSync(appCtx *cli.Context) error {
	deleteMode := appCtx.Bool("delete")
	dryRunMode := appCtx.Bool("dry-run")
	maxConcurrency := appCtx.Int("max-concurrency")
	showProgress := shouldShowProgress(appCtx)
	pageSize := int32(appCtx.Int("page-size"))
	verboseMode := appCtx.Bool("verbose")

	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	if !appCtx.Args().Present() {
		return fmt.Errorf("sync needs at least one directory or filename argument")
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

	// to speed things up, we read all files and issue remote RPCs in parallel
	// at the same time.
	localFiles, err := findAllFilesWithExtension(marshaller.fileExt, appCtx.Bool("recursive"), appCtx.Args().Slice()...)
	if err != nil {
		return err
	}

	readProgress := newSyncProgress(showProgress)
	readBar := readProgress.AddBar("reading local files         ", len(localFiles))
	listRelBar := readProgress.AddBar("listing remote relationships", 1)
	listEntBar := readProgress.AddBar("listing remote entities     ", 1)
	logf := verboseLogf(readProgress.Writer(), verboseMode)

	readProgress.Start()

	type parsedFragment struct {
		entities      []*nmtspb.Entity
		relationships []*nmtspb.Relationship
	}

	parsedFragments := make([]parsedFragment, len(localFiles))
	// Only the identities of the remote elements are needed, to diff against the
	// local ones; the bodies are never read.
	remoteEntityKeys := set.NewSet[string]()
	remoteRelationshipKeys := set.NewSet[er.Relationship]()

	initialReadPool := pool.New().WithErrors()

	// Read and parse local files concurrently.
	initialReadPool.Go(func() error {
		readPool := pool.New().WithErrors().WithMaxGoroutines(maxConcurrency)
		for i, localFile := range localFiles {
			readPool.Go(func() error {
				contents, err := os.ReadFile(localFile)
				if err != nil {
					return err
				}

				fragment, err := readNormalizedFragment(marshaller, contents)
				if err != nil {
					return err
				}

				parsedFragments[i] = parsedFragment{
					entities:      fragment.GetEntity(),
					relationships: fragment.GetRelationship(),
				}
				readBar.Incr()
				return nil
			})
		}
		return readPool.Wait()
	})

	// List remote entities.
	initialReadPool.Go(func() error {
		entities, err := listAllEntities(appCtx.Context, clients[0], pageSize, logf)
		if err != nil {
			return err
		}
		for _, entity := range entities {
			remoteEntityKeys.Add(entity.GetId())
		}
		listEntBar.Incr()
		return nil
	})

	// List remote relationships.
	initialReadPool.Go(func() error {
		relationships, err := listAllRelationships(appCtx.Context, clients[1%maxConcurrency], "", pageSize, logf)
		if err != nil {
			return err
		}
		for _, relationship := range relationships {
			remoteRelationshipKeys.Add(er.RelationshipFromProto(relationship))
		}
		listRelBar.Incr()
		return nil
	})

	if err = initialReadPool.Wait(); err != nil {
		readProgress.Stop()
		return err
	}
	readProgress.Stop()

	// Merge parsed fragments into local collections. The entity bodies are kept
	// because they are the UpsertFragment payload; local relationships, like the
	// remote ones, are only diffed.
	localEntities := map[string]*nmtspb.Entity{}
	localRelationshipKeys := set.NewSet[er.Relationship]()
	for _, pf := range parsedFragments {
		for _, entity := range pf.entities {
			localEntities[entity.GetId()] = entity
		}
		for _, relationship := range pf.relationships {
			localRelationshipKeys.Add(er.RelationshipFromProto(relationship))
		}
	}

	if len(localEntities) == 0 && !deleteMode {
		return fmt.Errorf("no local entities to sync to remote instance and --delete is false")
	}
	if localRelationshipKeys.IsEmpty() {
		fmt.Fprintf(appCtx.App.ErrWriter, "# Warning: no local relationship to sync to remote instance")
	}
	localEntityKeys := set.NewSetFromMapKeys(localEntities)

	// Step 3: compute and print/enact differences.
	var deleteRelsTotal int
	if deleteMode {
		deleteRelsTotal = remoteRelationshipKeys.Difference(localRelationshipKeys).Cardinality()
	}

	deleteEntitiesTotal := 0
	if deleteMode {
		deleteEntitiesTotal = remoteEntityKeys.Difference(localEntityKeys).Cardinality()
	}

	addRelsTotal := localRelationshipKeys.Difference(remoteRelationshipKeys).Cardinality()

	applyProgress := newSyncProgress(showProgress)
	deleteRelsBar := applyProgress.AddBar("deleting relationships", deleteRelsTotal)
	upsertEntitiesBar := applyProgress.AddBar("upserting entities", len(localEntities))
	deleteEntitiesBar := applyProgress.AddBar("deleting entities", deleteEntitiesTotal)
	addRelsBar := applyProgress.AddBar("adding relationships", addRelsTotal)

	applyProgress.Start()
	defer applyProgress.Stop()

	errs := []error{}

	// Delete relationships before deleting entities.
	deleteRelationshipsPool := pool.New().WithErrors().WithMaxGoroutines(maxConcurrency)
	if deleteMode {
		relationshipsToBeDeleted := remoteRelationshipKeys.Difference(localRelationshipKeys)
		for relationship := range relationshipsToBeDeleted.Iter() {
			deleteRelationshipsPool.Go(func() error {
				var err error
				if !dryRunMode {
					err = withRetry(appCtx.Context, func(ctx context.Context) error {
						_, err := nextClient().DeleteRelationship(ctx, &modelpb.DeleteRelationshipRequest{
							Relationship: relationship.ToProto(),
						})
						return err
					})
				}
				deleteRelsBar.Incr()
				return err
			})
		}
	}
	// Wait for relationship deletes before adding entities and
	// relationships that might create difficult-to-analyze graphs.
	errs = append(errs, deleteRelationshipsPool.Wait())

	// Upsert all local entities via batched UpsertFragment calls, then delete
	// remote-only entities per element (delete is not batched). Batches are built
	// from the deduplicated localEntities map -- never the raw parsedFragments --
	// so UpsertFragment never sees a duplicate entity id.
	upsertPool := pool.New().WithErrors().WithMaxGoroutines(maxConcurrency)

	entitiesFragment := &nmtspb.Fragment{Entity: make([]*nmtspb.Entity, 0, len(localEntities))}
	for _, entity := range localEntities {
		entitiesFragment.Entity = append(entitiesFragment.Entity, entity)
	}
	for _, batch := range er.SplitBySize(entitiesFragment, er.MaxFragmentUpsertBytes, er.MaxFragmentUpsertElements) {
		upsertPool.Go(func() error {
			var err error
			if !dryRunMode {
				err = withRetryTimeout(appCtx.Context, upsertFragmentTimeout, func(ctx context.Context) error {
					_, err := nextClient().UpsertFragment(ctx, &modelpb.UpsertFragmentRequest{Fragment: batch})
					return err
				})
			}
			// Advance by the committed batch's element count only on success, so
			// the bar never shows partial-within-batch progress and stops short
			// on failure.
			if err == nil {
				upsertEntitiesBar.IncrBy(len(batch.GetEntity()))
			}
			return err
		})
	}

	if deleteMode {
		entitiesToBeDeleted := remoteEntityKeys.Difference(localEntityKeys)
		for entityID := range entitiesToBeDeleted.Iter() {
			upsertPool.Go(func() error {
				var err error
				if !dryRunMode {
					err = withRetry(appCtx.Context, func(ctx context.Context) error {
						_, err := nextClient().DeleteEntity(ctx, &modelpb.DeleteEntityRequest{
							EntityId: entityID,
						})
						return err
					})
					if err != nil && isNotFoundError(err) {
						err = nil
					}
				}
				deleteEntitiesBar.Incr()
				return err
			})
		}
	}

	errs = append(errs, upsertPool.Wait())

	// Add relationships via batched UpsertFragment calls. Only the delta
	// (local - remote) is sent, built from the deduplicated relationship set.
	// The upsertPool.Wait() above is the entities-before-relationships barrier:
	// every entity a delta relationship references is already committed.
	addRelationshipsPool := pool.New().WithErrors().WithMaxGoroutines(maxConcurrency)
	relationshipsToBeAdded := localRelationshipKeys.Difference(remoteRelationshipKeys)
	relsFragment := &nmtspb.Fragment{Relationship: make([]*nmtspb.Relationship, 0, relationshipsToBeAdded.Cardinality())}
	for relationship := range relationshipsToBeAdded.Iter() {
		relsFragment.Relationship = append(relsFragment.Relationship, relationship.ToProto())
	}
	for _, batch := range er.SplitBySize(relsFragment, er.MaxFragmentUpsertBytes, er.MaxFragmentUpsertElements) {
		addRelationshipsPool.Go(func() error {
			var err error
			if !dryRunMode {
				err = withRetryTimeout(appCtx.Context, upsertFragmentTimeout, func(ctx context.Context) error {
					_, err := nextClient().UpsertFragment(ctx, &modelpb.UpsertFragmentRequest{Fragment: batch})
					return err
				})
			}
			if err == nil {
				addRelsBar.IncrBy(len(batch.GetRelationship()))
			}
			return err
		})
	}
	errs = append(errs, addRelationshipsPool.Wait())

	return errors.Join(errs...)
}

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
	"fmt"
	"io"
	"os"

	set "github.com/deckarep/golang-set/v2"
	"github.com/samber/lo"
	"github.com/sourcegraph/conc/pool"
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	er "outernetcouncil.org/nmts/v1/lib/entityrelationship"
	nmtspb "outernetcouncil.org/nmts/v1/proto"

	modelpb "aalyria.com/spacetime/api/model/v1"
)

const modelAPISubDomain = "model-v1"

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

	conn, err := openAPIConnection(appCtx, modelAPISubDomain)
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

	conn, err := openAPIConnection(appCtx, modelAPISubDomain)
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

	conn, err := openAPIConnection(appCtx, modelAPISubDomain)
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

	conn, err := openAPIConnection(appCtx, modelAPISubDomain)
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

	conn, err := openAPIConnection(appCtx, modelAPISubDomain)
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

// TODO: turn these into one atomic RPC call in the modelfe.
func ModelUpsertFragment(appCtx *cli.Context) error {
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}
	nmtsFragment := &nmtspb.Fragment{}
	if err := readProtoFromCommandLineFilenameArgument(appCtx, marshaller, nmtsFragment); err != nil {
		return err
	}

	conn, err := openAPIConnection(appCtx, modelAPISubDomain)
	if err != nil {
		return err
	}
	defer conn.Close()
	modelClient := modelpb.NewModelClient(conn)

	for _, nmtsEntity := range nmtsFragment.GetEntity() {
		_, err = modelClient.CreateEntity(
			appCtx.Context,
			&modelpb.CreateEntityRequest{
				Entity: nmtsEntity,
			})
		if err != nil {
			return err
		}
	}

	for _, nmtsRelationship := range nmtsFragment.GetRelationship() {
		_, err = modelClient.CreateRelationship(
			appCtx.Context,
			&modelpb.CreateRelationshipRequest{
				Relationship: nmtsRelationship,
			})
		if err != nil {
			return err
		}
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

	conn, err := openAPIConnection(appCtx, modelAPISubDomain)
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

func ModelListEntities(appCtx *cli.Context) error {
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	conn, err := openAPIConnection(appCtx, modelAPISubDomain)
	if err != nil {
		return err
	}
	defer conn.Close()
	modelClient := modelpb.NewModelClient(conn)

	response, err := modelClient.ListEntities(appCtx.Context, &modelpb.ListEntitiesRequest{})
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

func ModelListRelationships(appCtx *cli.Context) error {
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	conn, err := openAPIConnection(appCtx, modelAPISubDomain)
	if err != nil {
		return err
	}
	defer conn.Close()
	modelClient := modelpb.NewModelClient(conn)

	response, err := modelClient.ListRelationships(appCtx.Context, &modelpb.ListRelationshipsRequest{})
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

func nmtsEntitiesAreEquivalent(a, b *nmtspb.Entity) bool {
	// TODO: find a more robust equivalency check.
	// Additionally, allow for not overwriting certain changes,
	// like not changing interface admin status (because this
	// can be changed in the UI), and perhaps not changing
	// a KeplerianElements if the only change is the epoch
	// time (just wastes a lot of motion computation).
	return proto.Equal(a, b)
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	st, ok := status.FromError(err)
	if !ok {
		// Not a gRPC status error
		return false
	}

	// Check if the code is NotFound (code 5)
	return st.Code() == codes.NotFound
}

func ModelDeleteAll(appCtx *cli.Context) error {
	dryRunMode := !appCtx.Bool("execute")
	verboseMode := appCtx.Bool("verbose")
	printMode := dryRunMode || verboseMode

	conn, err := openAPIConnection(appCtx, modelAPISubDomain)
	if err != nil {
		return err
	}
	defer conn.Close()
	modelClient := modelpb.NewModelClient(conn)

	listPool := pool.New().WithErrors()

	var entityIds []string
	listPool.Go(func() error {
		entityList, err := modelClient.ListEntities(appCtx.Context, &modelpb.ListEntitiesRequest{})
		if err != nil {
			return err
		}

		entityIds = lo.Map(entityList.Entities, func(item *nmtspb.Entity, _ int) string {
			return item.GetId()
		})

		return nil
	})
	var relationships []*nmtspb.Relationship
	listPool.Go(func() error {
		relationshipList, err := modelClient.ListRelationships(appCtx.Context, &modelpb.ListRelationshipsRequest{})
		if err != nil {
			return err
		}

		relationships = relationshipList.GetRelationships()
		return nil
	})

	errs := listPool.Wait()
	if errs != nil {
		return errors.Join(errs)
	}

	deleteRelationshipsPool := pool.New().WithErrors()
	for _, relationship := range relationships {
		deleteRelationshipsPool.Go(func() error {
			if printMode {
				fmt.Printf("delete relationship: %s\n", relationship)
			}
			var err error
			if !dryRunMode {
				_, err = modelClient.DeleteRelationship(appCtx.Context, &modelpb.DeleteRelationshipRequest{
					Relationship: relationship,
				})
				if err != nil && isNotFoundError(err) {
					err = nil
				}
			}
			return err
		})
	}

	errs = deleteRelationshipsPool.Wait()
	if errs != nil {
		return errors.Join(errs)
	}

	deleteEntitiesPool := pool.New().WithErrors()
	for _, entityId := range entityIds {
		deleteEntitiesPool.Go(func() error {
			if printMode {
				fmt.Printf("delete entity: %s\n", entityId)
			}
			var err error
			if !dryRunMode {
				_, err = modelClient.DeleteEntity(appCtx.Context, &modelpb.DeleteEntityRequest{
					EntityId: entityId,
				})
				if err != nil && isNotFoundError(err) {
					err = nil
				}
			}
			return err
		})
	}

	errs = deleteEntitiesPool.Wait()

	return errors.Join(errs)
}

func ModelSync(appCtx *cli.Context) error {
	deleteMode := appCtx.Bool("delete")
	dryRunMode := appCtx.Bool("dry-run")
	verboseMode := appCtx.Bool("verbose")
	maxConcurrency := appCtx.Int("max-concurrency")
	printMode := dryRunMode || verboseMode

	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	if !appCtx.Args().Present() {
		return fmt.Errorf("sync needs at least one directory or filename argument")
	}

	// Step 1: load up all the local entity and relationship elements.
	localEntities := map[string]*nmtspb.Entity{}
	localRelationships := er.NewRelationshipSet()

	localFiles, err := findAllFilesWithExtension(marshaller.fileExt, appCtx.Bool("recursive"), appCtx.Args().Slice()...)
	if err != nil {
		return err
	}
	for _, localFile := range localFiles {
		fmt.Fprintf(os.Stdout, "reading %s\n", localFile)
		contents, err := os.ReadFile(localFile)
		if err != nil {
			return err
		}

		fragment := &nmtspb.Fragment{}
		err = marshaller.unmarshal(contents, fragment)
		if err != nil {
			return err
		}

		for _, entity := range fragment.GetEntity() {
			localEntities[entity.GetId()] = entity
		}
		for _, relationship := range fragment.GetRelationship() {
			localRelationships.Insert(er.RelationshipFromProto(relationship))
		}
	}

	if len(localEntities) == 0 {
		return fmt.Errorf("no local entities to sync to remote instance")
	}
	if len(localRelationships.Relations) == 0 {
		fmt.Fprintf(os.Stderr, "# Warning: no local relationship to sync to remote instance")
	}
	localEntityKeys := set.NewSetFromMapKeys(localEntities)
	localRelationshipKeys := set.NewSetFromMapKeys(localRelationships.Relations)
	fmt.Fprintf(os.Stdout, "local model elements:\n")
	fmt.Fprintf(os.Stdout, "- %d NMTS Entities\n", localEntityKeys.Cardinality())
	fmt.Fprintf(os.Stdout, "- %d NMTS Relationships\n", localRelationshipKeys.Cardinality())

	// Step 2: load up all the remote instance entity and relationship elements.
	remoteEntities := map[string]*nmtspb.Entity{}
	remoteRelationships := er.NewRelationshipSet()

	conn, err := openAPIConnection(appCtx, modelAPISubDomain)
	if err != nil {
		return err
	}
	defer conn.Close()
	modelClient := modelpb.NewModelClient(conn)

	listPool := pool.New().WithErrors()

	listPool.Go(func() error {
		entityList, err := modelClient.ListEntities(appCtx.Context, &modelpb.ListEntitiesRequest{})
		if err != nil {
			return err
		}
		for _, entity := range entityList.GetEntities() {
			remoteEntities[entity.GetId()] = entity
		}
		return nil
	})

	listPool.Go(func() error {
		relationshipList, err := modelClient.ListRelationships(appCtx.Context, &modelpb.ListRelationshipsRequest{})
		if err != nil {
			return err
		}
		for _, relationship := range relationshipList.GetRelationships() {
			remoteRelationships.Insert(er.RelationshipFromProto(relationship))
		}
		return nil
	})

	if err = listPool.Wait(); err != nil {
		return err
	}
	remoteEntityKeys := set.NewSetFromMapKeys(remoteEntities)
	remoteRelationshipKeys := set.NewSetFromMapKeys(remoteRelationships.Relations)
	fmt.Fprintf(os.Stdout, "remote model elements:\n")
	fmt.Fprintf(os.Stdout, "- %d NMTS Entities\n", remoteEntityKeys.Cardinality())
	fmt.Fprintf(os.Stdout, "- %d NMTS Relationships\n", remoteRelationshipKeys.Cardinality())

	// Step 3: compute and print/enact differences.
	errs := []error{}

	// Delete relationships before deleting entities.
	deleteRelationshipsPool := pool.New().WithErrors().WithMaxGoroutines(maxConcurrency)
	if deleteMode {
		relationshipsToBeDeleted := remoteRelationshipKeys.Difference(localRelationshipKeys)
		for relationship := range relationshipsToBeDeleted.Iter() {
			deleteRelationshipsPool.Go(func() error {
				if printMode {
					fmt.Printf("delete relationship: %s\n", relationship)
				}
				var err error
				if !dryRunMode {
					_, err = modelClient.DeleteRelationship(appCtx.Context, &modelpb.DeleteRelationshipRequest{
						Relationship: relationship.ToProto(),
					})
				}
				return err
			})
		}
	}
	// Wait for relationship deletes before adding entities and
	// relationships that might create difficult-to-analyze graphs.
	errs = append(errs, deleteRelationshipsPool.Wait())

	// Update entities.
	updatePool := pool.New().WithErrors().WithMaxGoroutines(maxConcurrency)
	entitiesInCommon := localEntityKeys.Intersect(remoteEntityKeys)
	for entity := range entitiesInCommon.Iter() {
		updatePool.Go(func() error {
			if nmtsEntitiesAreEquivalent(localEntities[entity], remoteEntities[entity]) {
				return nil
			}
			if printMode {
				fmt.Printf("update entity: %s\n", entity)
			}
			var err error
			if !dryRunMode {
				_, err = modelClient.UpdateEntity(appCtx.Context, &modelpb.UpdateEntityRequest{
					Entity: localEntities[entity],
				})
			}
			return err
		})
	}

	// Maybe delete entities (and prune collaterally deleted relationships)
	if deleteMode {
		entitiesToBeDeleted := remoteEntityKeys.Difference(localEntityKeys)

		for entity := range entitiesToBeDeleted.Iter() {
			updatePool.Go(func() error {
				if printMode {
					fmt.Printf("delete entity: %s\n", entity)
				}
				var err error
				if !dryRunMode {
					_, err = modelClient.DeleteEntity(appCtx.Context, &modelpb.DeleteEntityRequest{
						EntityId: entity,
					})
					if err != nil && isNotFoundError(err) {
						err = nil
					}
				}
				return err
			})
		}
	}

	errs = append(errs, updatePool.Wait())

	// Add entities.
	addEntitiesPool := pool.New().WithErrors().WithMaxGoroutines(maxConcurrency)
	entitiesToBeAdded := localEntityKeys.Difference(remoteEntityKeys)
	for entity := range entitiesToBeAdded.Iter() {
		addEntitiesPool.Go(func() error {
			if printMode {
				fmt.Printf("add entity: %s\n", entity)
			}
			var err error
			if !dryRunMode {
				_, err = modelClient.CreateEntity(appCtx.Context, &modelpb.CreateEntityRequest{
					Entity: localEntities[entity],
				})
			}
			return err
		})
	}
	// Need to wait for entities to be added before adding any
	// relationships that might reference them.
	errs = append(errs, addEntitiesPool.Wait())

	// Add relationships.
	addRelationshipsPool := pool.New().WithErrors().WithMaxGoroutines(maxConcurrency)
	relationshipsToBeAdded := localRelationshipKeys.Difference(remoteRelationshipKeys)
	for relationship := range relationshipsToBeAdded.Iter() {
		addRelationshipsPool.Go(func() error {
			if printMode {
				fmt.Printf("add relationship: %s\n", relationship)
			}
			var err error
			if !dryRunMode {
				_, err = modelClient.CreateRelationship(appCtx.Context, &modelpb.CreateRelationshipRequest{
					Relationship: relationship.ToProto(),
				})
			}
			return err
		})
	}
	errs = append(errs, addRelationshipsPool.Wait())

	return errors.Join(errs...)
}

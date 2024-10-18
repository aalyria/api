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
	"fmt"
	"io"
	"os"

	modelpb "aalyria.com/spacetime/api/model/v1alpha"
	"github.com/urfave/cli/v2"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	nmtspb "outernetcouncil.org/nmts/proto"
)

const modelAPISubDomain = "model"

func prettyPrintProto[ProtoT proto.Message](appCtx *cli.Context, msg ProtoT) error {
	txt, err := prototext.MarshalOptions{Multiline: true}.Marshal(msg)
	if err != nil {
		return err
	}
	fmt.Fprint(appCtx.App.Writer, string(txt))
	return nil
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

func readProtoFromCommandLineFilenameArgument[ProtoT proto.Message](appCtx *cli.Context, msg ProtoT) error {
	data, err := readDataFromCommandLineFilenameArgument(appCtx)
	if err != nil {
		return err
	}
	return prototext.Unmarshal(data, msg)
}

func ModelUpsertEntity(appCtx *cli.Context) error {
	nmtsEntity := &nmtspb.Entity{}
	if err := readProtoFromCommandLineFilenameArgument(appCtx, nmtsEntity); err != nil {
		return err
	}

	conn, err := openAPIConnection(appCtx, modelAPISubDomain)
	if err != nil {
		return err
	}
	defer conn.Close()
	modelClient := modelpb.NewModelClient(conn)

	_, err = modelClient.UpsertEntity(
		appCtx.Context,
		&modelpb.UpsertEntityRequest{
			Entity: nmtsEntity,
		})
	if err == nil {
		fmt.Fprintln(appCtx.App.ErrWriter, "# OK")
	}
	return err
}

func ModelUpdateEntity(appCtx *cli.Context) error {
	nmtsPartialEntity := &nmtspb.PartialEntity{}
	if err := readProtoFromCommandLineFilenameArgument(appCtx, nmtsPartialEntity); err != nil {
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
			Patch: nmtsPartialEntity,
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

func ModelInsertRelationship(appCtx *cli.Context) error {
	nmtsRelationship := &nmtspb.Relationship{}
	if err := readProtoFromCommandLineFilenameArgument(appCtx, nmtsRelationship); err != nil {
		return err
	}

	conn, err := openAPIConnection(appCtx, modelAPISubDomain)
	if err != nil {
		return err
	}
	defer conn.Close()
	modelClient := modelpb.NewModelClient(conn)

	_, err = modelClient.InsertRelationship(
		appCtx.Context,
		&modelpb.InsertRelationshipRequest{
			Relationship: nmtsRelationship,
		})
	if err == nil {
		fmt.Fprintln(appCtx.App.ErrWriter, "# OK")
	}
	return err
}

func ModelDeleteRelationship(appCtx *cli.Context) error {
	nmtsRelationship := &nmtspb.Relationship{}
	if err := readProtoFromCommandLineFilenameArgument(appCtx, nmtsRelationship); err != nil {
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

func ModelGetEntity(appCtx *cli.Context) error {
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

	prettyPrintProto(appCtx, response)
	debugPrintNMTSEntityEdges(appCtx.App.ErrWriter, response.GetEntityEdges())
	return nil
}

func debugPrintNMTSEntityEdges(stderr io.Writer, entityEdges *nmtspb.EntityEdges) {
	numEntities, numRelationships := 0, 0
	if entityEdges != nil {
		numEntities, numRelationships = 1, len(entityEdges.Relationship)
	}
	fmt.Fprintf(stderr, "# %v entity/ies, %v relationship/s\n", numEntities, numRelationships)
}

func ModelListElements(appCtx *cli.Context) error {
	conn, err := openAPIConnection(appCtx, modelAPISubDomain)
	if err != nil {
		return err
	}
	defer conn.Close()
	modelClient := modelpb.NewModelClient(conn)

	response, err := modelClient.ListElements(appCtx.Context, &modelpb.ListElementsRequest{})
	if err != nil {
		return err
	}

	prettyPrintProto(appCtx, response)
	debugPrintNMTSFragment(appCtx.App.ErrWriter, response.GetElements())
	return nil
}

func debugPrintNMTSFragment(stderr io.Writer, fragment *nmtspb.Fragment) {
	numEntities, numRelationships := 0, 0
	if fragment != nil {
		numEntities, numRelationships = len(fragment.Entity), len(fragment.Relationship)
	}
	fmt.Fprintf(stderr, "# %v entity/ies, %v relationship/s\n", numEntities, numRelationships)
}

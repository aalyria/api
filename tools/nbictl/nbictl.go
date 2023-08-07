// Copyright 2023 Aalyria Technologies, Inc., and its affiliates.
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
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pb "aalyria.com/spacetime/api/nbi/v1alpha"
	"google.golang.org/protobuf/encoding/prototext"
)

var typeList = generateTypeList()

const (
	clientName = "nbictl"

	// if oidcURL is empty, the Spacetime OAUTH library will use its default URL
	oidcURLDefault = ""
)

func Create(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet(clientName+" create", flag.ExitOnError)
	contextName := fs.String("context", "", "name of context you want to use")
	files := fs.String("files", "", "[REQUIRED] a `path` to the textproto file containing information of the entity you want to create")
	fs.Parse(args)

	if *files == "" {
		return errors.New("--files required")
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("unable to obtain the default config directory: %w", err)
	}
	filePath := filepath.Join(configDir, clientName, confFileName)

	setting, err := GetContext(*contextName, filePath)
	if err != nil {
		return fmt.Errorf("unable to obtain context information: %w", err)
	}
	conn, err := OpenConnection(ctx, setting)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pb.NewNetOpsClient(conn)

	textprotoFiles, err := filepath.Glob(*files)
	if err != nil {
		return fmt.Errorf("unable to expand the file path: %w", err)
	} else if len(textprotoFiles) == 0 {
		return fmt.Errorf("no files found under the given file path: %s", *files)
	}

	var entities []*pb.Entity
	for _, textProtoFile := range textprotoFiles {
		entity := &pb.Entity{}
		if err := readFromFile(textProtoFile, entity); err != nil {
			return fmt.Errorf("error while parsing create entity request for file %s: %w", textProtoFile, err)
		}
		entities = append(entities, entity)
	}

	for idx, entity := range entities {
		entityType := entity.Group.GetType().String()
		createEntityRequest := &pb.CreateEntityRequest{Type: &entityType, Entity: entity}

		res, err := client.CreateEntity(ctx, createEntityRequest)
		if err != nil {
			return fmt.Errorf("unable to create an entity: %w", err)
		}
		protoMessage, err := prototext.MarshalOptions{Multiline: true}.Marshal(res)
		if err != nil {
			return fmt.Errorf("unable to convert the response into textproto format: %w", err)
		}
		fmt.Println(string(protoMessage))
		fmt.Fprintf(os.Stderr, "entity successfully created!:\nid: %s commit_timestamp: %d type: %v file_location: %s\n",
			*res.Id, *res.CommitTimestamp, res.GetGroup().GetType(), textprotoFiles[idx])
	}
	return nil
}

func Update(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet(clientName+" update", flag.ExitOnError)
	contextName := fs.String("context", "", "name of context you want to use")
	files := fs.String("files", "", "[REQUIRED] a `path` to the textproto file containing information of the entity you want to update")
	fs.Parse(args)

	if *files == "" {
		return errors.New("--files required")
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("unable to obtain the default config directory: %w", err)
	}
	filePath := filepath.Join(configDir, clientName, confFileName)

	setting, err := GetContext(*contextName, filePath)
	if err != nil {
		return fmt.Errorf("unable to obtain context information: %w", err)
	}

	conn, err := OpenConnection(ctx, setting)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pb.NewNetOpsClient(conn)

	textprotoFiles, err := filepath.Glob(*files)
	if err != nil {
		return fmt.Errorf("unable to expand the file path %w", err)
	} else if len(textprotoFiles) == 0 {
		return fmt.Errorf("no files found under the given file path: %s", *files)
	}

	var entities []*pb.Entity

	for _, textProtoFile := range textprotoFiles {
		entity := &pb.Entity{}

		if err := readFromFile(textProtoFile, entity); err != nil {
			return fmt.Errorf("error while parsing update entity for file %s: %w", textProtoFile, err)
		}
		entities = append(entities, entity)
	}

	for idx, entity := range entities {

		entityType := entity.Group.GetType().String()
		entityID := entity.GetId()
		updateEntityRequest := &pb.UpdateEntityRequest{Type: &entityType, Id: &entityID, Entity: entity}
		res, err := client.UpdateEntity(ctx, updateEntityRequest)
		if err != nil {
			return fmt.Errorf("unable to update the entity: %w", err)
		}

		protoMessage, err := prototext.MarshalOptions{Multiline: true}.Marshal(res)
		if err != nil {
			return fmt.Errorf("unable to convert the response into textproto format: %w", err)
		}
		fmt.Println(string(protoMessage))
		fmt.Fprintf(os.Stderr, "update successful:\n id: %s commit_timestamp: %d type: %v file_location: %s\n", *res.Id, *res.CommitTimestamp, res.GetGroup().GetType(), textprotoFiles[idx])
	}

	return nil
}

func Delete(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet(clientName+" delete", flag.ExitOnError)
	entityType := fs.String("type", "", fmt.Sprintf("[REQUIRED] type of entities you want to delete. list of possible types: %v", typeList))
	id := fs.String("id", "", "[REQUIRED] the id of the entity you want to delete")
	commitTime := fs.Int64("commit_time", -1, "[REQUIRED] commit timestamp of the entity you want to delete")
	contextName := fs.String("context", "", "name of context you want to use")

	fs.Parse(args)
	switch {
	case *entityType == "":
		return errors.New("--type required")
	case *id == "":
		return errors.New("--id required")
	case *commitTime == -1:
		return errors.New("--commit_time required")
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("unable to obtain the default config directory: %w", err)
	}
	filePath := filepath.Join(configDir, clientName, confFileName)

	setting, err := GetContext(*contextName, filePath)
	if err != nil {
		return fmt.Errorf("unable to obtain context information: %w", err)
	}

	conn, err := OpenConnection(ctx, setting)
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pb.NewNetOpsClient(conn)

	deleteEntityRequest := &pb.DeleteEntityRequest{Type: entityType, Id: id, CommitTimestamp: commitTime}

	if _, err := client.DeleteEntity(ctx, deleteEntityRequest); err != nil {
		return fmt.Errorf("unable to delete the entity: %w", err)
	}
	fmt.Fprintln(os.Stderr, "deletion successful")
	return nil
}

func List(ctx context.Context, args []string) error {
	list := flag.NewFlagSet(clientName+" list", flag.ExitOnError)
	contextName := list.String("context", "", "name of context you want to use")
	listType := list.String("type", "", fmt.Sprintf("[REQUIRED] type of entities you want to query. list of possible types: %v", typeList))
	list.Parse(args)

	if *listType == "" {
		return errors.New("--type required")
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("unable to obtain the default config directory: %w", err)
	}
	filePath := filepath.Join(configDir, clientName, confFileName)

	setting, err := GetContext(*contextName, filePath)
	if err != nil {
		return fmt.Errorf("unable to obtain context information: %w", err)
	}

	conn, err := OpenConnection(ctx, setting)
	if err != nil {
		return err
	}

	client := pb.NewNetOpsClient(conn)

	if _, exists := pb.EntityType_value[*listType]; !exists {
		return fmt.Errorf("unknown entity type %q is not one of [%s]", *listType, strings.Join(typeList, ", "))
	}

	res, err := client.ListEntities(ctx, &pb.ListEntitiesRequest{Type: listType})
	if err != nil {
		return fmt.Errorf("unable to list entities: %w", err)
	}
	protoMessage, err := prototext.MarshalOptions{Multiline: true}.Marshal(res)
	if err != nil {
		return fmt.Errorf("unable to convert the response into textproto format: %w", err)
	}
	fmt.Println(string(protoMessage))
	fmt.Fprintf(os.Stderr, "successfully queried a list of entities. number of entities: %d\n", len(res.Entities))
	return nil
}

func generateTypeList() []string {
	var typeList []string
	for _, val := range pb.EntityType_name {
		if val != "ENTITY_TYPE_UNSPECIFIED" {
			typeList = append(typeList, val)
		}
	}
	return typeList
}

func readFromFile(filePath string, entity *pb.Entity) error {
	msg, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("invalid file path: %w", err)
	}

	if err := prototext.Unmarshal(msg, entity); err != nil {
		return fmt.Errorf("invalid file content: %w", err)
	}
	return nil
}

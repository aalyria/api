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

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"

	nbictl "aalyria.com/spacetime/github/tools/nbictl"
)

var subCmds = map[string]func(context.Context, []string) error{
	"list":          nbictl.List,
	"create":        nbictl.Create,
	"update":        nbictl.Update,
	"delete":        nbictl.Delete,
	"generate-keys": nbictl.GenerateKeys,
	"set-context":   nbictl.SetContext,
}

const (
	linkToAuthGuide = "https://docs.spacetime.aalyria.com/authentication"
	clientName      = "nbictl"
)

func getSubcommandNames() []string {
	var cmdList []string
	for cmd := range subCmds {
		cmdList = append(cmdList, cmd)
	}
	return cmdList
}

func run() error {
	args := flag.Args()
	if flag.NArg() == 0 {
		return errors.New("Please specify a subcommand")
	}
	cmd, args := args[0], args[1:]
	ctx := context.Background()

	cmdToPerform, ok := subCmds[cmd]
	if !ok {
		return fmt.Errorf("invalid command: %s. must be one of %v", cmd, getSubcommandNames())
	}
	return cmdToPerform(ctx, args)
}

func main() {
	flag.Parse()
	if err := run(); err != nil {
		log.Fatalf("fatal error: %v", err)
	}
}

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
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
	intervalpb "google.golang.org/genproto/googleapis/type/interval"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonpb "aalyria.com/spacetime/api/common"
	nbipb "aalyria.com/spacetime/api/nbi/v1alpha"
	resourcespb "aalyria.com/spacetime/api/nbi/v1alpha/resources"
)

var Version = "0.0.0+development"

const (
	confFileName = "config.textproto"

	nbiAPISubDomain = "nbi-v1alpha"

	// modified from
	// https://github.com/urfave/cli/blob/c023d9bc5a3122830c9355a0a8c17137e0c8556f/template.go#L98
	readmeDocTemplate = `{{if gt .SectionNum 0}}% {{ .App.Name }} {{ .SectionNum }}

{{end}}# NAME

{{ .App.Name }}{{ if .App.Usage }} - {{ .App.Usage }}{{ end }}

# SYNOPSIS

{{ if .SynopsisArgs }}` + "```" + `
{{ .App.Name }} {{ range $f := .App.VisibleFlags -}}{{ range $n := $f.Names }}[{{ if len $n | lt 1 }}--{{ else }}-{{ end }}{{ $n }}{{ if $f.TakesValue }}=value{{ end }}] {{ end }}{{ end }}<command> [COMMAND OPTIONS] [ARGUMENTS...]
` + "```" + `
{{ end }}{{ if .GlobalArgs }}
# GLOBAL OPTIONS
{{ range $v := .GlobalArgs }}
{{ $v }}{{ end }}
{{ end }}{{ if .Commands }}# COMMANDS
{{ range $v := .Commands }}
{{ $v }}{{ end }}{{ end }}`

	appName = "nbictl"
)

var entityTypeList = generateTypeList()

func init() {
	cli.MarkdownDocTemplate = readmeDocTemplate
}

type protoFormat struct {
	marshal   func(proto.Message) ([]byte, error)
	unmarshal func([]byte, proto.Message) error
	fileExt   string
}

func marshallerForFormat(format string) (protoFormat, error) {
	switch format {
	case "json":
		return protoFormat{fileExt: "json", marshal: (protojson.MarshalOptions{Multiline: true, Indent: "  "}).Marshal, unmarshal: protojson.Unmarshal}, nil
	case "text":
		return protoFormat{fileExt: "txtpb", marshal: (prototext.MarshalOptions{Multiline: true, Indent: "  "}).Marshal, unmarshal: prototext.Unmarshal}, nil
	case "binary":
		return protoFormat{fileExt: "binpb", marshal: proto.Marshal, unmarshal: proto.Unmarshal}, nil
	default:
		return protoFormat{}, fmt.Errorf("unknown proto format %q", format)
	}
}

func App() *cli.App {
	formatFlag := &cli.StringFlag{
		Name:     "format",
		Usage:    "The format to use for encoding and decoding protobuf messages. One of [text, json, binary].",
		Required: false,
		Value:    "text",
		Action:   validateProtoFormat,
	}
	dryrunFlag := &cli.BoolFlag{
		Name:    "dry-run",
		Usage:   "perform a trial run that doesn't make any changes",
		Aliases: []string{"n"},
	}
	verboseFlag := &cli.BoolFlag{
		Name:    "verbose",
		Usage:   "increase verbosity",
		Aliases: []string{"v"},
	}

	return &cli.App{
		Name:                 appName,
		Version:              Version,
		Usage:                "Interact with the Spacetime NBI service from the command line.",
		Description:          fmt.Sprintf("`%s` is a tool that allows you to interact with the Spacetime NBI APIs from the command-line.", appName),
		BashComplete:         cli.DefaultAppComplete,
		EnableBashCompletion: true,
		Suggest:              true,
		Reader:               os.Stdin,
		Writer:               os.Stdout,
		ErrWriter:            os.Stderr,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "profile",
				Usage:   "Configuration profile to use.",
				Aliases: []string{"context"},
			},
			&cli.StringFlag{
				Name:        "config_dir",
				Usage:       "Directory to use for configuration.",
				DefaultText: "$XDG_CONFIG_HOME/" + appName,
			},
		},
		Commands: []*cli.Command{
			{
				Name:     "readme",
				Category: "help",
				Usage:    "Prints the help information as Markdown.",
				Hidden:   true,
				Action: func(appCtx *cli.Context) error {
					md, err := appCtx.App.ToMarkdown()
					if err != nil {
						return err
					}
					fmt.Fprintln(appCtx.App.Writer, `<!--`)
					fmt.Fprintln(appCtx.App.Writer, "This file is autogenerated! Do not edit by hand!")
					fmt.Fprintf(appCtx.App.Writer, "Run `%s readme > README.md` to update it.\n", appName)
					fmt.Fprintln(appCtx.App.Writer, "-->")
					fmt.Fprintln(appCtx.App.Writer)

					fmt.Fprintln(appCtx.App.Writer, md)
					return nil
				},
			},
			{
				Name:     "man",
				Category: "help",
				Usage:    "Prints the help information as a man page.",
				Hidden:   true,
				Action: func(appCtx *cli.Context) error {
					man, err := appCtx.App.ToMan()
					if err != nil {
						return err
					}
					fmt.Fprintln(appCtx.App.Writer, man)
					return nil
				},
			},
			{
				Name:     "get",
				Category: "entities",
				Usage:    "Gets the entity with the given type and ID.",
				Flags: append([]cli.Flag{
					&cli.StringFlag{
						Name:     "type",
						Usage:    fmt.Sprintf("[REQUIRED] Type of entity to delete. Allowed values: [%s]", strings.Join(entityTypeList, ", ")),
						Aliases:  []string{"t"},
						Required: true,
						Action:   validateEntityType,
					},
					&cli.StringFlag{
						Name:     "id",
						Usage:    "[REQUIRED] ID of entity to delete.",
						Aliases:  []string{},
						Required: true,
					},
				}, formatFlag),
				Action: Get,
			},
			{
				Name:     "create",
				Category: "entities",
				Usage:    "Create one or more entities described in a file tree.",
				Flags: append([]cli.Flag{
					&cli.StringFlag{
						Name:     "files",
						Usage:    "[REQUIRED] Glob of files containing serialized Entity messages. Use - to read from stdin.",
						Aliases:  []string{"f"},
						Required: true,
					},
				}, formatFlag),
				Action: Create,
			},
			{
				Name:     "edit",
				Category: "entities",
				Usage:    "Opens the specified entity in $EDITOR, then updates the NBI's version with any changes made.",
				Flags: append([]cli.Flag{
					&cli.StringFlag{
						Name:     "type",
						Usage:    fmt.Sprintf("[REQUIRED] Type of entity to edit. Allowed values: [%s]", strings.Join(entityTypeList, ", ")),
						Aliases:  []string{"t"},
						Required: true,
						Action:   validateEntityType,
					},
					&cli.StringFlag{
						Name:     "id",
						Usage:    "[REQUIRED] ID of entity to edit.",
						Aliases:  []string{},
						Required: true,
					},
				}, formatFlag),
				Action: Edit,
			},
			{
				Name:     "update",
				Category: "entities",
				Usage:    "Updates, or creates if missing, one or more entities described in a file tree.",
				Flags: append([]cli.Flag{
					&cli.StringFlag{
						Name:     "files",
						Usage:    "[REQUIRED] Glob of files containing serialized Entity messages. Use - to read from stdin.",
						Aliases:  []string{"f"},
						Required: true,
					},
					&cli.BoolFlag{
						Name:        "ignore_consistency_check",
						DefaultText: "false",
						Usage:       "Always update or create the entity, without verifying that the provided `commit_timestamp` matches the currently stored entity.",
					},
				}, formatFlag),
				Action: Update,
			},
			{
				Name:     "list",
				Category: "entities",
				Usage:    "Lists all entities of a given type.",
				Flags: append([]cli.Flag{
					&cli.StringFlag{
						Name:     "type",
						Usage:    fmt.Sprintf("[REQUIRED] Type of entities to query. Allowed values: [%s]", strings.Join(entityTypeList, ", ")),
						Aliases:  []string{"t"},
						Required: true,
						Action:   validateEntityType,
					},
					&cli.StringFlag{
						Name:     "field_masks",
						Usage:    "Comma-separated allow-list of fields to include in the response; see the aalyria.spacetime.api.nbi.v1alpha.EntityFilter.field_masks documentation for usage details.",
						Required: false,
						Aliases:  []string{},
					},
				}, formatFlag),
				Action: List,
			},
			{
				Name:     "delete",
				Category: "entities",
				Usage:    "Deletes one or more entities. Provide the type and ID to delete a single entity, or a directory of files containing serialized Entity messages to delete multiple entities.",
				Flags: append([]cli.Flag{
					&cli.StringFlag{
						Name:    "type",
						Usage:   fmt.Sprintf("Type of entity to delete. Allowed values: [%s]", strings.Join(entityTypeList, ", ")),
						Aliases: []string{"t"},
						Action:  validateEntityType,
					},
					&cli.StringFlag{
						Name:    "id",
						Usage:   "ID of entity to delete.",
						Aliases: []string{},
					},
					&cli.IntFlag{
						Name:    "last_commit_timestamp",
						Usage:   "Delete the entity only if `last_commit_timestamp` matches the `commit_timestamp` of the currently stored entity.",
						Aliases: []string{},
					},
					&cli.BoolFlag{
						Name:        "ignore_consistency_check",
						DefaultText: "false",
						Usage:       "Always update or create the entity, without verifying that the provided `commit_timestamp` matches the value in the currently stored entity.",
						Aliases:     []string{},
					},
					&cli.StringFlag{
						Name:    "files",
						Usage:   "Glob of serialized protobuf files that represent one or more Entity messages. Use - to read from stdin.",
						Aliases: []string{"f"},
					},
				}, formatFlag),
				Action: Delete,
			},
			{
				Name:        "get-link-budget",
				Usage:       "Gets link budget details",
				Description: "Gets link budget details for a given signal propagation request between a transmitter and a target platform.",
				Flags: append([]cli.Flag{
					&cli.StringFlag{
						Name:  "input_file",
						Usage: "A path to a file containing a serialized SignalPropagationRequest message. If set, it will be used as the request to the SignalPropagation service. If unset, the request will be built from the other flags.",
					},
					&cli.StringFlag{
						Name:  "tx_platform_id",
						Usage: "The Entity ID of the PlatformDefinition that represents the transmitter.",
					},
					&cli.StringFlag{
						Name:  "tx_transceiver_model_id",
						Usage: "The ID of the transceiver model on the transmitter.",
					},
					&cli.StringFlag{
						Name:  "target_platform_id",
						Usage: "The Entity ID of the PlatformDefinition that represents the target. Leave unset if the antenna is fixed or non-steerable, in which case coverage calculations will be returned.",
					},
					&cli.StringFlag{
						Name:  "target_transceiver_model_id",
						Usage: "The ID of the transceiver model on the target.Leave unset if the antenna is fixed or non-steerable, in which case coverage calculations will be returned.",
					},
					&cli.StringFlag{
						Name:  "band_profile_id",
						Usage: "The Entity ID of the BandProfile used for this link.",
					},
					&cli.TimestampFlag{
						Name:   "analysis_start_timestamp",
						Layout: time.RFC3339,
						Usage:  "An RFC3339 formatted timestamp for the beginning of the interval to evaluate the signal propagation. Defaults to the current local timestamp.",
					},
					&cli.TimestampFlag{
						Name:   "analysis_end_timestamp",
						Layout: time.RFC3339,
						Usage:  "An RFC3339 formatted timestamp for the end of the interval to evaluate the signal propagation. If unset, the signal propagation is evaluated at the instant of the `analysis_start_timestamp.`",
					},
					&cli.DurationFlag{
						Name:        "step_size",
						DefaultText: "1m",
						Usage:       "The analysis step size and the temporal resolution of the response.",
					},
					&cli.DurationFlag{
						Name:        "spatial_propagation_step_size",
						DefaultText: "1m",
						Usage:       "The analysis step size for spatial propagation metrics.",
					},
					&cli.BoolFlag{
						Name:        "explain_inaccessibility",
						DefaultText: "false",
						Usage:       "If true, the server will spend additional computational time determining the specific set of access constraints that were not satisfied and including these reasons in the response.",
					},
					&cli.TimestampFlag{
						Name:        "reference_data_timestamp",
						Layout:      time.RFC3339,
						Usage:       "An RFC3339 formatted timestamp for the instant at which to reference the versions of the platforms. Defaults to `analysis_start_timestamp`.",
						DefaultText: "analysis_start_timestamp",
					},
					&cli.PathFlag{
						Name:        "output_file",
						Usage:       "Path to a file to write the response. If unset, defaults to stdout.",
						DefaultText: "/dev/stdout",
					},
				}, formatFlag),
				Action: GetLinkBudget,
			},
			{
				Name:      "generate-keys",
				Usage:     "Generate RSA keys to use for authentication with the Spacetime APIs.",
				UsageText: "After creating the Private-Public keypair, you will need to request API access by sharing the `.crt` file (a self-signed x509 certificate containing the public key) with Aalyria to receive the `USER_ID` and a `KEY_ID` needed to complete the nbictl configuration. Only share the public certificate (`.crt`) with Aalyria or third-parties. The private key (`.key`) must be protected and should never be sent by email or communicated to others.",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "dir",
						Usage:       "Directory to store the generated RSA keys in.",
						DefaultText: "~/.config/" + appName + "/keys",
						Aliases:     []string{"directory"},
					},
					&cli.StringFlag{
						Name:     "org",
						Usage:    "[REQUIRED] Organization of certificate.",
						Aliases:  []string{"organization"},
						Required: true,
					},
					&cli.StringFlag{
						Name:  "country",
						Usage: "Country of certificate.",
					},
					&cli.StringFlag{
						Name:  "state",
						Usage: "State of certificate.",
					},
					&cli.StringFlag{
						Name:  "location",
						Usage: "Location of certificate.",
					},
				},
				Action: GenerateKeys,
			},
			{
				Name:  "config",
				Usage: fmt.Sprintf("Provides subcommands for managing %s configuration.", appName),
				Subcommands: []*cli.Command{
					{
						Name:   "list-profiles",
						Usage:  "List all configuration profiles (ignores any `--profile` flag)",
						Action: ListConfigs,
					},
					{
						Name:   "describe",
						Usage:  "Prints the NBI connection settings associated with the configuration profile given by the `--profile` flag (defaults to \"DEFAULT\").",
						Action: GetConfig,
					},
					{
						Name:  "set",
						Usage: "Sets or updates a configuration profile settings. You can create multiple profiles by specifying the `--profile` flag (defaults to \"DEFAULT\").",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "priv_key",
								Usage: "Path to the private key to use for authentication.",
							},
							&cli.StringFlag{
								Name:  "key_id",
								Usage: "Key ID associated with the private key provided by Aalyria.",
							},
							&cli.StringFlag{
								Name:  "user_id",
								Usage: "User ID associated with the private key provided by Aalyria.",
							},
							&cli.StringFlag{
								Name:  "url",
								Usage: "Spacetime endpoint specified as `host[:port]` (port is optional and defaults to 443). MUST NOT include any scheme (e.g. `https://` or `dns:///`) nor any API-specific prefix (e.g. `nbi.` or `model-v1.`).",
							},
							&cli.StringFlag{
								Name:  "transport_security",
								Usage: "Transport security to use when connecting to the NBI service. Allowed values: [insecure, system_cert_pool]",
							},
						},
						Action: SetConfig,
					},
				},
			},
			{
				Name:  "model-v1",
				Usage: "Subcommands for Model API v1, to manage the model elements comprising the digital twin.",
				Subcommands: []*cli.Command{
					{
						Name:     "create-entity",
						Usage:    "Create the model NMTS Entity contained within the file provided on the command line ('-' reads from stdin).",
						Category: "model entities",
						Action:   ModelCreateEntity,
						Flags:    []cli.Flag{formatFlag},
					},
					{
						Name:     "update-entity",
						Usage:    "Update the model NMTS Entity contained within the file provided on the command line ('-' reads from stdin).",
						Category: "model entities",
						Action:   ModelUpdateEntity,
						Flags:    []cli.Flag{formatFlag},
						// TODO: support partial update by adding a flag to specify the update_mask
					},
					{
						Name:     "delete-entity",
						Usage:    "Delete the model NMTS Entity associated with the entity ID provided on the command line, along with any relationships in which it participates.",
						Category: "model entities",
						Action:   ModelDeleteEntity,
						Flags:    []cli.Flag{formatFlag},
					},
					{
						Name:     "get-entity",
						Usage:    "Get the model NMTS Entity associated with the entity ID given on the command line.",
						Category: "model entities",
						Action:   ModelGetEntity,
						Flags:    []cli.Flag{formatFlag},
					},
					{
						Name:     "create-relationship",
						Usage:    "Insert the model NMTS Relationship contained within the file provided on the command line ('-' reads from stdin).",
						Category: "model relationships",
						Action:   ModelCreateRelationship,
						Flags:    []cli.Flag{formatFlag},
					},
					{
						Name:     "delete-relationship",
						Usage:    "Delete the model NMTS Relationship contained within the file provided on the command line ('-' reads from stdin).",
						Category: "model relationships",
						Action:   ModelDeleteRelationship,
						Flags:    []cli.Flag{formatFlag},
					},
					{
						Name:     "upsert-fragment",
						Usage:    "Upsert the model NMTS Fragment contained within the file provided on the command line ('-' reads from stdin).",
						Category: "model relationships",
						Action:   ModelUpsertFragment,
						Flags:    []cli.Flag{formatFlag},
					},
					{
						Name:     "list-entities",
						Usage:    "List all model entities.",
						Category: "model entities",
						Action:   ModelListEntities,
						Flags:    []cli.Flag{formatFlag},
					},
					{
						Name:     "list-relationships",
						Usage:    "List all model relationships.",
						Category: "model relationships",
						Action:   ModelListRelationships,
						Flags:    []cli.Flag{formatFlag},
						// TODO: support filter param
					},
					{
						Name:     "sync",
						Aliases:  []string{"rsync"},
						Usage:    "Sync all model entities and relationships from file and directory arguments.",
						Category: "model entities and relationships",
						Action:   ModelSync,
						Flags: []cli.Flag{
							formatFlag,
							dryrunFlag,
							verboseFlag,
							&cli.BoolFlag{
								Name:    "delete",
								Usage:   "delete entities and relationships from remote instance not present in local sources",
								Aliases: []string{"d", "delete-before"},
							},
							&cli.BoolFlag{
								Name:    "recursive",
								Usage:   "descend recursively into directory arguments",
								Aliases: []string{"r"},
							},
						},
					},
				},
			},
			{
				Name:  "provisioning-v1alpha",
				Usage: "Subcommands for Provisioning API v1alpha, to manage the provisioned resources within the digital twin.",
				Subcommands: []*cli.Command{
					{
						Name:     "sync",
						Aliases:  []string{"rsync"},
						Usage:    "Sync all provisioning resources from file and directory arguments.",
						Category: "provisioning resources",
						Action:   ProvisioningSync,
						Flags: []cli.Flag{
							formatFlag,
							dryrunFlag,
							verboseFlag,
							&cli.BoolFlag{
								Name:    "delete",
								Usage:   "delete resources from remote instance not present in local sources",
								Aliases: []string{"d", "delete-before"},
							},
							&cli.BoolFlag{
								Name:    "recursive",
								Usage:   "descend recursively into directory arguments",
								Aliases: []string{"r"},
							},
						},
					},
				},
			},
			{
				Name:  "grpcurl",
				Usage: "Provides curl-like equivalents for interacting with the NBI.",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "subdomain",
						Usage: "The subdomain to connect to (e.g. 'model', 'provisioning', ...).",
					},
				},
				Subcommands: []*cli.Command{
					{
						Name:   "describe",
						Usage:  "Takes an optional fully-qualified symbol (service, enum, or message). If provided, the descriptor for that symbol is shown. If not provided, the descriptor for all exposed or known services are shown.",
						Action: GRPCDescribe,
					},
					{
						Name:   "list",
						Usage:  "Takes an optional fully-qualified service name. If provided, lists all methods of that service. If not provided, all exposed services are listed.",
						Action: GRPCList,
					},
					{
						Name:    "call",
						Aliases: []string{"invoke"},
						Usage:   "Takes a fully-qualified method name in 'service.method' or 'service/method' format. Invokes the method using the provided request body.",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:        "format",
								Usage:       formatFlag.Usage,
								DefaultText: "json",
								Aliases:     []string{"f"},
								Action:      formatFlag.Action,
							},
							&cli.StringFlag{
								Name:        "request",
								Usage:       "File containing the request to make encoded in the selected --format. Defaults to -, which uses stdin.",
								DefaultText: "-",
								Aliases:     []string{"r"},
							},
						},
						Action: GRPCCall,
					},
				},
			},
			{
				Name:  "generate-auth-token",
				Usage: "Generate a self-signed JWT token for API authentication.",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "audience",
						Usage:   "The audience (aud) to set in the JWT token.",
						Aliases: []string{"aud"},
					},
					&cli.DurationFlag{
						Name:        "expiration",
						Usage:       "The validity duration of token, from the time of creation.",
						Aliases:     []string{"exp"},
						DefaultText: "1h",
						Value:       1 * time.Hour,
					},
				},
				Action: GenerateAuthToken,
			},
		},
	}
}

func Create(appCtx *cli.Context) error {
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	conn, err := openAPIConnection(appCtx, nbiAPISubDomain)
	if err != nil {
		return err
	}
	defer conn.Close()
	client := nbipb.NewNetOpsClient(conn)

	createEntityFunc := func(ctx context.Context, e *nbipb.Entity) error {
		req := &nbipb.CreateEntityRequest{Entity: e}
		res, err := client.CreateEntity(ctx, req)
		if err != nil {
			return fmt.Errorf("create failed for entity %s/%s: %w", req.Entity.GetGroup().GetType(), req.GetEntity().GetId(), err)
		}
		fmt.Fprintf(appCtx.App.ErrWriter, "successfully created:  %s/%s\n", res.GetGroup().GetType(), res.GetId())
		return nil
	}
	return processEntitiesFromFiles(appCtx, marshaller, appCtx.String("files"), createEntityFunc)
}

func Edit(appCtx *cli.Context) error {
	ed := ""
	for _, env := range []string{"VISUAL", "EDITOR"} {
		ed = os.Getenv(env)
		if ed != "" {
			break
		}
	}
	if ed == "" {
		return fmt.Errorf("No $EDITOR value set, don't know which editor to use")
	}
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	conn, err := openAPIConnection(appCtx, nbiAPISubDomain)
	if err != nil {
		return err
	}
	defer conn.Close()
	client := nbipb.NewNetOpsClient(conn)

	id := appCtx.String("id")
	entityType := appCtx.String("type")
	et, found := nbipb.EntityType_value[entityType]
	if !found {
		return fmt.Errorf("invalid type: %q", entityType)
	}
	oldEntity, err := client.GetEntity(appCtx.Context, &nbipb.GetEntityRequest{Type: nbipb.EntityType(et).Enum(), Id: &id})
	if err != nil {
		return fmt.Errorf("unable to get the entity via the NBI: %w", err)
	}

	tmp, err := os.MkdirTemp("", "nbictl")
	if err != nil {
		return fmt.Errorf("opening tmp dir: %w", err)
	}
	defer os.RemoveAll(tmp)

	oldData, err := marshaller.marshal(oldEntity)
	if err != nil {
		return fmt.Errorf("marshalling entity as textproto: %w", err)
	}

	fname := filepath.Join(tmp, fmt.Sprintf("entity.%s", marshaller.fileExt))
	if err := os.WriteFile(fname, oldData, 0o755); err != nil {
		return fmt.Errorf("writing entity to file %s: %w", fname, err)
	}

	cmd := exec.CommandContext(appCtx.Context, ed, fname)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command `%s` exited with an error: %w", cmd.Args, err)
	}

	newData, err := os.ReadFile(fname)
	if err != nil {
		return fmt.Errorf("reading modified entity from %s: %w", fname, err)
	}
	newEntity := &nbipb.Entity{}
	if err := marshaller.unmarshal(newData, newEntity); err != nil {
		return fmt.Errorf("unmarshalling modified entity: %w", err)
	}
	if _, err := client.UpdateEntity(appCtx.Context, &nbipb.UpdateEntityRequest{Entity: newEntity}); err != nil {
		return fmt.Errorf("calling UpdateEntity: %w", err)
	}
	return nil
}

func Update(appCtx *cli.Context) error {
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}
	conn, err := openAPIConnection(appCtx, nbiAPISubDomain)
	if err != nil {
		return err
	}
	defer conn.Close()
	client := nbipb.NewNetOpsClient(conn)

	updateEntityFunc := func(ctx context.Context, e *nbipb.Entity) error {
		req := &nbipb.UpdateEntityRequest{Entity: e, IgnoreConsistencyCheck: proto.Bool(true)}
		res, err := client.UpdateEntity(ctx, req)
		if err != nil {
			return fmt.Errorf("update failed for entity %s/%s: %w", req.Entity.GetGroup().GetType(), req.GetEntity().GetId(), err)
		}
		fmt.Fprintf(appCtx.App.ErrWriter, "successfully updated: %s/%s\n", res.GetGroup().GetType(), res.GetId())
		return nil
	}
	return processEntitiesFromFiles(appCtx, marshaller, appCtx.String("files"), updateEntityFunc)
}

func Get(appCtx *cli.Context) error {
	entityType := appCtx.String("type")
	id := appCtx.String("id")

	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	conn, err := openAPIConnection(appCtx, nbiAPISubDomain)
	if err != nil {
		return err
	}
	defer conn.Close()
	client := nbipb.NewNetOpsClient(conn)

	entityTypeEnumValue, found := nbipb.EntityType_value[entityType]
	if !found {
		return fmt.Errorf("invalid type: %q", entityType)
	}
	entityTypeEnum := nbipb.EntityType(entityTypeEnumValue)
	entity, err := client.GetEntity(appCtx.Context, &nbipb.GetEntityRequest{Type: &entityTypeEnum, Id: &id})
	if err != nil {
		return fmt.Errorf("unable to get the entity: %w", err)
	}
	marshalledProtos, err := marshaller.marshal(&nbipb.TxtpbEntities{Entity: []*nbipb.Entity{entity}})
	if err != nil {
		return fmt.Errorf("unable to marshal the response: %w", err)
	}
	fmt.Fprintln(appCtx.App.Writer, string(marshalledProtos))
	return nil
}

func Delete(appCtx *cli.Context) error {
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	conn, err := openAPIConnection(appCtx, nbiAPISubDomain)
	if err != nil {
		return err
	}
	defer conn.Close()
	client := nbipb.NewNetOpsClient(conn)

	deleteFunc := func(ctx context.Context, req *nbipb.DeleteEntityRequest) error {
		if _, err := client.DeleteEntity(appCtx.Context, req); err != nil {
			return fmt.Errorf("deletion failed for entity %s/%s: %w", req.Type, *req.Id, err)
		}
		fmt.Fprintf(appCtx.App.ErrWriter, "successfully deleted: %s/%s\n", req.Type, *req.Id)
		return nil
	}

	if appCtx.IsSet("type") && appCtx.IsSet("id") {
		entityId := appCtx.String("id")
		entityType := appCtx.String("type")
		entityTypeEnumValue, found := nbipb.EntityType_value[entityType]
		if !found {
			return fmt.Errorf("invalid type: %q", entityType)
		}
		if appCtx.IsSet("last_commit_timestamp") == appCtx.Bool("ignore_consistency_check") {
			return fmt.Errorf(`when deleting a single entity, either "last_commit_timestamp" or "ignore_consistency_check" flags should be set.`)
		}
		entityTypeEnum := nbipb.EntityType(entityTypeEnumValue)
		req := &nbipb.DeleteEntityRequest{Type: &entityTypeEnum, Id: &entityId}
		if appCtx.IsSet("last_commit_timestamp") {
			req.LastCommitTimestamp = proto.Int64(appCtx.Int64("last_commit_timestamp"))
		}
		if appCtx.Bool("ignore_consistency_check") {
			req.IgnoreConsistencyCheck = proto.Bool(true)
		}
		return deleteFunc(appCtx.Context, req)
	} else if appCtx.IsSet("files") {
		deleteEntityFunc := func(ctx context.Context, e *nbipb.Entity) error {
			entityId := e.GetId()
			entityType := e.GetGroup().GetType()
			req := &nbipb.DeleteEntityRequest{Type: &entityType, Id: &entityId}
			if e.CommitTimestamp != nil {
				req.LastCommitTimestamp = e.CommitTimestamp
			}
			if appCtx.Bool("ignore_consistency_check") {
				req.IgnoreConsistencyCheck = proto.Bool(true)
			}
			return deleteFunc(ctx, req)
		}
		return processEntitiesFromFiles(appCtx, marshaller, appCtx.String("files"), deleteEntityFunc)
	} else {
		return fmt.Errorf(`either the "type" and "id" flags must be set, or the "files" flag must be set.`)
	}
}

func List(appCtx *cli.Context) error {
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	entityType := appCtx.String("type")
	fieldMasks := strings.Split(appCtx.String("field_masks"), ",")
	entityTypeEnumValue, found := nbipb.EntityType_value[entityType]
	if !found {
		return fmt.Errorf("unknown entity type %q is not one of [%s]", entityType, strings.Join(entityTypeList, ", "))
	}
	entityTypeEnum := nbipb.EntityType(entityTypeEnumValue)
	entityFilter := nbipb.EntityFilter{}
	for _, value := range fieldMasks {
		if trimmedValue := strings.TrimSpace(value); trimmedValue != "" {
			entityFilter.FieldMasks = append(entityFilter.FieldMasks, trimmedValue)
		}
	}

	conn, err := openAPIConnection(appCtx, nbiAPISubDomain)
	if err != nil {
		return err
	}
	defer conn.Close()
	client := nbipb.NewNetOpsClient(conn)

	res, err := client.ListEntities(appCtx.Context, &nbipb.ListEntitiesRequest{Type: &entityTypeEnum, Filter: &entityFilter})
	if err != nil {
		return fmt.Errorf("unable to list entities: %w", err)
	}
	marshalledProtos, err := marshaller.marshal(&nbipb.TxtpbEntities{Entity: res.Entities})
	if err != nil {
		return fmt.Errorf("unable to marshal the response: %w", err)
	}
	fmt.Fprintln(appCtx.App.Writer, string(marshalledProtos))
	fmt.Fprintf(appCtx.App.ErrWriter, "Successfully queried a list of entities. Number of entities: %d\n", len(res.Entities))
	return nil
}

func GetLinkBudget(appCtx *cli.Context) error {
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	conn, err := openAPIConnection(appCtx, nbiAPISubDomain)
	if err != nil {
		return err
	}
	defer conn.Close()
	client := nbipb.NewSignalPropagationClient(conn)

	spReq := &nbipb.SignalPropagationRequest{}
	if appCtx.IsSet("input_file") {
		reqPath := appCtx.String("input_file")
		// If the user input a textproto file, use it to build the request.
		req, err := os.ReadFile(reqPath)
		if err != nil {
			return fmt.Errorf("invalid file path: %w", err)
		}

		if err := marshaller.unmarshal(req, spReq); err != nil {
			return fmt.Errorf("reading SignalPropagationRequest from file %s: %w", reqPath, err)
		}
	} else {
		txPlatformID := appCtx.String("tx_platform_id")
		txTransceiverModelID := appCtx.String("tx_transceiver_model_id")
		bandProfileID := appCtx.String("band_profile_id")

		errs := []error{}
		if txPlatformID == "" {
			errs = append(errs, errors.New("--tx_platform_id required"))
		}
		if txTransceiverModelID == "" {
			errs = append(errs, errors.New("--tx_transceiver_model_id required"))
		}
		if bandProfileID == "" {
			// TODO: Output a list of valid band profile IDs.
			errs = append(errs, errors.New("--band_profile_id required"))
		}

		startTime := appCtx.Timestamp("analysis_start_timestamp")
		if startTime == nil {
			// If the user did not specify the start of the analysis interval,
			// it is set to the current local time.
			now := time.Now()
			startTime = &now
		}

		endTime := appCtx.Timestamp("analysis_end_timestamp")
		if endTime == nil {
			// If the user did not provide the end of the analysis interval, it
			// is set to the start time. Therefore, the signal propagation will
			// be evaluated at the instant of the start time.
			endTime = startTime
		}

		refDataTime := appCtx.Timestamp("reference_data_timestamp")
		if refDataTime == nil {
			// If the user did not specify a reference data time, it is set to
			// the start of the analysis interval. Therefore, the version of
			// the entities used in the signal propagation analysis will match
			// the start of the analysis interval.
			refDataTime = startTime
		}

		target := &resourcespb.TransceiverProvider{}
		switch {
		case appCtx.IsSet("target_platform_id") && appCtx.IsSet("target_transceiver_model_id"):
			target = &resourcespb.TransceiverProvider{
				Source: &resourcespb.TransceiverProvider_IdInStore{
					IdInStore: &commonpb.TransceiverModelId{
						PlatformId:         proto.String(appCtx.String("target_platform_id")),
						TransceiverModelId: proto.String(appCtx.String("target_transceiver_model_id")),
					},
				},
			}
		case !appCtx.IsSet("target_platform_id") && !appCtx.IsSet("target_transceiver_model_id"):
			// When the target's platform ID and transceiver model ID are not
			// specified, the target field should be left unset (as opposed to
			// setting it to an empty TransceiverProvider) to model the case of
			// a fixed antenna.
			target = nil
		case !appCtx.IsSet("target_platform_id"):
			errs = append(errs, errors.New("--target_platform_id required"))
		case !appCtx.IsSet("target_transceiver_model_id"):
			errs = append(errs, errors.New("--target_transceiver_model_id required."))
		}

		if err := errors.Join(errs...); err != nil {
			return err
		}

		stepSize := appCtx.Duration("step_size")
		spatialPropagationStepSize := appCtx.Duration("spatial_propagation_step_size")
		explainInaccessibility := appCtx.Bool("explain_inaccessibility")

		spReq = &nbipb.SignalPropagationRequest{
			TransmitterModel: &resourcespb.TransceiverProvider{
				Source: &resourcespb.TransceiverProvider_IdInStore{
					IdInStore: &commonpb.TransceiverModelId{
						PlatformId:         &txPlatformID,
						TransceiverModelId: &txTransceiverModelID,
					},
				},
			},
			BandProfileId: &bandProfileID,
			Target:        target,
			AnalysisTime: &nbipb.SignalPropagationRequest_AnalysisInterval{
				AnalysisInterval: &intervalpb.Interval{
					StartTime: timestamppb.New(*startTime),
					EndTime:   timestamppb.New(*endTime),
				},
			},
			StepSize:                   durationpb.New(stepSize),
			SpatialPropagationStepSize: durationpb.New(spatialPropagationStepSize),
			ExplainInaccessibility:     &explainInaccessibility,
			ReferenceDataTime:          timestamppb.New(*refDataTime),
		}
	}

	spRes, err := client.Evaluate(appCtx.Context, spReq)
	if err != nil {
		return fmt.Errorf("SignalPropagation.Evaluate: %w", err)
	}
	spResProto, err := marshaller.marshal(spRes)
	if err != nil {
		return fmt.Errorf("unable to marshal the response: %w", err)
	}

	if !appCtx.IsSet("output_file") {
		fmt.Fprintln(appCtx.App.Writer, string(spResProto))
	} else {
		outPath := appCtx.Path("output_file")
		// Creates the output file, if necessary, with read and write permissions.
		if err := os.WriteFile(outPath, spResProto, 0o666); err != nil {
			return fmt.Errorf("writing to output file %s: %w", outPath, err)
		}
	}
	fmt.Fprintln(appCtx.App.ErrWriter, "Successfully retrieved link budget.")
	return nil
}

func generateTypeList() []string {
	var typeList []string
	for _, val := range nbipb.EntityType_name {
		if val != "ENTITY_TYPE_UNSPECIFIED" {
			typeList = append(typeList, val)
		}
	}
	sort.Strings(typeList)
	return typeList
}

func processEntitiesFromFiles(appCtx *cli.Context, marshaller protoFormat, fileGlob string, f func(context.Context, *nbipb.Entity) error) error {
	entities := &nbipb.TxtpbEntities{}

	switch {
	case fileGlob == "-":
		data, err := io.ReadAll(appCtx.App.Reader)
		if err != nil {
			return fmt.Errorf("reading from stdin: %w", err)
		}
		if err := marshaller.unmarshal(data, entities); err != nil {
			return err
		}

	default:
		files, err := filepath.Glob(fileGlob)
		if err != nil {
			return fmt.Errorf("unable to expand the file glob %q: %w", fileGlob, err)
		} else if len(files) == 0 {
			return fmt.Errorf("no files found under the given file glob: %s", fileGlob)
		}

		for _, path := range files {
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("invalid file path %q: %w", path, err)
			}
			fileEnts := &nbipb.TxtpbEntities{}
			if err := marshaller.unmarshal(data, fileEnts); err != nil {
				return fmt.Errorf("parsing file %q: %w", path, err)
			}
			entities.Entity = append(entities.Entity, fileEnts.Entity...)
		}
	}

	g, gCtx := errgroup.WithContext(appCtx.Context)
	for _, e := range entities.Entity {
		entity := e
		g.Go(func() error { return f(gCtx, entity) })
	}
	return g.Wait()
}

func validateEntityType(_ *cli.Context, t string) error {
	for _, et := range entityTypeList {
		if t == et {
			return nil
		}
	}
	return fmt.Errorf("unknown entity type %q", t)
}

func validateProtoFormat(_ *cli.Context, f string) error {
	switch f {
	case "text", "json", "binary":
		return nil
	default:
		return fmt.Errorf("unknown format %q", f)
	}
}

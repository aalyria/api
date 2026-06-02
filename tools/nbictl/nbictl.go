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
	"os"
	"runtime"
	"runtime/pprof"
	"slices"
	"time"

	"github.com/urfave/cli/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

var Version = "0.0.0+development"

const (
	confFileName = "config.textproto"

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
	// Flag for when default should always be dry run
	executeFlag := &cli.BoolFlag{
		Name:    "execute",
		Usage:   "execute command, skipping trial run that doesn't make any changes",
		Aliases: []string{"y", "no-dry-run"},
		Value:   false,
	}
	verboseFlag := &cli.BoolFlag{
		Name:    "verbose",
		Usage:   "increase verbosity",
		Aliases: []string{"v"},
	}
	concurrencyFlag := &cli.IntFlag{
		Name:    "max-concurrency",
		Aliases: []string{"j", "max_concurrency"},
		Value:   100,
		Usage:   "Limit the number of in-flight requests at once.",
	}
	progressFlag := &cli.StringFlag{
		Name:  "progress",
		Usage: "Show progress bars during sync. One of [auto, on, off]. 'auto' enables when stderr is a TTY.",
		Value: "auto",
		Action: func(_ *cli.Context, val string) error {
			switch val {
			case "auto", "on", "off":
				return nil
			default:
				return fmt.Errorf("unknown progress mode %q, must be one of: auto, on, off", val)
			}
		},
	}

	commonFlags := []cli.Flag{
		&cli.StringFlag{
			Name:  "cpu-profile",
			Usage: "Path to write a CPU profile on exit. If empty, no CPU profile is written.",
		},
		&cli.StringFlag{
			Name:  "mem-profile",
			Usage: "Path to write a memory profile on exit. If empty, no memory profile is written.",
		},
		&cli.StringFlag{
			Name:  "block-profile",
			Usage: "Path to write a block profile on exit. If empty, no block profile is written.",
		},
	}
	var cpuProfileFile *os.File
	before := func(appCtx *cli.Context) error {
		if path := appCtx.String("cpu-profile"); path != "" {
			f, err := os.Create(path)
			if err != nil {
				return fmt.Errorf("creating cpu profile: %w", err)
			}
			if err := pprof.StartCPUProfile(f); err != nil {
				f.Close()
				return fmt.Errorf("starting cpu profile: %w", err)
			}
			cpuProfileFile = f
		}
		return nil
	}
	after := func(appCtx *cli.Context) error {
		if cpuProfileFile != nil {
			pprof.StopCPUProfile()
			cpuProfileFile.Close()
		}
		if path := appCtx.String("mem-profile"); path != "" {
			f, err := os.Create(path)
			if err != nil {
				return fmt.Errorf("creating mem profile: %w", err)
			}
			defer f.Close()
			runtime.GC()
			if err := pprof.Lookup("allocs").WriteTo(f, 0); err != nil {
				return fmt.Errorf("writing mem profile: %w", err)
			}
		}
		if path := appCtx.String("block-profile"); path != "" {
			f, err := os.Create(path)
			if err != nil {
				return fmt.Errorf("creating block profile: %w", err)
			}
			defer f.Close()
			if err := pprof.Lookup("block").WriteTo(f, 0); err != nil {
				return fmt.Errorf("writing block profile: %w", err)
			}
		}
		return nil
	}

	return &cli.App{
		Name:                 appName,
		Version:              Version,
		Usage:                "Interact with the Spacetime APIs from the command line.",
		Description:          fmt.Sprintf("`%s` is a tool that allows you to interact with the Spacetime APIs from the command-line.", appName),
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
				Before:   before,
				After:    after,
				Flags:    commonFlags,
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
				Before:   before,
				After:    after,
				Flags:    commonFlags,
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
				Name:      "generate-keys",
				Usage:     "Generate RSA keys to use for authentication with the Spacetime APIs.",
				UsageText: "After creating the Private-Public keypair, you will need to request API access by sharing the `.crt` file (a self-signed x509 certificate containing the public key) with Aalyria to receive the `USER_ID` and a `KEY_ID` needed to complete the nbictl configuration. Only share the public certificate (`.crt`) with Aalyria or third-parties. The private key (`.key`) must be protected and should never be sent by email or communicated to others.",
				Before:    before,
				After:     after,
				Flags: slices.Concat(commonFlags, []cli.Flag{
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
				}),
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
						Before: before,
						After:  after,
						Flags:  commonFlags,
					},
					{
						Name:   "describe",
						Usage:  "Prints the NBI connection settings associated with the configuration profile given by the `--profile` flag (defaults to \"DEFAULT\").",
						Action: GetConfig,
						Before: before,
						After:  after,
						Flags:  commonFlags,
					},
					{
						Name:   "set",
						Usage:  "Sets or updates a configuration profile settings. You can create multiple profiles by specifying the `--profile` flag (defaults to \"DEFAULT\").",
						Before: before,
						After:  after,
						Flags: slices.Concat(commonFlags, []cli.Flag{
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
								Usage: "Spacetime endpoint specified as `host[:port]` (port is optional and defaults to 443). MUST NOT include any scheme (e.g. `https://` or `dns:///`) nor any API-specific prefix (e.g. `model-v1.`).",
							},
							&cli.StringFlag{
								Name:  "transport_security",
								Usage: "Transport security to use when connecting to the API endpoints. Allowed values: [insecure, system_cert_pool]",
							},
							&cli.StringFlag{
								Name:  "transport",
								Usage: "Transport protocol to use. Allowed values: [tcp, quic] (default: tcp)",
							},
							&cli.StringFlag{
								Name:  "auth_strategy",
								Usage: "Authentication strategy. Allowed values: [none, jwt, oidc]. When 'none', no authentication credentials are sent.",
							},
							&cli.StringFlag{
								Name:  "client_id",
								Usage: "OIDC client ID (used with --auth_strategy=oidc).",
							},
							&cli.StringFlag{
								Name:  "token_url",
								Usage: "OIDC token endpoint URL (used with --auth_strategy=oidc).",
							},
							&cli.StringFlag{
								Name:  "endpoint_config",
								Usage: "Endpoint resolution strategy. Allowed values: [subdomain, single_domain, custom]",
							},
							&cli.StringFlag{
								Name:  "model_url",
								Usage: "URL for the Model API (custom endpoint_config only).",
							},
							&cli.StringFlag{
								Name:  "status_url",
								Usage: "URL for the Status API (custom endpoint_config only).",
							},
							&cli.StringFlag{
								Name:  "provisioning_url",
								Usage: "URL for the Provisioning API (custom endpoint_config only).",
							},
							&cli.StringFlag{
								Name:  "default_url",
								Usage: "Fallback URL for unlisted services (custom endpoint_config only).",
							},
						}),
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
						Before:   before,
						After:    after,
						Flags:    slices.Concat(commonFlags, []cli.Flag{formatFlag}),
					},
					{
						Name:     "update-entity",
						Usage:    "Update the model NMTS Entity contained within the file provided on the command line ('-' reads from stdin).",
						Category: "model entities",
						Action:   ModelUpdateEntity,
						Before:   before,
						After:    after,
						Flags:    slices.Concat(commonFlags, []cli.Flag{formatFlag}),
						// TODO: support partial update by adding a flag to specify the update_mask
					},
					{
						Name:     "delete-entity",
						Usage:    "Delete the model NMTS Entity associated with the entity ID provided on the command line, along with any relationships in which it participates.",
						Category: "model entities",
						Action:   ModelDeleteEntity,
						Before:   before,
						After:    after,
						Flags:    slices.Concat(commonFlags, []cli.Flag{formatFlag}),
					},
					{
						Name:     "get-entity",
						Usage:    "Get the model NMTS Entity associated with the entity ID given on the command line.",
						Category: "model entities",
						Action:   ModelGetEntity,
						Before:   before,
						After:    after,
						Flags:    slices.Concat(commonFlags, []cli.Flag{formatFlag}),
					},
					{
						Name:     "create-relationship",
						Usage:    "Insert the model NMTS Relationship contained within the file provided on the command line ('-' reads from stdin).",
						Category: "model relationships",
						Action:   ModelCreateRelationship,
						Before:   before,
						After:    after,
						Flags:    slices.Concat(commonFlags, []cli.Flag{formatFlag}),
					},
					{
						Name:     "delete-relationship",
						Usage:    "Delete the model NMTS Relationship contained within the file provided on the command line ('-' reads from stdin).",
						Category: "model relationships",
						Action:   ModelDeleteRelationship,
						Before:   before,
						After:    after,
						Flags:    slices.Concat(commonFlags, []cli.Flag{formatFlag}),
					},
					{
						Name:     "upsert-fragment",
						Usage:    "Upsert the model NMTS Fragment contained within the file provided on the command line ('-' reads from stdin).",
						Category: "model relationships",
						Action:   ModelUpsertFragment,
						Before:   before,
						After:    after,
						Flags:    slices.Concat(commonFlags, []cli.Flag{formatFlag}),
					},
					{
						Name:     "list-entities",
						Usage:    "List all model entities.",
						Category: "model entities",
						Action:   ModelListEntities,
						Before:   before,
						After:    after,
						Flags:    slices.Concat(commonFlags, []cli.Flag{formatFlag}),
					},
					{
						Name:     "list-relationships",
						Usage:    "List all model relationships.",
						Category: "model relationships",
						Action:   ModelListRelationships,
						Before:   before,
						After:    after,
						Flags:    slices.Concat(commonFlags, []cli.Flag{formatFlag}),
						// TODO: support filter param
					},
					{
						Name:     "sync",
						Aliases:  []string{"rsync"},
						Usage:    "Sync all model entities and relationships from file and directory arguments.",
						Category: "model entities and relationships",
						Action:   ModelSync,
						Before:   before,
						After:    after,
						Flags: slices.Concat(commonFlags, []cli.Flag{
							formatFlag,
							dryrunFlag,
							concurrencyFlag,
							progressFlag,
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
						}),
					},
					{
						Name:     "delete-all",
						Aliases:  []string{"clear"},
						Usage:    "Delete all model entities and relationships from remote.",
						Category: "model entities and relationships",
						Action:   ModelDeleteAll,
						Before:   before,
						After:    after,
						Flags: slices.Concat(commonFlags, []cli.Flag{
							executeFlag,
							verboseFlag,
							concurrencyFlag,
							progressFlag,
						}),
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
						Before:   before,
						After:    after,
						Flags: slices.Concat(commonFlags, []cli.Flag{
							formatFlag,
							dryrunFlag,
							verboseFlag,
							concurrencyFlag,
							progressFlag,
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
						}),
					},
					{
						Name:     "list",
						Usage:    "List all provisioning resources from remote.",
						Category: "provisioning resources",
						Action:   ProvisioningList,
						Before:   before,
						After:    after,
						Flags: slices.Concat(commonFlags, []cli.Flag{
							formatFlag,
							dryrunFlag,
							verboseFlag,
						}),
					},
					{
						Name:      "delete",
						Usage:     "Delete provisioning resources from remote.",
						Category:  "provisioning resources",
						Action:    ProvisioningDelete,
						Args:      true,
						ArgsUsage: "[resource names]...",
						Before:    before,
						After:     after,
						Flags: slices.Concat(commonFlags, []cli.Flag{
							formatFlag,
							dryrunFlag,
							verboseFlag,
							concurrencyFlag,
						}),
					},
					{
						Name:     "delete-all",
						Aliases:  []string{"clear"},
						Usage:    "Delete all provisioning resources from remote.",
						Category: "provisioning resources",
						Action:   ProvisioningDeleteAll,
						Before:   before,
						After:    after,
						Flags: slices.Concat(commonFlags, []cli.Flag{
							executeFlag,
							verboseFlag,
							concurrencyFlag,
							progressFlag,
						}),
					},
				},
			},
			{
				Name:  "status-v1",
				Usage: "Subcommands for Status API v1.",
				Subcommands: []*cli.Command{
					{
						Name:   "get-version",
						Usage:  "Retrieve the version of this Spacetime instance.",
						Action: StatusGetVersion,
						Before: before,
						After:  after,
						Flags: slices.Concat(commonFlags, []cli.Flag{
							formatFlag,
						}),
					},
					{
						Name:   "get-metrics",
						Usage:  "Retrieve the insight metrics of this Spacetime instance.",
						Action: StatusGetMetrics,
						Before: before,
						After:  after,
						Flags: slices.Concat(commonFlags, []cli.Flag{
							formatFlag,
						}),
					},
				},
			},
			{
				Name:  "grpcurl",
				Usage: "Provides curl-like equivalents for interacting with the Spacetime APIs.",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "api",
						Usage: "The API endpoint to connect to (e.g. 'model-v1', 'provisioning-v1alpha').",
					},
				},
				Subcommands: []*cli.Command{
					{
						Name:   "describe",
						Usage:  "Takes an optional fully-qualified symbol (service, enum, or message). If provided, the descriptor for that symbol is shown. If not provided, the descriptor for all exposed or known services are shown.",
						Action: GRPCDescribe,
						Before: before,
						After:  after,
						Flags:  commonFlags,
					},
					{
						Name:   "list",
						Usage:  "Takes an optional fully-qualified service name. If provided, lists all methods of that service. If not provided, all exposed services are listed.",
						Action: GRPCList,
						Before: before,
						After:  after,
						Flags:  commonFlags,
					},
					{
						Name:    "call",
						Aliases: []string{"invoke"},
						Usage:   "Takes a fully-qualified method name in 'service.method' or 'service/method' format. Invokes the method using the provided request body.",
						Action:  GRPCCall,
						Before:  before,
						After:   after,
						Flags: slices.Concat(commonFlags, []cli.Flag{
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
						}),
					},
				},
			},
			{
				Name:   "generate-auth-token",
				Usage:  "Generate a self-signed JWT token for API authentication.",
				Before: before,
				After:  after,
				Flags: slices.Concat(commonFlags, []cli.Flag{
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
				}),
				Action: GenerateAuthToken,
			},
		},
	}
}

func validateProtoFormat(_ *cli.Context, f string) error {
	switch f {
	case "text", "json", "binary":
		return nil
	default:
		return fmt.Errorf("unknown format %q", f)
	}
}

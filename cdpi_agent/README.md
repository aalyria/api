# agent

A Go implementation of a CDPI agent.

## Overview

The Control-to-Data-Plane Interface (CDPI) works by exchanging
[Protocol Buffers](https://protobuf.dev/), a language-agnostic format and toolchain for serializing
messages, transmitted using [gRPC](https://grpc.io/), a performant RPC framework with many
sophisticated features.

This directory provides two important pieces for interacting with the Spacetime CDPI:

- The `agent` (`//cdpi_agent/cmd/agent`), a Go binary that handle the CDPI protocol and
  authentication details while delegating the actual implementation of enactments to a
  user-configured external process. While the details of the CDPI protocol are still subject to
  change, the command line interface for these binaries is intended to be significantly more stable
  and provide an easy to develop against abstraction that insulates platform integrators from the
  majority of those changes.

- The `cdpi_agent` library (`//cdpi_agent`), a Go package that provides a growing set of
  abstractions for writing a new CDPI agent. This library is subject to change alongside the CDPI
  protocol, so platform integrators are encouraged to use the `agent` binary until the underlying
  APIs reach a stable milestone.

## Building

This repo uses the [bazel](https://bazel.build/) build system. Once you have a copy of `bazel` in
your `$PATH`, running `bazel build //cdpi_agent/cmd/agent` will build the Go binary. Similarly,
running `bazel build //cdpi_agent` will build the Go library.

For a full list of available build targets, you can use `bazel query`:

```bash
bazel query //cdpi_agent/...:all
```

## Getting started with the `agent` binary

### Configuration

The `agent` binary accepts configuration in the form of a protobuf message, documented in
[config.proto](cdpi_agent/cmd/agent/config.proto). The message can be encoded in
[prototext format](https://protobuf.dev/reference/protobuf/textformat-spec/) (human readable and
writable), [json](https://protobuf.dev/programming-guides/proto3/#json), or the
[binary proto format](https://protobuf.dev/programming-guides/encoding/). Most users will find the
prototext format the easiest to use, and as such it's the default.

More details for each aspect of the configuration are provided below, but a simple configuration
file might look like this:

```textproto
connection_params: {
  # TODO: change to point to the domain of your spacetime instance
  cdpi_endpoint: "dns:///my_instance.spacetime.aalyria.com"

  transport_security: {
    system_cert_pool: {}
  }

  auth_strategy: {
    jwt: {
      # TODO: change to the domain of your spacetime instance
      audience: "my_instance.spacetime.aalyria.com"
      # TODO: use the email your Aalyria representative will share with you
      email: "my-cdpi-agent@example.com"
      # TODO: use the private key ID your Aalyria representative will share with you
      private_key_id: "BADDB0BACAFE"
      signing_strategy: {
	# TODO: change to the path of your PEM-encoded RSA private key
        private_key_file: "/path/to/agent/private/key.pem"
      }
    }
  }
}

# each network_node is configured with a stanza like so:
network_nodes: {
  id: "node-a"
  state_backend: {
    static_initial_state: {}
  }
  enactment_backend: {
    external_command: {
      # while each command invocation will receive a node ID as part of the
      # enactment request, you can also pass additional arguments here to help
      # integrate with your own systems
      args: "/usr/local/bin/do_enactments"
      args: "--node=a"
      args: "--format=json"
      # Encode enactment requests as JSON to the process's standard input and
      # expect any new state messages to be written as JSON to the process's
      # standard output (this is the default)
      proto_format: JSON
    }
  }
}

network_nodes: {
  id: "node-b"
  state_backend: {
    static_initial_state: {}
  }
  enactment_backend: {
    external_command: {
      args: "/usr/local/bin/some_other_enactment_cmd"
      # Use the protobuf binary format for encoding both stdin and stdout messages.
      proto_format: WIRE
    }
  }
}
```

See the documentation in [config.proto](cdpi_agent/cmd/agent/config.proto) for more details on the
available options. You can use the `--dry-run` flag to check that your configuration is valid:

```bash
bazel run //cdpi_agent/cmd/agent -- --log-level trace --config "$PWD/config.textproto" --dry-run
INFO: Invocation ID: d8a4b02f-1e01-47cb-bd26-5366704165af
INFO: Analyzed target //cdpi_agent/cmd/agent:agent (0 packages loaded, 0 targets configured).
INFO: Found 1 target...
Target //cdpi_agent/cmd/agent:agent up-to-date:
  bazel-bin/cdpi_agent/cmd/agent/agent_/agent
INFO: Elapsed time: 1.768s, Critical Path: 1.56s
INFO: 3 processes: 1 internal, 2 linux-sandbox.
INFO: Build completed successfully, 3 total actions
INFO: Running command line: bazel-bin/cdpi_agent/cmd/agent/agent_/agent --log-level trace --config /path/to/config.textproto --format text --dry-run
2023-04-19 11:57:01AM INF config is valid
```

### Authentication

The agent uses signed [JSON Web Tokens (JWTs)](https://www.rfc-editor.org/rfc/rfc7519) to
authenticate with the CDPI service. The JWT needs to be signed using an RSA private key with a
corresponding public key that's been shared - inside of a self-signed x509 certificate - with the
Aalyria team.

#### Creating a test keypair

For testing purposes, you can generate a valid key using the `openssl` tool:

```bash
# generate a private key of size 4096 and save it to agent_priv_key.pem
openssl genrsa -out agent_priv_key.pem 4096
# extract the public key and save it to an x509 certificate named
# agent_pub_key.cer (with an expiration far into the future)
openssl req -new -x509 -key agent_priv_key.pem -out agent_pub_key.cer -days 36500
```

### Starting the agent

Assuming you've saved your configuration in a file called `config.textproto`, you can use the
following command to start the agent:

```bash
bazel run //cdpi_agent/cmd/agent -- --config "$PWD/my_config.textproto" --log-level debug
```

NOTE: `bazel run` changes the working directory of the process, so you'll need
to use absolute paths to point to the config file.

If the agent was able to authenticate correctly, you should see something like this appear as output
(requires `--log-level` be "debug" or "trace"):

```
2023-04-18 08:44:48PM INF starting agent
2023-04-18 08:44:48PM DBG node controller starting nodeID=Atlantis-groundstation
```

## Next steps

### Writing a custom extproc enactment backend

Writing a custom enactment backend using the `agent` is relatively simple as the agent takes care of
the CDPI protocol details, including timing and error reporting. When the agent receives a scheduled
control update, it invokes the configured external process, writes the incoming
`ScheduledControlUpdate` message in the encoding format of your choice to the process's stdin, and
optionally reads a new `ControlPlaneState` message (using the same encoding format) from the
process's stdout.

- If nothing is written to stdout and the process terminates with an exit code of 0, the enactment
  is considered successful and the node state is assumed to have been updated to match the change.

- If anything goes wrong during the enactment (indicated by a non-zero exit code), the process's
  stderr and exit code are combined to form a gRPC status which is conveyed back to the CDPI
  endpoint as the (failing) result of the enactment.

Since the external process only needs to be able to encode and decode JSON, it's trivial to write
the platform-specific logic in whatever language best suits the task. Included in this repo are some
sample programs that demonstrate basic error handling and message parsing in different languages:

<!-- TODO: add a go example here -->

- examples/enact_flow_forward_updates.py: A python script that reads the input messages as ad-hoc
  JSON, implements some basic error handling, and demonstrates how one might go about enacting flow
  updates (the actual logic for forwarding packets is left as an exercise for the reader).

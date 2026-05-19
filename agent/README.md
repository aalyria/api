# agent

A Go implementation of an SBI agent.

## Overview

The Southbound Interface (SBI) works by exchanging
[Protocol Buffers](https://protobuf.dev/), a language-agnostic format and toolchain for serializing
messages, transmitted using [gRPC](https://grpc.io/), a performant RPC framework with many
sophisticated features. The SBI comprises a variety of APIs, the principle one being the
Scheduling API whereby Spacetime sends scheduled state changes to SDN agents.

This directory provides two important pieces for interacting with the Spacetime SBI:

- The `agent` (`//agent/cmd/agent`), a Go binary that handle the SBI protocols and
  authentication details while delegating the actual implementation of enactments to a
  user-configured external process. While the details of SBI protocols are still subject to
  change, the command line interface for these binaries is intended to be significantly more stable
  and provide an easy to develop against abstraction that insulates platform integrators from the
  majority of those changes.

- The `agent` library (`//agent`), a Go package that provides a growing set of
  abstractions for writing a new SBI agent. This library is subject to change alongside the SBI
  protocols, so platform integrators are encouraged to use the `agent` binary until the underlying
  APIs reach a stable milestone.

## Building

This repo uses the [bazel](https://bazel.build/) build system. Once you have a copy of `bazel` in
your `$PATH`, running `bazel build //agent/cmd/agent` will build the Go binary. Similarly,
running `bazel build //agent` will build the Go library.

For a full list of available build targets, you can use `bazel query`:

```bash
bazel query //agent/...:all
```

## Getting started with the `agent` binary

### Configuration

The `agent` binary accepts configuration in the form of a protobuf message, documented in
[config.proto](internal/configpb/config.proto). The message can be encoded in
[prototext format](https://protobuf.dev/reference/protobuf/textformat-spec/) (human readable and
writable), [json](https://protobuf.dev/programming-guides/proto3/#json), or the
[binary proto format](https://protobuf.dev/programming-guides/encoding/). Most users will find the
prototext format the easiest to use, and as such it's the default.

More details for each aspect of the configuration are provided below, but a simple configuration
file might look like this:

```textproto
# each SDN agent is configured with a stanza like so:
sdn_agents: {
  # TODO: replace ${AGENT_ID_1} with the specific SDN Agent ID.
  id: "${AGENT_ID_1}"

  enactment_driver: {
    connection_params: {
      # TODO: replace ${DOMAIN} to the domain of your spacetime instance
      endpoint_uri: "scheduling-v1alpha.${DOMAIN}"

      transport_security: { system_cert_pool: {} }

      auth_strategy: {
        jwt: {
          # TODO: replace ${DOMAIN} to the domain of your spacetime instance
          audience: "scheduling-v1alpha.${DOMAIN}"
          # TODO: use the email your Aalyria representative will share with you
          email: "my-sdn-agent@example.com"
          # TODO: use the private key ID your Aalyria representative will share with you
          private_key_id: "BADDB0BACAFE"
          signing_strategy: {
            # TODO: change to the path of your PEM-encoded RSA private key
            private_key_file: "/path/to/agent/private/key.pem"
          }
        }
      }
    }

    external_command: {
      args: "bash"
      args: "-c"
      args: "jq -c . >> /tmp/AGENT_ID_2.enactments.json"
      # Encode enactment requests as JSON to the process's standard input (this
      # is the default)
      proto_format: JSON
    }
  }
}

sdn_agents: {
  # TODO: replace ${AGENT_ID_2} with the specific SDN Agent ID.
  id: "${AGENT_ID_2}"

  enactment_driver: {
    connection_params: {
      # TODO: replace ${DOMAIN} to the domain of your spacetime instance
      endpoint_uri: "scheduling-v1alpha.${DOMAIN}"

      transport_security: { system_cert_pool: {} }

      auth_strategy: {
        jwt: {
          # TODO: replace ${DOMAIN} to the domain of your spacetime instance
          audience: "scheduling-v1alpha.${DOMAIN}"
          # TODO: use the email your Aalyria representative will share with you
          email: "my-sdn-agent@example.com"
          # TODO: use the private key ID your Aalyria representative will share with you
          private_key_id: "BADDB0BACAFE"
          signing_strategy: {
            # TODO: change to the path of your PEM-encoded RSA private key
            private_key_file: "/path/to/agent/private/key.pem"
          }
        }
      }
    }

    external_command: {
      # while each command invocation will receive a node ID as part of the
      # enactment request, you can also pass additional arguments here to help
      # integrate with your own systems
      args: "/usr/local/bin/do_enactments"
      args: "--node=a"
      args: "--format=wirepb"
      proto_format: WIRE
    }
  }
}
```

See the documentation in [config.proto](internal/configpb/config.proto) for more details on the
available options. You can use the `--dry-run` flag to check that your configuration is valid:

```bash
bazel run //agent/cmd/agent -- --log-level trace --config "$PWD/config.textproto" --dry-run
INFO: Invocation ID: d8a4b02f-1e01-47cb-bd26-5366704165af
INFO: Analyzed target //agent/cmd/agent:agent (0 packages loaded, 0 targets configured).
INFO: Found 1 target...
Target //agent/cmd/agent:agent up-to-date:
  bazel-bin/agent/cmd/agent/agent_/agent
INFO: Elapsed time: 1.768s, Critical Path: 1.56s
INFO: 3 processes: 1 internal, 2 linux-sandbox.
INFO: Build completed successfully, 3 total actions
INFO: Running command line: bazel-bin/agent/cmd/agent/agent_/agent --log-level trace --config /path/to/config.textproto --format text --dry-run
2023-04-19 11:57:01AM INF config is valid
```

### Authentication

The agent uses signed [JSON Web Tokens (JWTs)](https://www.rfc-editor.org/rfc/rfc7519) to
authenticate with the SBI service. The JWT needs to be signed using an RSA private key with a
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
bazel run //agent/cmd/agent -- --config "$PWD/my_config.textproto" --log-level debug
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

<!-- TODO: this needs a more thoughtful rewrite. -->

Writing a custom enactment backend using the `agent` is relatively simple as
the agent takes care of the SBI [Scheduling API](../api/scheduling/) protocol
details, including timing and error reporting. When the agent is due to enact a
scheduled configuration change, it invokes the configured external process,
writes the `CreateEntryRequest` message in the encoding format of your choice
to the process's stdin, and examines the process's exit code.

- If the process terminates with an exit code of 0, the enactment
  is considered successful and the commanded element's state is assumed to have been updated to match the change.

- If anything goes wrong during the enactment (indicated by a non-zero exit code), the process's
  exit code is used to form an error message which is logged by the agent. The exact state
  in which the commanded element was left may be reported via the [Telemetry API](../api/telemetry/).

Since the external process only needs to be able to encode and decode JSON, it's trivial to write
the platform-specific logic in whatever language best suits the task.

## Operational notes

### Time synchronization

All agent implementations and instantiations MUST have some means to maintain time synchronization.
Ideally an agent's time would be synchronzied to the same ultimate source(s) used by Spacetime itself, though this is not a strict requirement.

RECOMMENDED time synchronization mechanisms include:
* GPS or another Global Navigation Satellite System (GNSS) PNT service
* Network Time Protocol v4 ([NTPv4](https://www.rfc-editor.org/rfc/rfc5905.html)), ideally with Network Time Security ([NTS](https://www.rfc-editor.org/rfc/rfc8915.html))
* IEEE Precision Time Protocol (PTP, [IEEE 1588-2019](https://ieeexplore.ieee.org/document/9120376))

Some experimental time synchronization mechanisms include:
* Network Time Protocol v5 ([draft](https://datatracker.ietf.org/doc/draft-ietf-ntp-ntpv5/))
* roughtime ([draft](https://datatracker.ietf.org/doc/draft-ietf-ntp-roughtime/))

An agent SHOULD maintain a sub-second difference from its chosen time source(s), and all RECOMMENDED time synchronization mechanisms can in principle achieve much better accuracies.
The exact requirements and realistically achievable accuracies, however, are specific to a given deployment scenario.

If an agent instance or its underlying platform cannot maintain adequate time synchronization, for a mission-specific definition of "adequate", then it MUST do one of the following:
* include notification of the loss of adequate time synchronization with any reported telemetry, or
* not report telemetry (information is too untrustworthy to be useful).

An agent MAY continue to enact Scheduling API commands, especially if doing so might lead to restoring adequate time synchronization (e.g. restoration of a data plane connection over which a networked time protocol's messages are forwarded).

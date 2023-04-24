# extproc_agent

The `extproc_agent` is a simple to use CDPI agent that accepts most of its configuration via command
line flags.

NOTE: Most users are encouraged to use the [`agent`](../agent) binary instead. It offers an easier
to use configuration and more fine-grained control over individual parameters. See the
[README](../../README.md) for more information on how to configure the `agent`.

## Building

This repo uses the [bazel](https://bazel.build/) build system. Once you have a copy of `bazel` in
your `$PATH`, running `bazel build //cdpi_agent/cmd/extproc_agent` will build the Go binary.
Similarly, running `bazel build //cdpi_agent` will build the Go library.

## Getting started with the `extproc_agent`

### Authentication

The extproc agent uses signed [JSON Web Tokens (JWTs)](https://www.rfc-editor.org/rfc/rfc7519) to
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

#### Creating test JWTs

Once you have your private key and you've shared the public key with Aalyria, you can create a
signed JWT to authorize the agent. To facilitate testing agent authorization, this repo includes a
small Go program
([`tools/generate_jwt/generate_jwt.go`](../../../tools/generate_jwt/generate_jwt.go)) that can be
used to generate signed JWTs.

While the CDPI APIs are in alpha, authorization requires two *different* JWTs:

- A Spacetime JWT, passed in the `Authorization` header / `--authorization-jwt` flag and referenced
  below as `SPACETIME_AUTH_JWT`

- A supplementary JWT consumed by the CDPI service's secure proxy, passed in the
  `Proxy-Authorization` header / `--proxy-authorization-jwt` flag and referenced below as
  `PROXY_AUTH_JWT`

NOTE: Your contact on the Aalyria team will be able to provide the full values for the below
`$AGENT_EMAIL`, `$CDPI_DOMAIN`, and `$AGENT_PRIV_KEY_ID` variables.

Using the included `generate_jwt.go` program to create these tokens is simple.

```bash
# Customer-specific details:
CDPI_DOMAIN="${CDPI_DOMAIN:?should be provided by your Aalyria contact}"
AGENT_EMAIL="${AGENT_EMAIL:?should be provided by your Aalyria contact}"
AGENT_PRIV_KEY_ID="${AGENT_PRIV_KEY_ID:?should be provided by your Aalyria contact}"
# This is the "agent_priv_key.pem" file from above:
AGENT_PRIV_KEY_FILE="/path/to/your/private/key/in/PKSC8/format.pem"

# Spacetime-specific details:
PROXY_AUDIENCE="https://www.googleapis.com/oauth2/v4/token"
PROXY_TARGET_AUDIENCE="60292403139-me68tjgajl5dcdbpnlm2ek830lvsnslq.apps.googleusercontent.com"

SPACETIME_AUTH_JWT=$(bazel run //tools/generate_jwt \
  -- \
  --issuer "$AGENT_EMAIL" \
  --subject "$AGENT_EMAIL" \
  --audience "$CDPI_DOMAIN" \
  --key-id "$AGENT_PRIV_KEY_ID" \
  --private-key "$AGENT_PRIV_KEY_FILE")

PROXY_AUTH_JWT=$(bazel run //tools/generate_jwt \
  -- \
  --issuer "$AGENT_EMAIL" \
  --subject "$AGENT_EMAIL" \
  --audience "$PROXY_AUDIENCE" \
  --target-audience "$PROXY_TARGET_AUDIENCE" \
  --key-id "$AGENT_PRIV_KEY_ID" \
  --private-key "$AGENT_PRIV_KEY_FILE")
  CDPI_ENDPOINT="dns:///$CDPI_DOMAIN"
```

### Starting the agent

To run the extproc agent we'll need a backend command that the agent will run to handle enactments.
`true` (as in `/usr/bin/true`) is a great no-op choice for checking that authentication is working
and the agent is able to connect to the CDPI endpoint. To make things easier, we'll use a couple
more environment variables to hold our configuration values:

```
NODE_ID="${NODE_ID:?should be provided by your Aalyria contact}"
ENACTMENT_BACKEND=(true)
# could also be "trace" for more details or "info" for fewer:
LOG_LEVEL=debug
```

Now we're ready to run the agent (this assumes the `$SPACETIME_AUTH_JWT` and `$PROXY_AUTH_JWT`
variables are set as above):

```bash
bazel run //cdpi_agent/cmd/extproc_agent \
  --\
  --cdpi-endpoint "$CDPI_ENDPOINT" \
  --node "$NODE_ID" \
  --log-level "$LOG_LEVEL" \
  --authorization-jwt "$SPACETIME_AUTH_JWT" \
  --proxy-authorization-jwt "$PROXY_AUTH_JWT" \
  -- \
  "${ENACTMENT_BACKEND[@]}"
```

If the agent was able to authenticate correctly, you should see something like this appear as output
(requires `$LOG_LEVEL` be "debug" or "trace"):

```
11:00AM DBG registered node nodeID=Atlantis-groundstation
11:00AM DBG entering control loop nodeID=Atlantis-groundstation
```

## Next steps

### Writing a custom extproc enactment backend

Writing a custom enactment backend using the `extproc_agent` is relatively simple as the
extproc_agent takes care of the CDPI protocol details, including timing and error reporting. When
the agent receives a scheduled control update, it invokes the configured external process, writes
the incoming `ScheduledControlUpdate` message as JSON (using the `protojson` encoding) to the
process's stdin, and optionally reads a new `ControlPlaneState` message as JSON from the process's
stdout.

### Creating your own JWTs

In production, you'll likely want to use a Trusted Platform Module (TPM) to keep your private key
secure when signing the authorization JWTs. While the platform-specific details of generating signed
JWTs using a TPM are outside the scope of this project, the fields for both JWTs are listed below:

| Field             | Spacetime JWT                   | Proxy JWT                                                                 |
| ---------------   | ------------------------------- | -----------------------------------------------------------------------   |
| algorithm (`alg`) | `RS256`                         | `RS256`                                                                   |
| issuer (`iss`)    | `$AGENT_EMAIL`                  | `$AGENT_EMAIL`                                                            |
| subject (`sub`)   | `$AGENT_EMAIL`                  | `$AGENT_EMAIL`                                                            |
| audience (`aud`)  | `$CDPI_DOMAIN`                  | `https://www.googleapis.com/oauth2/v4/token`                              |
| key ID (`kid`)    | `$AGENT_PRIV_KEY_ID`            | `$AGENT_PRIV_KEY_ID`                                                      |
| lifetime (`exp`)  | Long-lived                      | Short-lived (now + `1h`)                                                  |
| `target_audience` | `N/A`                           | `60292403139-me68tjgajl5dcdbpnlm2ek830lvsnslq.apps.googleusercontent.com` |


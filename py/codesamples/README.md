# Northbound Interface (NBI) Client Code Samples

These binaries are sample implementations of how to interact with Spacetime's
NBI.

Start by creating a public and private key pair (see instructions [here](https://docs.spacetime.aalyria.com/authentication)).
Share the public key certificate with your contact on the Aalyria team, and they will provide you with a private key ID for 
your application.

```sh
    DOMAIN="${DOMAIN:?should be provided by your Aalyria contact}"
    AGENT_EMAIL="${AGENT_EMAIL:?should be provided by your Aalyria contact}"
    AGENT_PRIV_KEY_ID="${AGENT_PRIV_KEY_ID:?should be provided by your Aalyria contact}"
    # This is the path to your private key.
    AGENT_PRIV_KEY_FILE="/path/to/your/private/key/in/PKSC8/format.pem"
```

To run the `list_entities` sample, run:
```sh
    bazel run //py/codesamples:list_entities -- "$DOMAIN" "$AGENT_EMAIL" "$AGENT_PRIV_KEY_ID" "$AGENT_PRIV_KEY_FILE"
```

To run the `create_user_terminal_platform_definition` sample, run:
```sh
    bazel run //py/codesamples:create_user_terminal_platform_definition -- "$DOMAIN" "$AGENT_EMAIL" "$AGENT_PRIV_KEY_ID" "$AGENT_PRIV_KEY_FILE"
```

To run the `create_user_terminal_platform_definition_from_json` sample with the platform definition in `resources/user_terminal_platform_definition.json`, run:
```sh
    bazel run //py/codesamples:create_user_terminal_platform_definition_from_json -- "$DOMAIN" "$AGENT_EMAIL" "$AGENT_PRIV_KEY_ID" "$AGENT_PRIV_KEY_FILE"
```
Or, to provide your own JSON file, run:
```sh
    bazel run //py/codesamples:create_user_terminal_platform_definition_from_json -- "$DOMAIN" "$AGENT_EMAIL" "$AGENT_PRIV_KEY_ID" "$AGENT_PRIV_KEY_FILE" "/path/to/your/json/platform/definition/file.json"
```
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

To run the `ListEntities` sample, run:
```sh
    bazel run //github/code_samples/java_samples/src/main/java/com/aalyria/spacetime/nbi/sample_client:ListEntities "$DOMAIN" "$AGENT_EMAIL" "$AGENT_PRIV_KEY_ID" "$AGENT_PRIV_KEY_FILE"
```
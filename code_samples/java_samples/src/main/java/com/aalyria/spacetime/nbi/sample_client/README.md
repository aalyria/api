# Northbound Interface (NBI) Client Code Samples

These binaries are sample implementations of how to interact with Spacetime's
NBI using 2 JWTs for authentication, a SPACETIME_AUTH_JWT to authenticate to Spacetime's backend 
and a PROXY_AUTH_JWT to authenticate through the secure proxy.

Start by creating a public and private key pair (see instructions [here](https://docs.spacetime.aalyria.com/authentication)).
Once you have shared the public key with your contact on the Aalyria team, 
you can use the included tool in [tools/generate_jwt](/tools/generate_jwt) to generate these JWTs.

```sh
    DOMAIN="${DOMAIN:?should be provided by your Aalyria contact}"
    AGENT_EMAIL="${AGENT_EMAIL:?should be provided by your Aalyria contact}"
    AGENT_PRIV_KEY_ID="${AGENT_PRIV_KEY_ID:?should be provided by your Aalyria contact}"
    # This is the path to your private key.
    AGENT_PRIV_KEY_FILE="/path/to/your/private/key/in/PKSC8/format.pem"
    
    # Spacetime-specific details:
    PROXY_AUDIENCE="https://www.googleapis.com/oauth2/v4/token"
    PROXY_TARGET_AUDIENCE="60292403139-me68tjgajl5dcdbpnlm2ek830lvsnslq.apps.googleusercontent.com"

    SPACETIME_AUTH_JWT=$(bazel run //tools/generate_jwt \
    -- \
    --issuer "$AGENT_EMAIL" \
    --subject "$AGENT_EMAIL" \
    --audience "$DOMAIN" \
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
```

To run the `ListEntities` sample, run:
```sh
    bazel run //code_samples/java_samples/src/main/java/com/aalyria/spacetime/nbi/sample_client:ListEntities $DOMAIN 
        $SPACETIME_AUTH_JWT $PROXY_AUTH_JWT
```
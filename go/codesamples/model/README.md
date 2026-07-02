# Model API Client Code Samples

These binaries are sample implementations of how to interact with Spacetime's
Model API.

Start by creating a public and private key pair (see instructions [here](https://docs.spacetime.aalyria.com/authentication)).
Share the public key certificate with your contact on the Aalyria team, and they will provide you with a private key ID for 
your application.

```sh
# Set these variables with values provided by your Aalyria contact
export URL="api.example.com"  # Should be provided by your Aalyria contact
export EMAIL="your-email@example.com"  # Should be provided by your Aalyria contact
export KEY_ID="your-key-id"  # Should be provided by your Aalyria contact
export PRIVATE_KEY_PATH="/path/to/your/private-key.pem"  # Replace with actual path
```

To run the `model_client` sample, run:
```sh
bazel run //github/go/codesamples/model:model_client -- -target "$URL" -email "$EMAIL" -key_id "$KEY_ID" -private_key_path "$PRIVATE_KEY_PATH"
```

# How to ude the `generate_jwt.py` file
The `generate_jwt.py` generates two JWTS:
- One "standard"
- One for the reflections service (used to interact with the APIs by using `grpcurl`)

## How to use it
First, you have to have your private key file in the same folder of the Python file and call it `private_key.pem`.

Then, install the requirements:
```bash
pip install -r requirements.txt
```

You have to fill in the following data:

- `user_id` -> Your contact at aalyria gives it to you after you give them your public key.

- `domain` -> Your contact at aalyria gives it to you after you give them your public key.

- `grpc_service` -> Suppose you want to use the `NetOps` service in the [North Bound API](https://github.com/aalyria/api/blob/main/api/nbi/v1alpha/nbi.proto). In this case, you have to write: `grpc_service` = "aalyria.spacetime.api.nbi.v1alpha.NetOps"

- `grpc_method` -> Suppose you want to use tne `VersionInfo` method in the service `NetOps` (that returns the version of the API.) In this case, you have to write: `grpc_method` = "VersionInfo"

- `key_id` -> Your contact at aalyria gives it to you after you give them your public key.


Finally, in the reflection payload you currently find the following:

```python
reflect_jwt_payload = {
    "iss": user_id,
    "sub": user_id,
    "aud": f"https://{domain}/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo",
    "exp": datetime.datetime.utcnow() + datetime.timedelta(hours=1),
    "iat": datetime.datetime.utcnow()
}
```

You have to change the `aud` field by asking your contact at aalyria to give you the correct value for you.


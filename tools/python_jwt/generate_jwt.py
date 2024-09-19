''' 
Copyright 2023 Aalyria Technologies, Inc., and its affiliates.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
'''


import jwt
import datetime

# Your private key
with open('private_key.pem', 'r') as key_file:
    private_key = key_file.read()

# Your data
user_id = ""
domain = "nbi.demo.internal.spacetime.aalyria.com"
grpc_service = "aalyria.spacetime.api.nbi.v1alpha.NetOps"
grpc_method = "VersionInfo"
key_id = ""

## JWT
# Payload
payload = {
    "iss": user_id,
    "sub": user_id,
    "aud": f"https://{domain}/{grpc_service}/{grpc_method}",
    "exp": datetime.datetime.utcnow() + datetime.timedelta(hours=1),
    "iat": datetime.datetime.utcnow()
}

# Header
headers = {
    "alg": "RS256",
    "kid": key_id
}

# Generate il JWT
token = jwt.encode(payload, private_key, algorithm="RS256", headers=headers)

## REFLECT JWT
reflect_jwt_payload = {
    "iss": user_id,
    "sub": user_id,
    "aud": f"https://{domain}/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo",
    "exp": datetime.datetime.utcnow() + datetime.timedelta(hours=1),
    "iat": datetime.datetime.utcnow()
}

# Create JWT reflect
reflect_jwt = jwt.encode(
    reflect_jwt_payload,
    private_key,
    algorithm="RS256",
    headers=headers
)

# Append the JWTs to a new txt file
with open('jwt_tokens.txt', 'w') as file:
    file.write(f"JWT: {token}\n")
    file.write(f"REFLECT JWT: {reflect_jwt}\n")

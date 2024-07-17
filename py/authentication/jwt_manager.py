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

import base64
import json
from cryptography.hazmat.primitives.asymmetric import rsa, padding
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives import serialization
from datetime import timedelta, datetime, timezone


class JwtManager:

    def __init__(
        self,
        lifetime=timedelta(seconds=0),
        audience="",
        issuer="",
        subject="",
        target_audience="",
        private_key_id="",
        private_key="",
        jwt_value="",
    ):
        self.lifetime = lifetime
        self.audience = audience
        self.issuer = issuer
        self.subject = subject if subject else issuer
        self.target_audience = target_audience
        self.private_key_id = private_key_id
        self.private_key = private_key
        self.jwt_value = jwt_value if jwt_value else self.generate_jwt()

    def generate_jwt(self) -> str:
        private_key = self.decode_private_key()
        header = self.generate_header()
        payload = self.generate_payload()
        signature = self.generate_signature(header, payload, private_key)

        jwt = b".".join([header, payload, signature])
        jwt_value = jwt.decode()
        self.jwt_value = jwt_value
        return jwt_value

    def decode_private_key(self) -> rsa.RSAPrivateKey:
        private_key_pem = self.private_key.encode("utf-8")
        try:
            private_key = serialization.load_pem_private_key(
                private_key_pem, None)
        except (ValueError, TypeError) as e:
            raise ValueError("Invalid private key.") from e

        return private_key

    def generate_header(self) -> bytes:
        header = {
            "alg": "RS256",
            "typ": "JWT",
            "kid": self.private_key_id,
        }
        encoded_header = json.dumps(header).encode()
        return base64.urlsafe_b64encode(encoded_header)

    def generate_payload(self) -> bytes:
        now = datetime.now(timezone.utc)
        issue_time = int(now.timestamp())
        expiration_time = int((now + self.lifetime).timestamp())

        payload = {
            "aud": self.audience,
            "exp": expiration_time,
            "iat": issue_time,
            "iss": self.issuer,
            "sub": self.subject,
        }
        if self.target_audience:
            payload["target_audience"] = self.target_audience

        encoded_payload = json.dumps(payload).encode()
        return base64.urlsafe_b64encode(encoded_payload)

    def generate_signature(
        self,
        header: bytes,
        payload: bytes,
        private_key: rsa.RSAPrivateKey,
    ) -> bytes:
        message = b".".join([header, payload])
        signature = private_key.sign(
            message,
            padding.PKCS1v15(),
            hashes.SHA256(),
        )
        return base64.urlsafe_b64encode(signature)

    def get_jwt(self) -> str:
        return self.jwt_value

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

import grpc
import http.client
import json
from urllib.parse import urlencode
from datetime import timedelta, datetime, timezone

from py.authentication.jwt_manager import JwtManager


class SpacetimeCallCredentials(grpc.AuthMetadataPlugin):
    """A class that supplies per-RPC credentials, which are based on
    two signed JWTs, one for authenticating to the Spacetime backend
    and one for authenticating through the secure proxy.
    """

    # OIDC tokens are provided by Google's OAuth endpoint by default.
    DEFAULT_GCP_OIDC_TOKEN_CREATION_HOST = "www.googleapis.com"
    DEFAULT_GCP_OIDC_TOKEN_CREATION_PATH = "/oauth2/v4/token"
    # The OIDC tokens are valid for 1 hour by default.
    DEFAULT_OIDC_TOKEN_LIFETIME = timedelta(hours=1)
    PROXY_TARGET_AUDIENCE = "60292403139-me68tjgajl5dcdbpnlm2ek830lvsnslq.apps.googleusercontent.com"
    # If the OIDC token will expire within this margin, it will be re-created.
    OIDC_TOKEN_EXPIRATION_TIME_MARGIN = timedelta(minutes=5)

    def __init__(
        self,
        spacetime_auth_jwt_manager,
        proxy_auth_jwt_manager,
        gcp_oidc_token_creation_host,
        gcp_oidc_token_creation_path,
        oidc_token_lifetime,
    ):
        self.spacetime_auth_jwt_manager = spacetime_auth_jwt_manager
        self.proxy_auth_jwt_manager = proxy_auth_jwt_manager
        self.gcp_oidc_token_creation_host = gcp_oidc_token_creation_host
        self.gcp_oidc_token_creation_path = gcp_oidc_token_creation_path
        self.oidc_token_lifetime = oidc_token_lifetime

        self.oidc_token = ""
        self.oidc_token_expiration_time = datetime.now(timezone.utc)

    @classmethod
    def create_from_private_key(cls, host: str, agent_email: str,
                                private_key_id: str, private_key: str):
        spacetime_auth_jwt_manager = JwtManager(lifetime=timedelta(hours=1),
                                                issuer=agent_email,
                                                subject=agent_email,
                                                audience=host,
                                                private_key_id=private_key_id,
                                                private_key_pem=private_key)
        proxy_auth_jwt_manager = JwtManager(
            lifetime=timedelta(hours=1),
            issuer=agent_email,
            subject=agent_email,
            audience="https://{}{}".format(
                cls.DEFAULT_GCP_OIDC_TOKEN_CREATION_HOST,
                cls.DEFAULT_GCP_OIDC_TOKEN_CREATION_PATH),
            target_audience=cls.PROXY_TARGET_AUDIENCE,
            private_key_id=private_key_id,
            private_key_pem=private_key)
        return cls(spacetime_auth_jwt_manager, proxy_auth_jwt_manager,
                   cls.DEFAULT_GCP_OIDC_TOKEN_CREATION_HOST,
                   cls.DEFAULT_GCP_OIDC_TOKEN_CREATION_PATH,
                   cls.DEFAULT_OIDC_TOKEN_LIFETIME)

    @classmethod
    def create_from_jwt(cls, spacetime_auth_jwt: str, proxy_auth_jwt: str):
        spacetime_auth_jwt_manager = JwtManager(lifetime=timedelta.max,
                                                jwt_value=spacetime_auth_jwt)
        proxy_auth_jwt_manager = JwtManager(lifetime=timedelta.max,
                                            jwt_value=proxy_auth_jwt)
        return cls(spacetime_auth_jwt_manager, proxy_auth_jwt_manager,
                   cls.DEFAULT_GCP_OIDC_TOKEN_CREATION_HOST,
                   cls.DEFAULT_GCP_OIDC_TOKEN_CREATION_PATH,
                   cls.DEFAULT_OIDC_TOKEN_LIFETIME)

    # Supplies the authorization credentials in gRPC calls.
    def __call__(self, context, callback):
        now = datetime.now(timezone.utc)
        if self.is_oidc_token_expired(now):
            proxy_auth_jwt = self.proxy_auth_jwt_manager.generate_jwt()
            self.oidc_token_expiration_time = now + self.oidc_token_lifetime
            self.oidc_token = self._exchange_proxy_auth_jwt_for_oidc_token(
                proxy_auth_jwt)

        callback([
            ("authorization", "Bearer {}".format(
                self.spacetime_auth_jwt_manager.generate_jwt())),
            ("proxy-authorization", "Bearer {}".format(self.oidc_token)),
        ], None)

    def _exchange_proxy_auth_jwt_for_oidc_token(self, proxy_auth_jwt_value):
        data = {
            "grant_type": "urn:ietf:params:oauth:grant-type:jwt-bearer",
            "assertion": proxy_auth_jwt_value
        }
        headers = {"Content-Type": "application/x-www-form-urlencoded"}
        connection = http.client.HTTPSConnection(
            self.gcp_oidc_token_creation_host)
        connection.request("POST",
                           self.gcp_oidc_token_creation_path,
                           body=urlencode(data),
                           headers=headers)
        response = connection.getresponse()
        response_data = response.read()
        response_json = json.loads(response_data.decode("utf-8"))
        return response_json["id_token"]

    def is_oidc_token_expired(self, now):
        return (not self.oidc_token or now +
                SpacetimeCallCredentials.OIDC_TOKEN_EXPIRATION_TIME_MARGIN
                > self.oidc_token_expiration_time)

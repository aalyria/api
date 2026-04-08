# Copyright (c) Aalyria Technologies, Inc., and its affiliates.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""Authentication module for Spacetime API JWT credentials.

This module implements JWT-based authentication for Spacetime APIs using self-signed
JWTs that work with Google Cloud IAP (Identity-Aware Proxy). The JWT audience must
match the full gRPC method URI.
"""

import threading
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from typing import Any, BinaryIO, Dict, Optional
from urllib.parse import urlparse

import grpc
import jwt
from cryptography.hazmat.primitives.serialization import load_pem_private_key

# Constants
AUTHORIZATION_HEADER = "authorization"
TOKEN_LIFETIME = timedelta(hours=1)
TOKEN_EXPIRATION_WINDOW = timedelta(minutes=5)


class ExpiringToken:
    """Represents a token with an expiration time."""

    def __init__(self, token: str, expires_at: datetime):
        self.token = token
        self.expires_at = expires_at

    def is_stale(self, clock: Optional['Clock'] = None) -> bool:
        """Check if the token is stale (needs refresh)."""
        now = clock.now() if clock else datetime.now(timezone.utc)
        return now > (self.expires_at - TOKEN_EXPIRATION_WINDOW)


class Clock:
    """Clock interface for getting current time (allows mocking in tests)."""

    def now(self) -> datetime:
        """Return current time in UTC."""
        return datetime.now(timezone.utc)


@dataclass
class Config:
    """Configuration for creating authentication credentials.

    Attributes:
        clock: Clock instance for time operations (defaults to real clock)
        private_key: File-like object containing PEM-encoded private key
        private_key_id: Key ID to include in JWT header
        email: Email address for issuer and subject claims
        skip_transport_security: Disable transport security checks (testing only)
    """
    clock: Optional[Clock] = None
    private_key: Optional[BinaryIO] = None
    private_key_id: str = ""
    email: str = ""
    skip_transport_security: bool = False


@dataclass
class JWTOptions:
    """Options for JWT creation.

    Attributes:
        email: Email address for issuer and subject
        private_key_id: Key ID for JWT header
        audience: Audience claim (typically the API endpoint)
        expires_at: Expiration time
        issued_at: Issuance time
    """
    email: str
    private_key_id: str
    audience: str
    expires_at: datetime
    issued_at: datetime


class AuthCredentials(grpc.AuthMetadataPlugin):
    """Implementation of grpc.AuthMetadataPlugin for JWT authentication.

    Uses self-signed JWTs with audience set to the full gRPC method URI.
    This works with Google Cloud IAP's self-signed JWT support (added Aug 2025).
    """

    def __init__(self, config: Config, private_key: Any):
        self._lock = threading.RLock()
        self._tokens: Dict[str, ExpiringToken] = {}
        self._config = config
        self._private_key = private_key

    def _get_token(self, audience: str) -> str:
        """Get cached token or create new one for the given audience."""
        # Check if we have a valid cached token
        with self._lock:
            if audience in self._tokens:
                token = self._tokens[audience]
                if not token.is_stale(self._config.clock):
                    return token.token

            # Generate new token
            now = self._config.clock.now() if self._config.clock else datetime.now(
                timezone.utc)
            expires_at = now + TOKEN_LIFETIME

            opts = JWTOptions(
                email=self._config.email,
                private_key_id=self._config.private_key_id,
                audience=audience,
                expires_at=expires_at,
                issued_at=now,
            )

            new_token = create_jwt(opts, self._private_key)
            self._tokens[audience] = ExpiringToken(new_token, expires_at)
            return new_token

    def __call__(
        self,
        context: grpc.AuthMetadataContext,
        callback: grpc.AuthMetadataPluginCallback,
    ) -> None:
        """Called by gRPC to get authentication metadata for a request.

        Constructs a JWT with the audience set to the full gRPC method URI
        (https://host/package.Service/Method), which is required for IAP's
        self-signed JWT support.

        Args:
            context: gRPC authentication context containing method name and URL
            callback: Callback to invoke with metadata or error
        """
        try:
            # Parse the service URL to extract host and path
            # service_url format: https://host/package.Service
            # method_name format: MethodName (not the full path!)
            parsed_uri = urlparse(context.service_url)

            # Remove default port :443 if present
            host = parsed_uri.netloc
            if host.endswith(':443'):
                host = host[:-4]

            # Compose the audience: https://host/package.Service/Method
            # The service_url contains the package.Service path, and we append the method
            service_path = parsed_uri.path  # e.g., /aalyria.spacetime.api.model.v1.Model
            audience = f"https://{host}{service_path}/{context.method_name}"

            # Get or create token for this specific audience
            token = self._get_token(audience)

            # Return metadata with Bearer token (single authorization header, no proxy header needed)
            metadata = ((AUTHORIZATION_HEADER, f"Bearer {token}"),)
            callback(metadata, None)

        except Exception as e:
            callback(None, e)


def new_credentials(config: Config) -> grpc.CallCredentials:
    """Create gRPC call credentials for authenticating with Spacetime services.

    Args:
        config: Configuration containing email, private key, etc.

    Returns:
        grpc.CallCredentials instance that can be used with channel credentials

    Raises:
        ValueError: If required config fields are missing
        Exception: If private key cannot be parsed
    """
    # Validate required fields
    errors = []
    if config.clock is None:
        config.clock = Clock()
    if not config.email:
        errors.append("missing required field 'email'")
    if not config.private_key_id:
        errors.append("missing required field 'private_key_id'")
    if config.private_key is None:
        errors.append("missing required field 'private_key'")

    if errors:
        raise ValueError("; ".join(errors))

    # Read private key
    pkey_bytes = config.private_key.read()
    if not pkey_bytes:
        raise ValueError("empty private key")

    # Parse private key
    private_key = parse_private_key(pkey_bytes)

    # Create auth credentials plugin
    auth_plugin = AuthCredentials(config, private_key)

    # Return gRPC call credentials
    return grpc.metadata_call_credentials(auth_plugin)


def create_jwt(opts: JWTOptions, private_key: Any) -> str:
    """Create a JWT token with the specified options and private key.

    Args:
        opts: JWT options including email, audience, expiration, etc.
        private_key: Private key for signing (RSA or other supported type)

    Returns:
        Signed JWT token string

    Raises:
        Exception: If token signing fails
    """
    # Build claims
    claims = {
        "iss": opts.email,
        "sub": opts.email,
        "iat": int(opts.issued_at.timestamp()),
        "exp": int(opts.expires_at.timestamp()),
    }

    if opts.audience:
        claims["aud"] = opts.audience

    # Build headers
    headers = {}
    if opts.private_key_id:
        headers["kid"] = opts.private_key_id

    # Create and sign token
    token = jwt.encode(
        claims,
        private_key,
        algorithm="RS256",
        headers=headers,
    )

    return token


def parse_private_key(pem_data: bytes) -> Any:
    """Parse a PEM-encoded private key.

    Supports PKCS#1 and PKCS#8 formats.

    Args:
        pem_data: PEM-encoded private key bytes

    Returns:
        Private key object

    Raises:
        ValueError: If key cannot be parsed
    """
    try:
        private_key = load_pem_private_key(pem_data, password=None)
        return private_key
    except Exception as e:
        raise ValueError(f"failed to parse private key: {e}") from e

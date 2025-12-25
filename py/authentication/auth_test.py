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

"""Unit tests for auth module."""

import io
import unittest
from datetime import datetime, timezone
from unittest.mock import Mock

import grpc
import jwt
from cryptography.hazmat.primitives import serialization
from cryptography.hazmat.primitives.asymmetric import rsa

from py.authentication import auth


class FakeClock(auth.Clock):
    """Fake clock for testing."""

    def __init__(self, fixed_time: datetime):
        self._time = fixed_time

    def now(self) -> datetime:
        return self._time

    def advance(self, seconds: int):
        """Advance the clock by the given number of seconds."""
        from datetime import timedelta
        self._time += timedelta(seconds=seconds)


def generate_rsa_private_key() -> bytes:
    """Generate a test RSA private key in PEM format."""
    # Generate RSA key
    private_key = rsa.generate_private_key(
        public_exponent=65537,
        key_size=2048,
    )

    # Encode to PEM
    pem = private_key.private_bytes(
        encoding=serialization.Encoding.PEM,
        format=serialization.PrivateFormat.PKCS8,
        encryption_algorithm=serialization.NoEncryption()
    )

    return pem


class TestNewCredentialsValidation(unittest.TestCase):
    """Tests for new_credentials validation."""

    def setUp(self):
        self.test_key = generate_rsa_private_key()

    def test_missing_email(self):
        """Test that missing email raises ValueError."""
        config = auth.Config(
            email="",
            private_key=io.BytesIO(self.test_key),
            private_key_id="1",
            clock=auth.Clock(),
        )

        with self.assertRaises(ValueError) as cm:
            auth.new_credentials(config)

        self.assertIn("missing required field 'email'", str(cm.exception))

    def test_missing_private_key_id(self):
        """Test that missing private key ID raises ValueError."""
        config = auth.Config(
            email="test@example.com",
            private_key=io.BytesIO(self.test_key),
            private_key_id="",
            clock=auth.Clock(),
        )

        with self.assertRaises(ValueError) as cm:
            auth.new_credentials(config)

        self.assertIn("missing required field 'private_key_id'", str(cm.exception))

    def test_missing_private_key(self):
        """Test that missing private key raises ValueError."""
        config = auth.Config(
            email="test@example.com",
            private_key=None,
            private_key_id="1",
            clock=auth.Clock(),
        )

        with self.assertRaises(ValueError) as cm:
            auth.new_credentials(config)

        self.assertIn("missing required field 'private_key'", str(cm.exception))

    def test_empty_private_key(self):
        """Test that empty private key raises ValueError."""
        config = auth.Config(
            email="test@example.com",
            private_key=io.BytesIO(b""),
            private_key_id="1",
            clock=auth.Clock(),
        )

        with self.assertRaises(ValueError) as cm:
            auth.new_credentials(config)

        self.assertIn("empty private key", str(cm.exception))

    def test_default_clock_when_none(self):
        """Test that a default clock is created when none is provided."""
        config = auth.Config(
            email="test@example.com",
            private_key=io.BytesIO(self.test_key),
            private_key_id="1",
            clock=None,  # No clock provided
        )

        # Should not raise an error
        creds = auth.new_credentials(config)
        self.assertIsNotNone(creds)


class TestJWTGeneration(unittest.TestCase):
    """Tests for JWT generation and validation."""

    def setUp(self):
        self.test_key_pem = generate_rsa_private_key()
        self.test_key = auth.parse_private_key(self.test_key_pem)
        self.fixed_time = datetime(2025, 1, 1, 12, 0, 0, tzinfo=timezone.utc)
        self.clock = FakeClock(self.fixed_time)

    def test_jwt_structure_and_claims(self):
        """Test that generated JWT has correct structure and claims."""
        config = auth.Config(
            email="test@example.com",
            private_key=io.BytesIO(self.test_key_pem),
            private_key_id="test-key-id",
            clock=self.clock,
            skip_transport_security=True,
        )

        # Create AuthCredentials directly for testing
        private_key = auth.parse_private_key(self.test_key_pem)
        auth_plugin = auth.AuthCredentials(config, private_key)

        # Create mock context
        mock_context = Mock(spec=grpc.AuthMetadataContext)
        mock_context.service_url = "https://api.example.com:8080/service.TestService"
        mock_context.method_name = "TestMethod"

        # Capture the callback result
        callback_metadata = None
        callback_error = None

        def callback(metadata, error):
            nonlocal callback_metadata, callback_error
            callback_metadata = metadata
            callback_error = error

        # Call the plugin directly
        auth_plugin(mock_context, callback)

        # Verify no error
        self.assertIsNone(callback_error)
        self.assertIsNotNone(callback_metadata)

        # Extract token from metadata
        metadata_dict = dict(callback_metadata)
        auth_header = metadata_dict.get("authorization")
        self.assertIsNotNone(auth_header)
        self.assertTrue(auth_header.startswith("Bearer "))

        token_string = auth_header[7:]  # Remove "Bearer " prefix

        # Parse JWT without verification to examine structure
        decoded = jwt.decode(token_string, options={"verify_signature": False})

        # Validate claims
        self.assertEqual(decoded["iss"], "test@example.com")
        self.assertEqual(decoded["sub"], "test@example.com")
        self.assertEqual(decoded["aud"], "https://api.example.com:8080/service.TestService/TestMethod")
        self.assertEqual(decoded["iat"], int(self.fixed_time.timestamp()))
        self.assertEqual(decoded["exp"], int(self.fixed_time.timestamp()) + 3600)  # 1 hour later

        # Validate header
        header = jwt.get_unverified_header(token_string)
        self.assertEqual(header["alg"], "RS256")
        self.assertEqual(header["kid"], "test-key-id")

    def test_jwt_signature_validation(self):
        """Test that JWT signature can be validated with the public key."""
        config = auth.Config(
            email="test@example.com",
            private_key=io.BytesIO(self.test_key_pem),
            private_key_id="test-key-id",
            clock=self.clock,
            skip_transport_security=True,
        )

        # Create AuthCredentials directly for testing
        private_key = auth.parse_private_key(self.test_key_pem)
        auth_plugin = auth.AuthCredentials(config, private_key)

        # Create mock context
        mock_context = Mock(spec=grpc.AuthMetadataContext)
        mock_context.service_url = "https://api.example.com/service.TestService"
        mock_context.method_name = "TestMethod"

        callback_metadata = None

        def callback(metadata, error):
            nonlocal callback_metadata
            callback_metadata = metadata

        auth_plugin(mock_context, callback)

        # Extract token
        metadata_dict = dict(callback_metadata)
        token_string = metadata_dict["authorization"][7:]

        # Get public key from private key
        public_key = self.test_key.public_key()

        # Verify signature
        decoded = jwt.decode(
            token_string,
            public_key,
            algorithms=["RS256"],
            options={
                "verify_exp": False,
                "verify_aud": False
            }
        )

        self.assertEqual(decoded["iss"], "test@example.com")

    def test_audience_construction_with_default_port(self):
        """Test that default port :443 is removed from audience."""
        config = auth.Config(
            email="test@example.com",
            private_key=io.BytesIO(self.test_key_pem),
            private_key_id="test-key-id",
            clock=self.clock,
            skip_transport_security=True,
        )

        # Create AuthCredentials directly for testing
        private_key = auth.parse_private_key(self.test_key_pem)
        auth_plugin = auth.AuthCredentials(config, private_key)

        # Create mock context with :443 port
        mock_context = Mock(spec=grpc.AuthMetadataContext)
        mock_context.service_url = "https://api.example.com:443/service.TestService"
        mock_context.method_name = "TestMethod"

        callback_metadata = None

        def callback(metadata, error):
            nonlocal callback_metadata
            callback_metadata = metadata

        auth_plugin(mock_context, callback)

        # Extract and decode token
        metadata_dict = dict(callback_metadata)
        token_string = metadata_dict["authorization"][7:]
        decoded = jwt.decode(token_string, options={"verify_signature": False})

        # Verify
        self.assertEqual(decoded["aud"], "https://api.example.com/service.TestService/TestMethod")

    def test_token_caching(self):
        """Test that tokens are cached and reused for the same audience."""
        config = auth.Config(
            email="test@example.com",
            private_key=io.BytesIO(self.test_key_pem),
            private_key_id="test-key-id",
            clock=self.clock,
            skip_transport_security=True,
        )

        # Create AuthCredentials directly for testing
        private_key = auth.parse_private_key(self.test_key_pem)
        auth_plugin = auth.AuthCredentials(config, private_key)

        # Create mock context
        mock_context = Mock(spec=grpc.AuthMetadataContext)
        mock_context.service_url = "https://api.example.com/service.TestService"
        mock_context.method_name = "TestMethod"

        # Get first token
        first_token = None

        def callback1(metadata, error):
            nonlocal first_token
            first_token = dict(metadata)["authorization"]

        auth_plugin(mock_context, callback1)

        # Get second token for same audience
        second_token = None

        def callback2(metadata, error):
            nonlocal second_token
            second_token = dict(metadata)["authorization"]

        auth_plugin(mock_context, callback2)

        # Tokens should be identical (cached)
        self.assertEqual(first_token, second_token)

    def test_token_refresh_when_stale(self):
        """Test that stale tokens are refreshed."""
        config = auth.Config(
            email="test@example.com",
            private_key=io.BytesIO(self.test_key_pem),
            private_key_id="test-key-id",
            clock=self.clock,
            skip_transport_security=True,
        )

        # Create AuthCredentials directly for testing
        private_key = auth.parse_private_key(self.test_key_pem)
        auth_plugin = auth.AuthCredentials(config, private_key)

        # Create mock context
        mock_context = Mock(spec=grpc.AuthMetadataContext)
        mock_context.service_url = "https://api.example.com/service.TestService"
        mock_context.method_name = "TestMethod"

        # Get first token
        first_token = None

        def callback1(metadata, error):
            nonlocal first_token
            first_token = dict(metadata)["authorization"][7:]

        auth_plugin(mock_context, callback1)
        first_decoded = jwt.decode(first_token, options={"verify_signature": False})

        # Advance clock by 56 minutes (within expiration window of 1 hour - 5 min margin)
        self.clock.advance(56 * 60)

        # Get second token
        second_token = None

        def callback2(metadata, error):
            nonlocal second_token
            second_token = dict(metadata)["authorization"][7:]

        auth_plugin(mock_context, callback2)
        second_decoded = jwt.decode(second_token, options={"verify_signature": False})

        # Tokens should be different (refreshed)
        self.assertNotEqual(first_token, second_token)
        # Second token should have later timestamps
        self.assertGreater(second_decoded["iat"], first_decoded["iat"])


class TestParsePrivateKey(unittest.TestCase):
    """Tests for parse_private_key function."""

    def test_valid_pkcs8_key(self):
        """Test parsing a valid PKCS8 private key."""
        key_pem = generate_rsa_private_key()
        private_key = auth.parse_private_key(key_pem)
        self.assertIsNotNone(private_key)

    def test_invalid_key_data(self):
        """Test that invalid key data raises ValueError."""
        with self.assertRaises(ValueError) as cm:
            auth.parse_private_key(b"not a valid key")

        self.assertIn("failed to parse private key", str(cm.exception))


if __name__ == "__main__":
    unittest.main()

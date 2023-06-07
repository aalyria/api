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
import os
import sys
from pathlib import Path

from api.nbi.v1alpha.nbi_pb2 import CreateEntityRequest, Entity
from py.authentication.spacetime_call_credentials import SpacetimeCallCredentials
from google.protobuf import json_format
import api.nbi.v1alpha.nbi_pb2_grpc as NetOpsGrpc


# This sample constructs a PlatformDefinition entity for a simple User Terminal (UT)
# from a JSON file, and sends this entity to Spacetime through the Northbound interface (NBI).
def main():
    if len(sys.argv) < 5:
        print("Error parsing arguments. Provide a value for host, " +
              "agent email, agent private key ID, and agent private key file.",
              file=sys.stderr)
        sys.exit(-1)

    HOST = sys.argv[1]
    AGENT_EMAIL = sys.argv[2]
    AGENT_PRIV_KEY_ID = sys.argv[3]
    AGENT_PRIV_KEY_FILE = sys.argv[4]
    PORT = 443
    # Reads from the provided JSON file if it exists, otherwise uses the example in the resources/ directory.
    ENTITY_JSON_FILE = sys.argv[5] if len(sys.argv) > 5 else os.path.join(
        os.path.dirname(__file__),
        "resources/user_terminal_platform_definition.json")

    # The private key should start with "-----BEGIN RSA PRIVATE KEY-----" and
    # end with "-----END RSA PRIVATE KEY-----". In between, there should be newline-delimited
    # strings of characters.
    private_key = Path(AGENT_PRIV_KEY_FILE).read_text()

    # Sets up the channel using the two signed JWTs for RPCs to the NBI.
    credentials = grpc.metadata_call_credentials(
        SpacetimeCallCredentials.create_from_private_key(
            HOST, AGENT_EMAIL, AGENT_PRIV_KEY_ID, private_key))
    channel = grpc.secure_channel(
        f"{HOST}:{PORT}",
        grpc.composite_channel_credentials(grpc.ssl_channel_credentials(),
                                           credentials))

    # Sets up a stub to invoke RPCs against the NBI's NetOps service.
    # This stub can now be used to call any method in the NetOps service.
    stub = NetOpsGrpc.NetOpsStub(channel)

    # Sends a request to create the entity to the NBI using the platform
    # definition entity in the JSON file.
    # This file assumes that a BandProfile entity, with an ID of "band-profile-id",
    # and an AntennaPattern entity, with an ID of "antenna-pattern-id", were already created.
    # The IDs of these entities are used to construct the transceiver.
    request = CreateEntityRequest(type="PLATFORM_DEFINITION",
                                  entity=json_format.Parse(
                                      Path(ENTITY_JSON_FILE).read_text(),
                                      Entity()))
    platform_definition_entity = stub.CreateEntity(request)
    print("Entity created:\n", platform_definition_entity)


if __name__ == "__main__":
    main()

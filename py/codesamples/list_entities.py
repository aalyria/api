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
import sys
from pathlib import Path

import api.nbi.v1alpha.nbi_pb2 as Nbi
import api.nbi.v1alpha.nbi_pb2_grpc as NetOpsGrpc
from py.authentication.spacetime_call_credentials import SpacetimeCallCredentials


def main():
    if len(sys.argv) < 4:
        print("Error parsing arguments. Provide a value for host, " + 
                "agent email, agent private key ID, and agent private key file.",
              file=sys.stderr)
        sys.exit(-1)

    HOST = sys.argv[1]
    AGENT_EMAIL = sys.argv[2]
    AGENT_PRIV_KEY_ID = sys.argv[3]
    AGENT_PRIV_KEY_FILE = sys.argv[4]
    PORT = 443

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
    stub = NetOpsGrpc.NetOpsStub(channel)

    # This stub can now be used to call any method in the NetOps service.
    request = Nbi.ListEntitiesRequest(type="PLATFORM_DEFINITION")
    entities = stub.ListEntities(request)
    print("ListEntitiesResponse received:\n", entities)


if __name__ == "__main__":
    main()

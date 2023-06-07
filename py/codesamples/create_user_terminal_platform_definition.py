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

from api.common.coordinates_pb2 import Motion, GeodeticWgs84
from api.common.platform_antenna_pb2 import AntennaDefinition, Targeting, Projection
from api.common.platform_pb2 import PlatformDefinition
from api.common.wireless_pb2 import AmplifierDefinition
from api.common.wireless_receiver_pb2 import ReceiverDefinition, RxChannels, ReceiveSignalProcessor
from api.common.wireless_transceiver_pb2 import TransceiverModel
from api.common.wireless_transmitter_pb2 import TransmitterDefinition, TxChannels, TransmitSignalProcessor
from api.nbi.v1alpha.nbi_pb2 import CreateEntityRequest, Entity
from py.authentication.spacetime_call_credentials import SpacetimeCallCredentials
import api.nbi.v1alpha.nbi_pb2_grpc as NetOpsGrpc


# This sample constructs a PlatformDefinition entity for a simple User Terminal (UT),
# and sends this entity to Spacetime through the Northbound interface (NBI).
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

    # This sample assumes that a BandProfile and AntennaPattern entity were already created.
    # The IDs of these entities are used to construct the transceiver.
    band_profile_id = "band-profile-id"
    antenna_pattern_id = "antenna-pattern-id"

    # The UT has a fixed location in Livermore, CA.
    wgs84_coordinates = Motion(
        geodetic_wgs84=GeodeticWgs84(longitude_deg=-121.7, latitude_deg=37.7))
    # The UT has a single transceiver.
    transceiver_model = TransceiverModel(
        id="transceiver-model",
        transmitter=TransmitterDefinition(
            name="tx",
            # The transmitter operates at 11 GHz.
            channel_set={
                band_profile_id:
                TxChannels(
                    channel={
                        11_000_000_000:
                        TxChannels.TxChannelParams(max_power_watts=100)
                    })
            },
            # The transmit signal chain has a constant gain amplifier.
            signal_processing_step=[
                TransmitSignalProcessor(amplifier=AmplifierDefinition(
                    constant_gain=AmplifierDefinition.
                    ConstantGainAmplifierDefinition(
                        gain_db=10.0,
                        noise_factor=1,
                        reference_temperature_k=290.0)))
            ]),
        receiver=ReceiverDefinition(
            name="rx",
            # The transmitter operates at 12 GHz.
            channel_set={
                band_profile_id:
                RxChannels(center_frequency_hz=[12_000_000_000])
            },
            # The receive signal chain has a constant gain amplifier.
            signal_processing_step=[
                ReceiveSignalProcessor(amplifier=AmplifierDefinition(
                    constant_gain=AmplifierDefinition.
                    ConstantGainAmplifierDefinition(
                        gain_db=10.0,
                        noise_factor=1,
                        reference_temperature_k=290.0)))
            ]),
        # The UT has a steerable antenna whose field of regard has an outer half
        # angle of 50 degrees.
        antenna=AntennaDefinition(
            name="antenna",
            antenna_pattern_id=antenna_pattern_id,
            targeting=Targeting(),
            field_of_regard=Projection(conic=Projection.Conic(
                outer_half_angle_deg=50))))

    # Sends a request to create the entity to the NBI.
    request = CreateEntityRequest(
        type="PLATFORM_DEFINITION",
        entity=Entity(id="my-globally-unique-platform-id",
                      platform=PlatformDefinition(
                          name="user-terminal",
                          coordinates=wgs84_coordinates,
                          transceiver_model=[transceiver_model],
                      )))
    platform_definition_entity = stub.CreateEntity(request)
    print("Entity created:\n", platform_definition_entity)


if __name__ == "__main__":
    main()

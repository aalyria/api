'''
Copyright (c) Aalyria Technologies, Inc., and its affiliates.

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

import argparse
import sys
from pathlib import Path

import api.model.v1.model_pb2 as model_pb2
import api.model.v1.model_pb2_grpc as model_pb2_grpc
import grpc
import nmts.v1.proto.nmts_pb2 as nmts_pb2

from py.authentication.spacetime_call_credentials import \
    SpacetimeCallCredentials


def list_entities(stub: model_pb2_grpc.ModelStub) -> list[nmts_pb2.Entity]:
  request = model_pb2.ListEntitiesRequest()
  response = stub.ListEntities(request)
  return [entity for entity in response.entities]


def establish_connection(target: str, email: str, key_id: str,
                         private_key: str) -> model_pb2_grpc.ModelStub:
  # Sets up the channel using the two signed JWTs for RPCs to the Model API.
  credentials = grpc.metadata_call_credentials(
      SpacetimeCallCredentials.create_from_private_key(target, email, key_id,
                                                       private_key))
  channel = grpc.secure_channel(
      target,
      grpc.composite_channel_credentials(grpc.ssl_channel_credentials(),
                                         credentials),
      [
          (
              "grpc.max_receive_message_length",
              1024 * 1024 * 256,
          ),
      ])
  return model_pb2_grpc.ModelStub(channel)


def main():
  # Setup argparser
  parser = argparse.ArgumentParser()
  parser.add_argument(
      "target",
      type=str,
      help=
      "The target URL of the Spacetime Model API (e.g., 'api.example.com' or 'api.example.com:8080').",
  )
  parser.add_argument(
      "email",
      type=str,
      help="Client Email for Spacetime Auth.",
  )
  parser.add_argument(
      "key_id",
      type=str,
      help="Client Key ID for Spacetime Auth.",
  )
  parser.add_argument(
      "private_key_path",
      type=str,
      help="The Client Key File Path for Spacetime Auth.",
  )
  args = parser.parse_args()

  # The private key should start with "-----BEGIN RSA PRIVATE KEY-----" and
  # end with "-----END RSA PRIVATE KEY-----". In between, there should be newline-delimited
  # strings of characters.
  private_key = Path(args.private_key_path).read_text()

  stub = establish_connection(args.target, args.email, args.key_id, private_key)
  entities = list_entities(stub)
  print("ListEntitiesResponse received:\n", entities)


if __name__ == "__main__":
  main()

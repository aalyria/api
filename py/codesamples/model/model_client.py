"""
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
"""

import argparse
import sys
from io import BytesIO
from pathlib import Path

import api.model.v1.model_pb2 as model_pb2
import api.model.v1.model_pb2_grpc as model_pb2_grpc
import grpc
import nmts.v1.proto.nmts_pb2 as nmts_pb2

from py.authentication import auth


def list_entities(stub: model_pb2_grpc.ModelStub, page_size: int = 0) -> list[nmts_pb2.Entity]:
  """Reads every entity in the model, one page at a time.

  ListEntities answers with a page of entities and, when more remain, a token
  naming where the next page begins. A caller that wants the whole model has to
  follow those tokens until the response carries none, which is what the loop
  below does. Passing a page_size of 0 lets the server choose the page size.
  """
  entities = []
  request = model_pb2.ListEntitiesRequest(page_size=page_size)
  while True:
    response = stub.ListEntities(request)
    entities.extend(response.entities)
    if not response.next_page_token:
      return entities
    # A cursor that fails to advance would walk the same page forever. Returning
    # the entities read so far would look like the whole model to a caller that
    # acts on it, so this fails instead.
    if response.next_page_token == request.page_token:
      raise RuntimeError(
        f"ListEntities repeated the page token it was given ({len(entities)} entities read); the walk cannot advance"
      )
    request.page_token = response.next_page_token


def establish_connection(target: str, email: str, key_id: str, private_key: str) -> model_pb2_grpc.ModelStub:
  # Create auth config
  config = auth.Config(email=email, private_key_id=key_id, private_key=BytesIO(private_key.encode("utf-8")))

  # Create call credentials
  call_credentials = auth.new_credentials(config)

  # Combine with SSL channel credentials
  channel = grpc.secure_channel(
    target,
    grpc.composite_channel_credentials(grpc.ssl_channel_credentials(), call_credentials),
    [
      (
        "grpc.max_receive_message_length",
        1024 * 1024 * 256,
      ),
    ],
  )
  return model_pb2_grpc.ModelStub(channel)


def main():
  # Setup argparser
  parser = argparse.ArgumentParser()
  parser.add_argument(
    "target",
    type=str,
    help="The target URL of the Spacetime Model API (e.g., 'api.example.com' or 'api.example.com:8080').",
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
  parser.add_argument(
    "--page_size",
    type=int,
    default=0,
    help="The number of entities to request per page. 0 lets the server choose.",
  )
  args = parser.parse_args()

  # The private key should start with "-----BEGIN RSA PRIVATE KEY-----" and
  # end with "-----END RSA PRIVATE KEY-----". In between, there should be newline-delimited
  # strings of characters.
  private_key = Path(args.private_key_path).read_text()

  stub = establish_connection(args.target, args.email, args.key_id, private_key)
  entities = list_entities(stub, args.page_size)
  print("ListEntitiesResponse received:\n", entities)


if __name__ == "__main__":
  main()

// Copyright (c) Aalyria Technologies, Inc., and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package com.aalyria.spacetime.codesamples.model.client;

import static io.grpc.Grpc.newChannelBuilder;

import com.aalyria.spacetime.api.model.v1.ModelGrpc;
import com.aalyria.spacetime.api.model.v1.ModelOuterClass.ListEntitiesRequest;
import com.aalyria.spacetime.api.model.v1.ModelOuterClass.ListEntitiesResponse;
import com.aalyria.spacetime.authentication.SpacetimeCallCredentials;
import io.grpc.CompositeChannelCredentials;
import io.grpc.ManagedChannel;
import io.grpc.TlsChannelCredentials;
import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Paths;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.TimeUnit;
import org.outernetcouncil.nmts.v1.proto.Nmts.Entity;

/** A sample class that demonstrates how to interact with Spacetime's Model API. */
public class ModelClient {
  private static final int MAX_MESSAGE_SIZE = 1024 * 1024 * 256;

  /**
   * Reads every entity in the model, one page at a time.
   *
   * <p>ListEntities answers with a page of entities and, when more remain, a token naming where the
   * next page begins. A caller that wants the whole model has to follow those tokens until the
   * response carries none, which is what the loop below does.
   *
   * @param stub the gRPC stub for the Model service
   * @param pageSize the number of entities to request per page; 0 lets the server choose
   * @return every entity in the model
   */
  public static List<Entity> listEntities(ModelGrpc.ModelBlockingStub stub, int pageSize) {
    List<Entity> entities = new ArrayList<>();
    ListEntitiesRequest.Builder request = ListEntitiesRequest.newBuilder().setPageSize(pageSize);
    while (true) {
      ListEntitiesResponse response = stub.listEntities(request.build());
      entities.addAll(response.getEntitiesList());
      if (response.getNextPageToken().isEmpty()) {
        return entities;
      }
      // A cursor that fails to advance would walk the same page forever.
      // Returning the entities read so far would look like the whole model to a
      // caller that acts on it, so this fails instead.
      if (response.getNextPageToken().equals(request.getPageToken())) {
        throw new IllegalStateException(
            "ListEntities repeated the page token it was given ("
                + entities.size()
                + " entities read); the walk cannot advance");
      }
      request.setPageToken(response.getNextPageToken());
    }
  }

  /**
   * Establishes a connection to the Model API.
   *
   * @param target the target URL of the API (e.g., "api.example.com" or "api.example.com:8080")
   * @param email the client email for authentication
   * @param keyId the client key ID for authentication
   * @param privateKey the private key content
   * @return configured gRPC stub
   */
  public static ModelGrpc.ModelBlockingStub establishConnection(
      String target, String email, String keyId, String privateKey) {

    // Sets up the channel using the two signed JWTs for RPCs to the Model API
    ManagedChannel channel =
        newChannelBuilder(
                target,
                CompositeChannelCredentials.create(
                    TlsChannelCredentials.create(),
                    SpacetimeCallCredentials.createFromPrivateKey(
                        target, email, keyId, privateKey)))
            .maxInboundMessageSize(MAX_MESSAGE_SIZE)
            .keepAliveTime(30, TimeUnit.SECONDS)
            .keepAliveTimeout(10, TimeUnit.SECONDS)
            .keepAliveWithoutCalls(true)
            .enableRetry()
            .build();

    // Create stub
    return ModelGrpc.newBlockingStub(channel);
  }

  public static void main(String[] args) {
    if (args.length < 4) {
      System.err.println(
          "Usage: ModelClient <target> <email> <key_id> <private_key_path> [page_size]");
      System.err.println(
          "  target: The target URL of the Spacetime Model API (e.g., 'api.example.com' or"
              + " 'api.example.com:8080')");
      System.err.println("  email: Client email for Spacetime authentication");
      System.err.println("  key_id: Client key ID for Spacetime authentication");
      System.err.println("  private_key_path: Path to the private key file");
      System.err.println(
          "  page_size: The number of entities to request per page. 0 lets the server choose");
      System.exit(1);
    }

    String target = args[0];
    String email = args[1];
    String keyId = args[2];
    String privateKeyPath = args[3];
    int pageSize = args.length > 4 ? Integer.parseInt(args[4]) : 0;

    try {
      // The private key should start with "-----BEGIN RSA PRIVATE KEY-----" and
      // end with "-----END RSA PRIVATE KEY-----". In between, there should be newline-delimited
      // strings of characters.
      String privateKey = Files.readString(Paths.get(privateKeyPath));

      // Establish connection
      ModelGrpc.ModelBlockingStub stub = establishConnection(target, email, keyId, privateKey);

      // List entities
      List<Entity> entities = listEntities(stub, pageSize);

      System.out.println("ListEntitiesResponse received:\n" + entities.toString());

    } catch (IOException e) {
      System.err.println("Error reading private key file: " + e.getMessage());
      System.exit(1);
    } catch (Exception e) {
      System.err.println("Error: " + e.getMessage());
      e.printStackTrace();
      System.exit(1);
    }
  }
}

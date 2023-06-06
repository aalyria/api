// Copyright 2023 Aalyria Technologies, Inc., and its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package com.aalyria.spacetime.codesamples.nbi.client;

import static io.grpc.Grpc.newChannelBuilderForAddress;

import com.aalyria.spacetime.api.nbi.v1alpha.Nbi.EntityType;
import com.aalyria.spacetime.api.nbi.v1alpha.Nbi.ListEntitiesRequest;
import com.aalyria.spacetime.api.nbi.v1alpha.Nbi.ListEntitiesResponse;
import com.aalyria.spacetime.api.nbi.v1alpha.NetOpsGrpc;
import com.aalyria.spacetime.authentication.SpacetimeCallCredentials;
import io.grpc.CompositeChannelCredentials;
import io.grpc.ManagedChannel;
import io.grpc.TlsChannelCredentials;
import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Paths;

/**
 * A sample class that sets up the authentication parameters to call Spacetime's Northbound
 * Interface (NBI).
 */
public class ListEntities {
  private static final int PORT = 443;

  public static void main(String[] args) {
    if (args.length < 4) {
      System.err.println(
          "Error parsing arguments. Provide a value for host, "
              + "agent email, agent private key ID, and agent private key file.");
      System.exit(-1);
    }
    final String HOST = args[0];
    final String AGENT_EMAIL = args[1];
    final String AGENT_PRIV_KEY_ID = args[2];
    final String AGENT_PRIV_KEY_FILE = args[3];

    // The private key should start with "-----BEGIN RSA PRIVATE KEY-----" and
    // end with "-----END RSA PRIVATE KEY-----". In between, there should be newline-delimtted
    // strings of characters.
    String privateKey;
    try {
      privateKey = new String(Files.readAllBytes(Paths.get(AGENT_PRIV_KEY_FILE)));
    } catch (IOException e) {
      throw new RuntimeException("Private key could not be read.", e);
    }

    // Sets up the channel using the two signed JWTs for RPCs to the NBI.
    ManagedChannel channel =
        newChannelBuilderForAddress(
                HOST,
                PORT,
                CompositeChannelCredentials.create(
                    TlsChannelCredentials.create(),
                    SpacetimeCallCredentials.createFromPrivateKey(
                        HOST, AGENT_EMAIL, AGENT_PRIV_KEY_ID, privateKey)))
            .enableRetry()
            .build();

    // Sets up a stub to invoke RPCs against the NBI's NetOps service.
    NetOpsGrpc.NetOpsBlockingStub stub = NetOpsGrpc.newBlockingStub(channel);

    // This stub can now be used to call any method in the NetOps service.
    ListEntitiesRequest request =
        ListEntitiesRequest.newBuilder().setType(EntityType.PLATFORM_DEFINITION.name()).build();
    ListEntitiesResponse entities = stub.listEntities(request);
    System.out.println("ListEntitiesResponse received: \n" + entities.toString());
  }
}

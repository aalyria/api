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

package com.aalyria.spacetime.nbi.sample_client;

import static io.grpc.Grpc.newChannelBuilderForAddress;

import com.aalyria.spacetime.api.nbi.v1alpha.Nbi.EntityType;
import com.aalyria.spacetime.api.nbi.v1alpha.Nbi.ListEntitiesRequest;
import com.aalyria.spacetime.api.nbi.v1alpha.Nbi.ListEntitiesResponse;
import com.aalyria.spacetime.api.nbi.v1alpha.NetOpsGrpc;

import io.grpc.CompositeChannelCredentials;
import io.grpc.ManagedChannel;
import io.grpc.TlsChannelCredentials;

/**
 * A sample class that sets up the authentication parameters to call Spacetime's
 * Northbound Interface (NBI).
 */
public class ListEntities {
  private static final int PORT = 443;

  public static void main(String[] args) {
    if (args.length < 3) {
        System.err.println("Error parsing arguments. Provide a value for HOST, SPACETIME_AUTH_JWT, PROXY_AUTH_JWT.");
        System.exit(-1);
    }
    final String HOST = args[0];
    // The two JWTs required to authenticate across the NBI.
    final String SPACETIME_AUTH_JWT = args[1];
    final String PROXY_AUTH_JWT = args[2];

    // Sets up the channel using the two signed JWTs for RPCs to the NBI.
    ManagedChannel channel = newChannelBuilderForAddress(
        HOST,
        PORT,
        CompositeChannelCredentials.create(
            TlsChannelCredentials.create(),
            SpacetimeCallCredentials.createFromJwt(SPACETIME_AUTH_JWT, PROXY_AUTH_JWT)))
        .enableRetry()
        .build();

    // Sets up a stub to invoke RPCs against the NBI's NetOps service.
    NetOpsGrpc.NetOpsBlockingStub stub = NetOpsGrpc.newBlockingStub(channel);

    // This stub can now be used to call any method in the NetOps service.
    ListEntitiesRequest request = ListEntitiesRequest.newBuilder()
        .setType(EntityType.PLATFORM_DEFINITION.name())
        .build();
    ListEntitiesResponse entities = stub.listEntities(request);
    System.out.println("ListEntitiesResponse received: \n" + entities.toString());
  }
}

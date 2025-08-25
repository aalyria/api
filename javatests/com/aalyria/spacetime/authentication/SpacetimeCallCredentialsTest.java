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

package com.aalyria.spacetime.authentication;

import static java.nio.charset.StandardCharsets.UTF_8;
import static org.junit.Assert.assertEquals;
import static org.junit.Assert.assertTrue;
import static org.mockito.AdditionalAnswers.delegatesTo;
import static org.mockito.Mockito.any;
import static org.mockito.Mockito.doAnswer;
import static org.mockito.Mockito.doReturn;
import static org.mockito.Mockito.mock;

import com.aalyria.spacetime.api.nbi.v1alpha.Nbi.ListEntitiesRequest;
import com.aalyria.spacetime.api.nbi.v1alpha.Nbi.ListEntitiesResponse;
import com.aalyria.spacetime.api.nbi.v1alpha.NetOpsGrpc;
import com.google.common.io.Resources;
import com.google.gson.Gson;
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpHandler;
import com.sun.net.httpserver.HttpServer;
import io.grpc.ManagedChannel;
import io.grpc.Metadata;
import io.grpc.Metadata.Key;
import io.grpc.ServerCall;
import io.grpc.ServerCallHandler;
import io.grpc.ServerInterceptor;
import io.grpc.inprocess.InProcessChannelBuilder;
import io.grpc.inprocess.InProcessServerBuilder;
import io.grpc.stub.StreamObserver;
import io.grpc.testing.GrpcCleanupRule;
import io.helidon.grpc.core.ResponseHelper;
import java.io.IOException;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.ZoneId;
import java.util.Map;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicInteger;
import org.junit.Rule;
import org.junit.Test;
import org.junit.rules.ExternalResource;

public class SpacetimeCallCredentialsTest {
  @Rule public final GrpcCleanupRule grpcCleanup = new GrpcCleanupRule();

  @Rule
  public final MockOidcTokenHttpServer oidcTokenServer = new MockOidcTokenHttpServer(/* port= */ 0);

  // Mocks the NBI server with a no-op implementation.
  NetOpsGrpc.NetOpsImplBase netOpsServiceImpl =
      mock(NetOpsGrpc.NetOpsImplBase.class, delegatesTo(new NetOpsGrpc.NetOpsImplBase() {}));

  private static final String HOST = "nbi.example.spacetime.aalyria.com";
  private static final String AGENT_EMAIL = "some@example.com";
  private static final String AGENT_PRIVATE_ID = "f1f2569a4ee44663732b8740e1f3f3e92c1931b5";
  private static final String VALID_TOKEN =
      "eyJhbGciOiJSUzI1NiIsImtpZCI6IjcyMTk0YjI2MzU0YzIzYzBiYTU5YTZkNzUxZGZmYWEyNTg2NTkwNGUiLCJ0eXAiOiJKV1QifQ.eyJhdWQiOiJodHRwczovL3d3dy5nb29nbGVhcGlzLmNvbS9vYXV0aDIvdjQvdG9rZW4iLCJleHAiOjE2ODE3OTIzMTksImlhdCI6MTY4MTc4ODcxOSwiaXNzIjoiY2RwaS1hZ2VudEBhNWEtc3BhY2V0aW1lLWdrZS1iYWNrLWRldi5pYW0uZ3NlcnZpY2VhY2NvdW50LmNvbSIsInN1YiI6ImNkcGktYWdlbnRAYTVhLXNwYWNldGltZS1na2UtYmFjay1kZXYuaWFtLmdzZXJ2aWNlYWNjb3VudC5jb20iLCJ0YXJnZXRfYXVkaWVuY2UiOiI2MDI5MjQwMzEzOS1tZTY4dGpnYWpsNWRjZGJwbmxtMmVrODMwbHZzbnNscS5hcHBzLmdvb2dsZXVzZXJjb250ZW50LmNvbSJ9.QyOi7vkFCwdmjT4ChT3_yVY4ZObUJkZkYC0q7alF_thiotdJKRiSo1ZHp_XnS0nM4WSWcQYLGHUDdAMPS0R22brFGzCl8ndgNjqI38yp_LDL8QVTqnLBGUj-m3xB5wH17Q_Dt8riBB4IE-mSS8FB-R6sqSwn-seMfMDydScC0FrtOF3-2BCYpIAlf1AQKN083QdtKgNEVDi72npPr2MmsWV3tct6ydXHWNbxG423kfSD6vCZSUTvWXAuVjuOwnbc2LHZS04U-jiLpvHxu06OwHOQ5LoGVPyd69o8Ny_Bapd2m0YCX2xJr8_HH2nw1jH7EplFf-owbBYz9ZtQoQ2YTA";
  private static final String PATH_TO_PRIVATE_KEY =
      "com/aalyria/spacetime/authentication/resources/test_private_key.pem";
  private static final String MOCK_NBI_SERVER_NAME = "mockNbiServer";
  // The number of threads to spawn in order to test the synchronization logic.
  private static final int NUM_THREADS = 100;

  public class MockOidcTokenHttpServer extends ExternalResource {
    private final HttpServer server;
    private final AtomicInteger numCalls = new AtomicInteger(0);
    private final int port;

    MockOidcTokenHttpServer(int port) {
      this.port = port;
      try {
        server = HttpServer.create();
      } catch (IOException e) {
        throw new RuntimeException(e);
      }
    }

    @Override
    protected void before() throws Throwable {
      server.bind(new InetSocketAddress(port), 0);
      server.createContext("/", new MockOidcTokenServiceHandler());
      server.start();
    }

    @Override
    protected void after() {
      server.stop(0);
    }

    public void resetNumCalls() {
      numCalls.set(0);
    }

    private class MockOidcTokenServiceHandler implements HttpHandler {
      @Override
      public void handle(HttpExchange exchange) throws IOException {
        String response = (new Gson()).toJson(Map.of("id_token", VALID_TOKEN));
        exchange.sendResponseHeaders(200, response.length());
        OutputStream responseBody = exchange.getResponseBody();
        responseBody.write(response.getBytes());
        responseBody.close();
        numCalls.incrementAndGet();
      }
    }
  }

  @Test
  public void testCustomCallCredentials() throws Exception {
    // Creates a mock gRPC server for the NBI with an interceptor to verify
    // requests' metadata.
    AtomicInteger numVerifiedOidcTokensReceived = new AtomicInteger(0);
    grpcCleanup.register(
        InProcessServerBuilder.forName(MOCK_NBI_SERVER_NAME)
            .directExecutor()
            .addService(netOpsServiceImpl)
            .intercept(
                new ServerInterceptor() {
                  @Override
                  public <ReqT, RespT> ServerCall.Listener<ReqT> interceptCall(
                      ServerCall<ReqT, RespT> call,
                      Metadata requestHeaders,
                      ServerCallHandler<ReqT, RespT> next) {
                    Key<String> proxyAuthorizationHeaderKey =
                        Key.of("Proxy-Authorization", Metadata.ASCII_STRING_MARSHALLER);
                    assertEquals(
                        "Bearer " + VALID_TOKEN, requestHeaders.get(proxyAuthorizationHeaderKey));
                    numVerifiedOidcTokensReceived.incrementAndGet();
                    return next.startCall(call, requestHeaders);
                  }
                })
            .build()
            .start());
    ManagedChannel channel =
        grpcCleanup.register(
            InProcessChannelBuilder.forName(MOCK_NBI_SERVER_NAME).directExecutor().build());

    // Mocks the listEntities method in the NBI to return an empty response when invoked.
    doAnswer(
            invocation -> {
              var streamObserver =
                  (StreamObserver<ListEntitiesResponse>) invocation.getArguments()[1];
              ResponseHelper.complete(streamObserver, ListEntitiesResponse.newBuilder().build());
              return null;
            })
        .when(netOpsServiceImpl)
        .listEntities(any(ListEntitiesRequest.class), any(StreamObserver.class));

    String privateKey = Resources.toString(Resources.getResource(PATH_TO_PRIVATE_KEY), UTF_8);
    // Because the lifetime of the OIDC Token is set to 0 seconds, each thread calls the
    // mock OIDC Token server to retrieve a new token.
    String oidcTokenServerUrl = "http:/" + oidcTokenServer.server.getAddress().toString() + "/";
    SpacetimeCallCredentials callCredentials =
        SpacetimeCallCredentials.createFromPrivateKey(
            HOST,
            AGENT_EMAIL,
            AGENT_PRIVATE_ID,
            privateKey,
            oidcTokenServerUrl,
            /* oidcTokenLifetimeSeconds= */ Duration.ofSeconds(0),
            /* clock= */ Clock.system(ZoneId.systemDefault()));
    ExecutorService executor = Executors.newFixedThreadPool(NUM_THREADS);
    for (int i = 0; i < NUM_THREADS; ++i) {
      executor.submit(
          () -> {
            NetOpsGrpc.newBlockingStub(channel)
                .withCallCredentials(callCredentials)
                .listEntities(ListEntitiesRequest.newBuilder().build());
          });
    }
    executor.shutdown();
    // Allows a permissive timeout of 10 seconds for all threads to complete.
    assertTrue(executor.awaitTermination(10, TimeUnit.SECONDS));

    // Verifies that each thread successfully received a token from the OIDC Token
    // Service.
    assertEquals(NUM_THREADS, oidcTokenServer.numCalls.get());
    // Verifies that each thread passed a valid token to the mock NBI gRPC Server.
    assertEquals(NUM_THREADS, numVerifiedOidcTokensReceived.get());
  }

  @Test
  public void testTokenRefreshedAtExpirationTime() throws Exception {
    // Creates a mock gRPC server.
    grpcCleanup.register(
        InProcessServerBuilder.forName(MOCK_NBI_SERVER_NAME)
            .directExecutor()
            .addService(netOpsServiceImpl)
            .build()
            .start());
    ManagedChannel channel =
        grpcCleanup.register(
            InProcessChannelBuilder.forName(MOCK_NBI_SERVER_NAME).directExecutor().build());

    // Mocks the listEntities method in the NBI to return an empty response when
    // invoked.
    doAnswer(
            invocation -> {
              var streamObserver =
                  (StreamObserver<ListEntitiesResponse>) invocation.getArguments()[1];
              ResponseHelper.complete(streamObserver, ListEntitiesResponse.newBuilder().build());
              return null;
            })
        .when(netOpsServiceImpl)
        .listEntities(any(ListEntitiesRequest.class), any(StreamObserver.class));

    String privateKey = Resources.toString(Resources.getResource(PATH_TO_PRIVATE_KEY), UTF_8);
    // Mocks the clock object so that the token's expiration time can be simulated.
    Clock fakeClock = mock(Clock.class);
    Instant startClockTime = Instant.ofEpochMilli(0);
    doReturn(startClockTime).when(fakeClock).instant();
    Duration oidcTokenLifetime = Duration.ofHours(1);
    String oidcTokenServerUrl = "http:/" + oidcTokenServer.server.getAddress().toString() + "/";
    SpacetimeCallCredentials callCredentials =
        SpacetimeCallCredentials.createFromPrivateKey(
            HOST,
            AGENT_EMAIL,
            AGENT_PRIVATE_ID,
            privateKey,
            oidcTokenServerUrl,
            oidcTokenLifetime,
            fakeClock);

    // Calls ListEntities so that an initial OIDC token is created.
    NetOpsGrpc.newBlockingStub(channel)
        .withCallCredentials(callCredentials)
        .listEntities(ListEntitiesRequest.newBuilder().build());

    // Sets the clock's current time into the future so that the token is now stale.
    doReturn(startClockTime.plusSeconds(2 * oidcTokenLifetime.getSeconds()))
        .when(fakeClock)
        .instant();
    // Resets the number of calls to refresh the token as recorded by the OIDC Token
    // server.
    oidcTokenServer.resetNumCalls();

    // Across all of the NUM_THREADS concurrent calls to ListEntities, the token should only
    // be refreshed once, and all other threads should read the new value.
    ExecutorService executor = Executors.newFixedThreadPool(NUM_THREADS);
    for (int i = 0; i < NUM_THREADS; ++i) {
      executor.submit(
          () -> {
            NetOpsGrpc.newBlockingStub(channel)
                .withCallCredentials(callCredentials)
                .listEntities(ListEntitiesRequest.newBuilder().build());
          });
    }
    executor.shutdown();
    // Allows a permissive timeout of 10 seconds for all threads to complete.
    assertTrue(executor.awaitTermination(10, TimeUnit.SECONDS));
    // Verifies that the token was only created once.
    assertEquals(1, oidcTokenServer.numCalls.get());
  }
}

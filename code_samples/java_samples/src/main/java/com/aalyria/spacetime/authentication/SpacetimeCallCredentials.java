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

import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.net.HttpURLConnection;
import java.net.MalformedURLException;
import java.net.ProtocolException;
import java.net.URL;
import java.net.URLEncoder;
import java.time.Clock;
import java.time.Instant;
import java.time.ZoneId;
import java.util.Map;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.Executor;

import com.google.gson.Gson;
import com.google.gson.reflect.TypeToken;

import io.grpc.CallCredentials;
import io.grpc.CallCredentials.MetadataApplier;
import io.grpc.CallCredentials.RequestInfo;
import io.grpc.Metadata;

/**
 * A class that supplies per-RPC credentials, which are based on
 * two signed JWTs, one for authenticating to the Spacetime backend
 * and one for authenticating through the secure proxy.
 */
public class SpacetimeCallCredentials extends CallCredentials {
  // A manager for the JWT used to authenticate to the Spacetime backend.
  private final JwtManager spacetimeAuthJwtManager;
  // A manager for the JWT used to authenticate through the secure proxy.
  private final JwtManager proxyAuthJwtManager;
  // An OpenID Connect token that the proxyAuthJwt's value is exchanged to receive.
  private String oidcToken = "";
  private long oidcTokenExpirationTimeEpochSeconds;

  // Spacetime-specific parameters.
  private final static String GCP_OIDC_TOKEN_CREATION_URL = "https://www.googleapis.com/oauth2/v4/token";
  private final static String PROXY_TARGET_AUDIENCE = "60292403139-me68tjgajl5dcdbpnlm2ek830lvsnslq.apps.googleusercontent.com";
  // If the OIDC token will expire within this margin, it will be re-created.
  private static final int OIDC_TOKEN_EXPIRATION_TIME_MARGIN_SECONDS = 300;
  // The OIDC tokens are valid for 1 hour.
  private static final long OIDC_TOKEN_LIFETIME_SECONDS = 3600;

  private static final Gson gson = new Gson();
  private static final Clock clock = Clock.system(ZoneId.systemDefault());

  static public SpacetimeCallCredentials createFromPrivateKey(String host, String agentEmail,
          String privateKeyId, String privateKey) {
      JwtManager spacetimeAuthJwtManager = new JwtManager.Builder()
              .setIssuer(agentEmail)
              .setSubject(agentEmail)
              .setAudience(host)
              .setPrivateKeyId(privateKeyId)
              .setPrivateKey(privateKey).build();
      JwtManager proxyAuthJwtManager = new JwtManager.Builder()
              .setIssuer(agentEmail)
              .setSubject(agentEmail)
              .setAudience(GCP_OIDC_TOKEN_CREATION_URL)
              .setTargetAudience(PROXY_TARGET_AUDIENCE)
              .setPrivateKeyId(privateKeyId)
              .setPrivateKey(privateKey).build();
      return new SpacetimeCallCredentials(spacetimeAuthJwtManager, proxyAuthJwtManager);
  }

  static public SpacetimeCallCredentials createFromJwt(String spacetimeAuthJwt, String proxyAuthJwt) {
      JwtManager spacetimeAuthJwtManager = new JwtManager(spacetimeAuthJwt);
      JwtManager proxyAuthJwtManager = new JwtManager(proxyAuthJwt);
      return new SpacetimeCallCredentials(spacetimeAuthJwtManager, proxyAuthJwtManager);
  }

  private SpacetimeCallCredentials(JwtManager spacetimeAuthJwtManager, JwtManager proxyAuthJwtManager) {
    this.spacetimeAuthJwtManager = spacetimeAuthJwtManager;
    this.proxyAuthJwtManager = proxyAuthJwtManager;
  }

  // This method does not block as requested by the gRPC documentation, 
  // but the RPC will not proceed until the .apply method is called in the
  // async execution.
  @Override
  public void applyRequestMetadata(RequestInfo requestInfo, Executor executor,
          MetadataApplier metadataApplier) {
    CompletableFuture.supplyAsync(() -> {
        // If the OIDC token is expired, or within OIDC_TOKEN_EXPIRATION_TIME_MARGIN_SECONDS
        // of its expiration time, then the proxyAuthJwt should be regenerated. 
        Instant now = clock.instant();
        if (oidcToken.isEmpty() || 
                now.plusSeconds(OIDC_TOKEN_EXPIRATION_TIME_MARGIN_SECONDS)
                        .getEpochSecond() > oidcTokenExpirationTimeEpochSeconds) {
            String proxyAuthJwt = proxyAuthJwtManager.generateJwt();
            oidcTokenExpirationTimeEpochSeconds = now.plusSeconds(OIDC_TOKEN_LIFETIME_SECONDS).getEpochSecond(); 
            oidcToken = exchangeProxyAuthJwtForOidcToken(proxyAuthJwt);
        } 
        return oidcToken;
    }).thenAcceptAsync((String oidcToken) -> {
        Metadata headers = new Metadata();
        Metadata.Key<String> authorizationHeaderKey = Metadata.Key.of(
            "Authorization",
            Metadata.ASCII_STRING_MARSHALLER);
        headers.put(authorizationHeaderKey, "Bearer " + spacetimeAuthJwtManager.generateJwt());
        Metadata.Key<String> proxyAuthorizationKey = Metadata.Key.of(
            "Proxy-Authorization",
            Metadata.ASCII_STRING_MARSHALLER);
        headers.put(proxyAuthorizationKey, "Bearer " + oidcToken);
        metadataApplier.apply(headers);
    }, executor);
  }

  @Override
  public void thisUsesUnstableApi() {}

  // Takes a map of key, value pairs representing URL parameters and
  // encodes them so they can be sent in the URL of an HTTP request.
  private static String encodeUrlParams(Map<String, String> urlParams) {
    StringBuilder encodedUrlParams = new StringBuilder();
    for (Map.Entry<String, String> urlParam : urlParams.entrySet()) {
      if (encodedUrlParams.length() != 0) {
        encodedUrlParams.append("&");
      }
      encodedUrlParams.append(URLEncoder.encode(urlParam.getKey(), UTF_8));
      encodedUrlParams.append("=");
      encodedUrlParams.append(URLEncoder.encode(urlParam.getValue(), UTF_8));
    }
    return encodedUrlParams.toString();
  }

  // Exchanges the proxyAuthJwt for an OpenID Connect token.
  private static String exchangeProxyAuthJwtForOidcToken(String proxyAuthJwtValue) {
    // Constructs the URL parameters to fetch an OpenID Connect token.
    Map<String, String> urlParams = Map.of(
        "grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer",
        "assertion", proxyAuthJwtValue);
    byte[] postData = {};
    String encodedUrlParams = encodeUrlParams(urlParams);
    postData = encodedUrlParams.getBytes(UTF_8);

    String request_url = GCP_OIDC_TOKEN_CREATION_URL;
    StringBuilder response = new StringBuilder();
    try {
      // Creates the request.
      URL url = new URL(request_url);
      HttpURLConnection connection = (HttpURLConnection) url.openConnection();
      connection.setDoOutput(true);
      connection.setRequestMethod("POST");
      connection.setRequestProperty("Content-Type", "application/x-www-form-urlencoded");
      connection.setRequestProperty("Content-Length", Integer.toString(postData.length));
      connection.getOutputStream().write(postData);

      // Reads the response, which contains the OpenID Connect token.
      BufferedReader in = new BufferedReader(new InputStreamReader(connection.getInputStream()));
      String decodedString;
      while ((decodedString = in.readLine()) != null) {
        response.append(decodedString);
      }
      in.close();
    } catch (MalformedURLException e) {
      throw new RuntimeException("Error creating a URL from " + request_url, e);
    } catch (ProtocolException e) {
      throw new RuntimeException("Error setting the POST method", e);
    } catch (IOException e) {
      throw new RuntimeException("Error reading or writing from the connection's stream", e);
    }

    // Parses the OpenID Connect token.
    TypeToken<Map<String, String>> mapType = new TypeToken<Map<String, String>>() {};
    return gson.fromJson(response.toString(), mapType).get("id_token");
  }
}

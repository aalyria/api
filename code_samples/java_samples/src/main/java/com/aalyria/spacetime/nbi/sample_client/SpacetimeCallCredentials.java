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

import static java.nio.charset.StandardCharsets.UTF_8;

import java.io.BufferedReader;
import java.io.IOException;
import java.io.InputStreamReader;
import java.io.UnsupportedEncodingException;
import java.net.HttpURLConnection;
import java.net.MalformedURLException;
import java.net.ProtocolException;
import java.net.URL;
import java.net.URLEncoder;
import java.util.Map;
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

    // A JWT for authenticating to the Spacetime backend.
  private String spacetimeAuthJwt;
  // An OpenID Connect token used to authenticate through the secure proxy.
  private String oidcToken;

  private static final Gson gson = new Gson();

  static public SpacetimeCallCredentials createFromJwt(String spacetimeAuthJwt, String proxyAuthJwt) {
      return new SpacetimeCallCredentials(spacetimeAuthJwt, exchangeProxyAuthJwtForOidcToken(proxyAuthJwt));
  }

  private SpacetimeCallCredentials(String spacetimeAuthJwt, String oidcToken) {
    this.spacetimeAuthJwt = spacetimeAuthJwt;
    this.oidcToken = oidcToken;
  }

  @Override
  public void applyRequestMetadata(RequestInfo requestInfo, Executor executor,
          MetadataApplier metadataApplier) {
    executor.execute(() -> {
      Metadata headers = new Metadata();
      Metadata.Key<String> authorizationHeaderKey = Metadata.Key.of(
          "Authorization",
          Metadata.ASCII_STRING_MARSHALLER);
      headers.put(authorizationHeaderKey, "Bearer " + spacetimeAuthJwt);
      Metadata.Key<String> proxyAuthorizationKey = Metadata.Key.of(
          "Proxy-Authorization",
          Metadata.ASCII_STRING_MARSHALLER);
      headers.put(proxyAuthorizationKey, "Bearer " + oidcToken);
      metadataApplier.apply(headers);
    });
  }

  @Override
  public void thisUsesUnstableApi() {}

  // Takes a map of key, value pairs representing URL parameters and
  // encodes them so they can be sent in the URL of an HTTP request.
  private static String encodeUrlParams(Map<String, String> urlParams) throws UnsupportedEncodingException {
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
  private static String exchangeProxyAuthJwtForOidcToken(String proxyAuthJwt) {
    // Constructs the URL parameters to fetch an OpenID Connect token.
    Map<String, String> urlParams = Map.of(
        "grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer",
        "assertion", proxyAuthJwt);
    byte[] postData = {};
    try {
      String encodedUrlParams = encodeUrlParams(urlParams);
      postData = encodedUrlParams.getBytes(UTF_8);
    } catch (UnsupportedEncodingException e) {
      throw new RuntimeException("Error parsing URL params", e);
    }

    String request_url = "https://www.googleapis.com/oauth2/v4/token";
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

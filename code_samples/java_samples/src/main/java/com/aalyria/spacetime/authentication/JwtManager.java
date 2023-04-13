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

import java.io.ByteArrayOutputStream;
import java.io.IOException;
import java.nio.file.Files;
import java.nio.file.Paths;
import java.security.InvalidKeyException;
import java.security.KeyFactory;
import java.security.NoSuchAlgorithmException;
import java.security.PrivateKey;
import java.security.Signature;
import java.security.SignatureException;
import java.security.spec.InvalidKeySpecException;
import java.security.spec.PKCS8EncodedKeySpec;
import java.time.Clock;
import java.time.Duration;
import java.time.Instant;
import java.time.ZoneId;
import java.util.Base64;
import java.util.HashMap;
import java.util.Map;
import java.util.Objects;
import java.util.Optional;

import org.bouncycastle.asn1.ASN1Sequence;
import org.bouncycastle.asn1.DERNull;
import org.bouncycastle.asn1.pkcs.PKCSObjectIdentifiers;
import org.bouncycastle.asn1.pkcs.PrivateKeyInfo;
import org.bouncycastle.asn1.x509.AlgorithmIdentifier;

import com.google.gson.Gson;

/** 
 * This class handles creating the JWTs that are required to authenticate to Spacetime.
 */ 
public class JwtManager {
    private final Duration lifetime;
    private long issueTimeEpochSeconds;
    private long expirationTimeEpochSeconds;
    private final String audience;
    private final String issuer;
    private final String subject;
    private final String targetAudience;
    private final String privateKeyId;
    // The RSA private key in either PKCS8 or PKCS1, ASN.1 DER form
    // (PEM-encoded) used to sign the JWT.
    private final String privateKey;
    private String jwtValue = "";

    private static final String PRIVATE_KEY_HEADER = "-----BEGIN RSA PRIVATE KEY-----";
    private static final String PRIVATE_KEY_FOOTER = "-----END RSA PRIVATE KEY-----";
    private static final Gson gson = new Gson();
    private static final Clock clock = Clock.system(ZoneId.systemDefault());

    public static class Builder {
        private Duration lifetime = Duration.ofHours(1);
        private String audience = "";
        private String issuer = "";
        private String subject = "";
        private String targetAudience = "";
        private String privateKeyId = "";
        private String privateKey = "";

        public Builder setIssuer(String issuer) {
            this.issuer = Objects.requireNonNull(issuer);
            return this;
        }

        public Builder setSubject(String subject) {
            this.subject = Objects.requireNonNull(subject);
            return this;
        }

        public Builder setAudience(String audience) {
            this.audience = Objects.requireNonNull(audience);
            return this;
        }

        public Builder setTargetAudience(String targetAudience) {
            this.targetAudience = Objects.requireNonNull(targetAudience);
            return this;
        }

        public Builder setPrivateKeyId(String privateKeyId) {
            this.privateKeyId = Objects.requireNonNull(privateKeyId);
            return this;
        }

        public Builder setPrivateKey(String privateKey) {
            this.privateKey = Objects.requireNonNull(privateKey);
            return this;
        }

        public Builder setLifetime(Duration lifetime) {
            this.lifetime = Objects.requireNonNull(lifetime);
            return this;
        }

        public JwtManager build() {
            return new JwtManager(lifetime, audience, issuer, subject,
                    targetAudience, privateKeyId, privateKey);
        }
    }

    private JwtManager(Duration lifetime,
            String audience,
            String issuer,
            String subject,
            String targetAudience,
            String privateKeyId,
            String privateKey) {
        this.lifetime = lifetime;
        this.audience = audience;
        this.issuer = issuer;
        this.subject = !subject.isEmpty() ? subject : issuer;
        this.targetAudience = targetAudience;
        this.privateKeyId = privateKeyId;
        this.privateKey = privateKey;
        this.jwtValue = generateJwt();
    }

    // If the user passes in a JWT, this string will be used as the token
    // and will not be refreshed. 
    public JwtManager(String jwtValue) {
        lifetime = Duration.ofHours(1);
        expirationTimeEpochSeconds = Long.MAX_VALUE;
        audience = "";
        issuer = "";
        subject = "";
        targetAudience = "";
        privateKeyId = "";
        privateKey = "";
        this.jwtValue = jwtValue;
    }

    private Optional<PrivateKey> parsePrivateKey(byte[] encodedKey, KeyFactory keyFactory) {
        try {
            PKCS8EncodedKeySpec keySpec = new PKCS8EncodedKeySpec(encodedKey);
            return Optional.of(keyFactory.generatePrivate(keySpec));
        } catch (InvalidKeySpecException e) {
            return Optional.empty();
        }
    }

    private PrivateKey decodePrivateKey() {
        String encodedKeyValue = privateKey.replace(PRIVATE_KEY_HEADER, /* replacement= */"")
                .replaceAll(System.lineSeparator(), /* replacement= */"")
                .replace(PRIVATE_KEY_FOOTER, /* replacement= */"");
        byte[] encodedKey = Base64.getDecoder().decode(encodedKeyValue);

        Optional<PrivateKey> privateKey;
        try {
            // Assumes the key is in PKCS8 format.
            KeyFactory keyFactory = KeyFactory.getInstance("RSA");
            privateKey = parsePrivateKey(encodedKey, keyFactory);
            if (privateKey.isEmpty()) {
                // If the key could not be parsed in PKCS8 format, it should be in PKCS1
                // format.
                AlgorithmIdentifier algorithmIdentifier = new AlgorithmIdentifier(
                        PKCSObjectIdentifiers.rsaEncryption, DERNull.INSTANCE);
                PrivateKeyInfo privateKeyInfo = new PrivateKeyInfo(algorithmIdentifier,
                        ASN1Sequence.getInstance(encodedKey));
                privateKey = parsePrivateKey(privateKeyInfo.getEncoded(), keyFactory);
            }
        } catch (NoSuchAlgorithmException e) {
            throw new IllegalArgumentException("Algorithm for KeyFactory not found.", e);
        } catch (IOException e) {
            throw new IllegalArgumentException("Invalid private key.", e);
        }
        return privateKey.orElseThrow(() -> new IllegalArgumentException(
                "The private key could not be decoded."));
    }

    private byte[] generateHeader() {
        Map<String, String> header = Map.of(
                "alg", "RS256",
                "typ", "JWT",
                "kid", privateKeyId);
        return Base64.getEncoder().encode(gson.toJson(header).getBytes());
    }

    private byte[] generatePayload() {
        Instant now = clock.instant();
        issueTimeEpochSeconds = now.getEpochSecond();
        expirationTimeEpochSeconds = now.plusSeconds(lifetime.getSeconds()).getEpochSecond();
        Map<String, String> payload = new HashMap<>(Map.of(
                "aud", audience,
                "exp", String.valueOf(expirationTimeEpochSeconds),
                "iat", String.valueOf(issueTimeEpochSeconds),
                "iss", issuer,
                "sub", subject));
        if (!targetAudience.isEmpty()) {
            payload.put("target_audience", targetAudience);
        }
        return Base64.getEncoder().encode(gson.toJson(payload).getBytes());
    }

    private byte[] generateSignature(byte[] header, byte[] payload, PrivateKey privateKey) {
        try {
            ByteArrayOutputStream outputStream = new ByteArrayOutputStream();
            outputStream.write(header);
            outputStream.write((new String(".").getBytes()));
            outputStream.write(payload);

            Signature signature = Signature.getInstance("SHA256withRSA");
            signature.initSign(privateKey);
            signature.update(outputStream.toByteArray());
            return Base64.getEncoder().encode(signature.sign());
        } catch (IOException e) {
            throw new RuntimeException("Signature could not be written.", e);
        } catch (NoSuchAlgorithmException e) {
            throw new RuntimeException("Signature algorithm was invalid.", e);
        } catch (InvalidKeyException e) {
            throw new RuntimeException("Private key used in signature was invalid.", e);
        } catch (SignatureException e) {
            throw new RuntimeException("Signature could not be created.", e);
        }
    }

    public String generateJwt() {
        PrivateKey privateKey = decodePrivateKey();
        byte[] header = generateHeader();
        byte[] payload = generatePayload();
        byte[] signature = generateSignature(header, payload, privateKey);

        ByteArrayOutputStream jwtOutputStream = new ByteArrayOutputStream();
        byte[] separator = new String(".").getBytes();
        try {
            jwtOutputStream.write(header);
            jwtOutputStream.write(separator);
            jwtOutputStream.write(payload);
            jwtOutputStream.write(separator);
            jwtOutputStream.write(signature);
        } catch (IOException e) {
            throw new RuntimeException("Error writing JWT.", e);
        }
        jwtValue = jwtOutputStream.toString();
        return jwtValue;
    }

    // Returns the existing JWT.
    public String getJwt() {
        return jwtValue;
    }
}

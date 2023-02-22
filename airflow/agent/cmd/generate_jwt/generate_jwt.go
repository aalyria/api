// package main provides a simple utility to generate signed JWTs.
//
// Copyright (c) Aalyria Technologies, Inc., and its affiliates.
// Confidential and Proprietary. All rights reserved.
package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "fatal error: %s\n", err)
		os.Exit(2)
	}
}

func run(ctx context.Context) error {
	fs := flag.NewFlagSet("generate_jwt", flag.ContinueOnError)

	var (
		lifetime       = fs.Duration("lifetime", 1*time.Hour, "Lifetime that the JWT will be valid for")
		audience       = fs.String("audience", "", "Audience (aud) for the JWT")
		issuer         = fs.String("issuer", "", "Issuer (iss) for the JWT")
		subject        = fs.String("subject", "", "Subject (sub) for the JWT; defaults to the issuer")
		targetAudience = fs.String("target-audience", "", "JWT 'target_audience' field")
		keyID          = fs.String("key-id", "", "The ID corresponding to the private key")
		privKeyPath    = fs.String("private-key", "", "Path to the RSA private key file in either PKCS8 or PKCS1, ASN.1 DER form (PEM-encoded) used to sign the JWT")
	)

	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}
	if *audience == "" {
		return errors.New("no audience provided")
	}
	if *issuer == "" {
		return errors.New("no issuer provided")
	}
	if *keyID == "" {
		return errors.New("no key-id provided")
	}
	if *subject == "" {
		subject = issuer
	}

	jwt, err := generateJWT(*issuer, *subject, *audience, *targetAudience, *keyID, *privKeyPath, *lifetime)
	if err != nil {
		return err
	}

	fmt.Println(string(jwt))
	return nil
}

func generateJWT(issuer, subject, audience, targetAudience, keyID, keyPath string, lifetime time.Duration) ([]byte, error) {
	privKey, err := decodePrivateKey(keyPath)
	if err != nil {
		return nil, err
	}

	headerBytes, err := generateHeader(keyID)
	if err != nil {
		return nil, err
	}
	payloadBytes, err := generatePayload(issuer, subject, audience, targetAudience, lifetime)
	if err != nil {
		return nil, err
	}
	sigBytes, err := generateSignature(headerBytes, payloadBytes, privKey)
	if err != nil {
		return nil, err
	}
	return bytes.Join([][]byte{headerBytes, payloadBytes, sigBytes}, []byte(".")), nil
}

func generateHeader(keyID string) ([]byte, error) {
	return base64EncodeJSON(map[string]interface{}{
		"alg": "RS256",
		"typ": "JWT",
		"kid": keyID,
	})
}

func generatePayload(issuer, subject, audience, targetAudience string, lifetime time.Duration) ([]byte, error) {
	now := time.Now()
	payload := map[string]interface{}{
		"aud": audience,
		"exp": now.Add(lifetime).Unix(),
		"iat": now.Unix(),
		"iss": issuer,
		"sub": subject,
	}
	if targetAudience != "" {
		payload["target_audience"] = targetAudience
	}
	return base64EncodeJSON(payload)
}

func generateSignature(headerBytes, payloadBytes []byte, privKey *rsa.PrivateKey) ([]byte, error) {
	hasher := sha256.New()
	hasher.Write(headerBytes)
	hasher.Write([]byte("."))
	hasher.Write(payloadBytes)

	rawSig, err := rsa.SignPKCS1v15(rand.Reader, privKey, crypto.SHA256, hasher.Sum(nil))
	if err != nil {
		return nil, err
	}
	return base64Encode(rawSig)
}

func decodePrivateKey(keyPath string) (*rsa.PrivateKey, error) {
	f, err := os.Open(keyPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	keyBytes, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(keyBytes)

	return parsePrivateKey(block.Bytes)
}

func parsePrivateKey(data []byte) (*rsa.PrivateKey, error) {
	errs := []error{}

	pkcs8Key, err := x509.ParsePKCS8PrivateKey(data)
	if err == nil {
		if rsaPrivKey, ok := pkcs8Key.(*rsa.PrivateKey); ok {
			return rsaPrivKey, nil
		}
		errs = append(errs, fmt.Errorf("unexpected pkcs8 key type: %v", pkcs8Key))
	} else {
		errs = append(errs, fmt.Errorf("error parsing key as pkcs8: %w", err))
	}

	pkcs1Key, err := x509.ParsePKCS1PrivateKey(data)
	if err == nil {
		return pkcs1Key, nil
	} else {
		errs = append(errs, fmt.Errorf("error parsing key as pkcs1: %w", err))
	}

	return nil, errors.Join(errs...)
}

func base64EncodeJSON(inp map[string]interface{}) ([]byte, error) {
	data, err := json.Marshal(inp)
	if err != nil {
		return nil, err
	}
	return base64Encode(data)
}

func base64Encode(data []byte) ([]byte, error) {
	buf := bytes.Buffer{}
	enc := base64.NewEncoder(base64.RawURLEncoding, &buf)
	if _, err := enc.Write(data); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

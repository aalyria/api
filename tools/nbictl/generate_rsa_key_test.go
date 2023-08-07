// Copyright 2023 Aalyria Technologies, Inc., and its affiliates.
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

package nbictl

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"os"
	"os/exec"
	"testing"

	"github.com/bazelbuild/rules_go/go/tools/bazel"
)

const (
	exampleCertCountry      = "example.country"
	exampleCertOrganization = "example.organization"
	exampleCertState        = "example.state"
	exampleCertLocation     = "example.location"
)

func TestGenerateKey_ValiddateWithOpenSSL(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skipf("unable to find openssl path: %v", err)
	}

	nbictlConfig, err := bazel.NewTmpDir("nbictl")
	if err != nil {
		t.Fatal(err)
	}

	rsaKeyPath, err := GenerateRSAKeys(nbictlConfig, "", exampleCertOrganization, "", "")
	if err != nil {
		t.Fatalf("unable to generate RSA keys: %v", err)
	}
	privCmd := exec.Command("openssl", "rsa", "-noout", "-modulus", "-in", rsaKeyPath.PrivateKeyPath)
	certCmd := exec.Command("openssl", "x509", "-noout", "-modulus", "-in", rsaKeyPath.CertificatePath)

	privOutput, err := privCmd.Output()
	if err != nil {
		t.Fatalf("unable to run the openssl command for private key: %v", err)
	}

	pubOutput, err := certCmd.Output()
	if err != nil {
		t.Fatalf("unable to run the openssl command for public key: %v", err)
	}

	if !bytes.Equal(privOutput, pubOutput) {
		t.Fatalf("modulus mismatch: got %v from key but %v from cert", privOutput, pubOutput)
	}
}

func TestGenerateKey_ValidateWithGoLib(t *testing.T) {
	t.Parallel()
	nbictlConfig, err := bazel.NewTmpDir("nbictl")
	if err != nil {
		t.Fatal(err)
	}

	rsaKeyPath, err := GenerateRSAKeys(nbictlConfig, "", exampleCertOrganization, "", "")
	if err != nil {
		t.Fatalf("unable to generate RSA keys: %v", err)
	}

	rawPrivateKey, err := os.ReadFile(rsaKeyPath.PrivateKeyPath)
	if err != nil {
		t.Fatalf("failed to read file containing private key: %v", err)
	}

	rawCert, err := os.ReadFile(rsaKeyPath.CertificatePath)
	if err != nil {
		t.Fatalf("failed to read file containing certificate: %v", err)
	}

	pemPrivBlock, _ := pem.Decode(rawPrivateKey)
	pemCrtBlock, _ := pem.Decode(rawCert)

	if _, err := x509.ParsePKCS1PrivateKey(pemPrivBlock.Bytes); err != nil {
		t.Fatalf("failed to parse private key. not a valid private key: %v", err)
	}

	if _, err = x509.ParseCertificate(pemCrtBlock.Bytes); err != nil {
		t.Fatalf("failed to parse certificate: not a valid certificate: %v", err)
	}
}

func TestGenerateKey_ValidateSubjectAndIssuer(t *testing.T) {
	t.Parallel()
	nbictlConfig, err := bazel.NewTmpDir("nbictl")
	if err != nil {
		t.Fatal(err)
	}

	rsaKeyPath, err := GenerateRSAKeys(nbictlConfig, exampleCertCountry, exampleCertOrganization, exampleCertState, exampleCertLocation)
	if err != nil {
		t.Fatalf("unable to generate RSA keys: %v", err)
	}

	rawCert, err := os.ReadFile(rsaKeyPath.CertificatePath)
	if err != nil {
		t.Fatalf("failed to read file containing certificate: %v", err)
	}

	pemCrtBlock, _ := pem.Decode(rawCert)

	cert, err := x509.ParseCertificate(pemCrtBlock.Bytes)
	if err != nil {
		t.Fatalf("failed to parse certificate: %v", err)
	}

	switch {
	case cert.Subject.Country[0] != exampleCertCountry:
		t.Fatalf("subject country mismatch: want %s got %s", exampleCertCountry, cert.Subject.Country[0])
	case cert.Subject.Organization[0] != exampleCertOrganization:
		t.Fatalf("subject organization mismatch: want %s got %s", exampleCertOrganization, cert.Subject.Organization[0])
	case cert.Subject.Province[0] != exampleCertState:
		t.Fatalf("subject state mismatch: want %s got %s", exampleCertState, cert.Subject.Province[0])
	case cert.Subject.Locality[0] != exampleCertLocation:
		t.Fatalf("subject location mismatch: want %s got %s", exampleCertLocation, cert.Subject.Locality[0])
	}

	switch {
	case cert.Issuer.Country[0] != exampleCertCountry:
		t.Fatalf("issuer country mismatch: want %s got %s", exampleCertCountry, cert.Subject.Country[0])
	case cert.Issuer.Organization[0] != exampleCertOrganization:
		t.Fatalf("issuer organization mismatch: want %s got %s", exampleCertOrganization, cert.Subject.Organization[0])
	case cert.Issuer.Province[0] != exampleCertState:
		t.Fatalf("issuer state mismatch: want %s got %s", exampleCertState, cert.Subject.Province[0])
	case cert.Issuer.Locality[0] != exampleCertLocation:
		t.Fatalf("issuer location mismatch: want %s got %s", exampleCertLocation, cert.Subject.Locality[0])
	}
}

func TestGenerateKey_FilePermission(t *testing.T) {
	t.Parallel()
	nbictlConfig, err := bazel.NewTmpDir("nbictl")
	if err != nil {
		t.Fatal(err)
	}

	rsaKeyPath, err := GenerateRSAKeys(nbictlConfig, "", exampleCertOrganization, "", "")
	if err != nil {
		t.Fatalf("unable to generate RSA keys: %v", err)
	}

	privKeyInfo, err := os.Stat(rsaKeyPath.PrivateKeyPath)
	if err != nil {
		t.Fatalf("unable to get file info: %v", err)
	}

	privFilePerm := privKeyInfo.Mode().Perm()
	if privFilePerm != os.FileMode(privateKeysFilePerm) {
		t.Errorf("file must have permission %d, but has %s", privateKeysFilePerm, privFilePerm.String())
	}

	pubCertKeyInfo, err := os.Stat(rsaKeyPath.CertificatePath)
	if err != nil {
		t.Errorf("unable to get file info: %v", err)
	}

	pubCertPerm := pubCertKeyInfo.Mode().Perm()
	if pubCertPerm != os.FileMode(pubCertFilePerm) {
		t.Fatalf("file must have permission %d, but has %s", pubCertFilePerm, pubCertPerm.String())
	}
}

func TestGenerateKey_DirPermision(t *testing.T) {
	t.Parallel()
	nbictlConfig, err := bazel.NewTmpDir("test_nbictl")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(nbictlConfig, 0755); err != nil {
		t.Fatal(err)
	}

	if _, err := GenerateRSAKeys(nbictlConfig, "", exampleCertOrganization, "", ""); err == nil {
		t.Fatalf("unable to detect wrong directory permission: %v", err)
	}
}

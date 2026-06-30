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
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"time"

	"github.com/urfave/cli/v2"
)

const (
	rsaKeysBitSize           = 4096
	generatedKeysDirDefault  = "keys"
	defaultExpirationInYears = 1
	lenKeyFileName           = 12
	generatedKeysDirPerm     = os.FileMode(0o700)
	privateKeysFilePerm      = os.FileMode(0o600)
	pubCertFilePerm          = os.FileMode(0o644)
)

type RSAKeyPath struct {
	PrivateKeyPath  string
	CertificatePath string
}

func GenerateKeys(appCtx *cli.Context) error {
	directory := appCtx.String("dir")
	country := appCtx.String("country")
	org := appCtx.String("org")
	state := appCtx.String("state")
	location := appCtx.String("location")

	certIssuer := pkix.Name{}

	if org == "" {
		return errors.New("missing required key --org: organization for the certification must be provided")
	} else {
		certIssuer.Organization = []string{org}
	}

	if country != "" {
		certIssuer.Country = []string{country}
	}
	if state != "" {
		certIssuer.Province = []string{state}
	}
	if location != "" {
		certIssuer.Locality = []string{location}
	}

	generatedKeysDir := directory
	if generatedKeysDir == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		generatedKeysDir = filepath.Join(configDir, appCtx.App.Name, generatedKeysDirDefault)
	}

	if err := os.MkdirAll(generatedKeysDir, generatedKeysDirPerm); err != nil {
		return err
	}

	dirInfo, err := os.Stat(generatedKeysDir)
	if err != nil {
		return fmt.Errorf("unable to get directory info: %w", err)
	}

	if dirPerm := dirInfo.Mode().Perm(); dirPerm != generatedKeysDirPerm {
		return fmt.Errorf("directory does not have an appropriate permission: must have %v but have %v", generatedKeysDirPerm, dirPerm)
	}

	now := time.Now()
	certSerialNumber, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		return err
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, rsaKeysBitSize)
	if err != nil {
		return fmt.Errorf("unable to generate private key: %w", err)
	}

	publicKey := privateKey.PublicKey
	publicKeyBytes := x509.MarshalPKCS1PublicKey(&publicKey)
	shaPubKey := sha1.Sum(publicKeyBytes)

	authorityKeyId := shaPubKey[:]

	certTemplate := &x509.Certificate{
		SerialNumber:          certSerialNumber,
		Subject:               certIssuer,
		Issuer:                certIssuer,
		NotBefore:             now,
		NotAfter:              now.AddDate(defaultExpirationInYears, 0, 0),
		ExtKeyUsage:           []x509.ExtKeyUsage{},
		AuthorityKeyId:        authorityKeyId,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	cert, err := x509.CreateCertificate(rand.Reader, certTemplate, certTemplate, &publicKey, privateKey)
	if err != nil {
		return fmt.Errorf("unable to create certificate: %w", err)
	}

	pemPrivateBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	pemCertBlock := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert,
	}

	shaCert := sha256.Sum256(cert)

	rsaKeyPaths := RSAKeyPath{
		PrivateKeyPath:  filepath.Join(generatedKeysDir, hex.EncodeToString(shaCert[:lenKeyFileName])+".key"),
		CertificatePath: filepath.Join(generatedKeysDir, hex.EncodeToString(shaCert[:lenKeyFileName])+".crt"),
	}

	privFile, err := os.OpenFile(rsaKeyPaths.PrivateKeyPath, os.O_CREATE|os.O_RDWR|os.O_EXCL, privateKeysFilePerm)
	if err != nil {
		return fmt.Errorf("unable to create file: %w", err)
	}
	defer privFile.Close()

	pubFile, err := os.OpenFile(rsaKeyPaths.CertificatePath, os.O_CREATE|os.O_RDWR|os.O_EXCL, pubCertFilePerm)
	if err != nil {
		return fmt.Errorf("unable to create file: %w", err)
	}
	defer pubFile.Close()

	if err = pem.Encode(privFile, pemPrivateBlock); err != nil {
		return fmt.Errorf("unable to encode private key: %w", err)
	}

	if err := pem.Encode(pubFile, pemCertBlock); err != nil {
		return fmt.Errorf("unable to encode certificate: %w", err)
	}

	fmt.Fprintf(appCtx.App.ErrWriter, "private key is stored under: %s\n", rsaKeyPaths.PrivateKeyPath)
	fmt.Fprintf(appCtx.App.ErrWriter, "certificate is stored under: %s\n", rsaKeyPaths.CertificatePath)
	return nil
}

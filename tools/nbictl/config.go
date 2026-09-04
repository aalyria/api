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

package nbictl

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"

	"aalyria.com/spacetime/tools/nbictl/nbictlpb"
	"github.com/samber/lo"
	"github.com/urfave/cli/v2"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

func compareEndpoints(endpoint1, endpoint2 string) bool {
	// N.B.: does not support IPv6 string literals.
	if !strings.Contains(endpoint1, ":") {
		endpoint1 = endpoint1 + ":443"
	}
	if !strings.Contains(endpoint2, ":") {
		endpoint2 = endpoint2 + ":443"
	}

	h1, p1, err := net.SplitHostPort(endpoint1)
	if err != nil {
		return false
	}

	h2, p2, err := net.SplitHostPort(endpoint2)
	if err != nil {
		return false
	}

	return strings.EqualFold(h1, h2) && p1 == p2
}

func getAppConfDir(appCtx *cli.Context) (string, error) {
	if appCtx.IsSet("config_dir") {
		appConfDir := appCtx.String("config_dir")
		if appConfDir == "" {
			return "", errors.New("--config_dir can't be empty")
		}
		return appConfDir, nil
	}

	confDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("unable to obtain the default config directory: %w", err)
	}
	return filepath.Join(confDir, appCtx.App.Name), nil
}

func readConfig(profileName, confFilePath string) (*nbictlpb.Config, error) {
	confs, err := readConfigs(confFilePath)
	if err != nil {
		return nil, fmt.Errorf("unable to get config profiles: %w", err)
	}

	// If the profile name is not specified and there is only one context in the config file
	// the function will return that configuration context
	if profileName == "" {
		switch {
		case len(confs.GetConfigs()) == 1:
			return confs.GetConfigs()[0], nil
		default:
			return nil, errors.New("--profile flag required because there are multiple profiles defined in the configuration.")
		}
	}

	allProfileNames := []string{}
	matchingProfilesByHostPort := []*nbictlpb.Config{}
	for _, conf := range confs.GetConfigs() {
		if conf.GetName() == profileName {
			return conf, nil
		}
		allProfileNames = append(allProfileNames, conf.GetName())

		if compareEndpoints(profileName, conf.GetUrl()) {
			matchingProfilesByHostPort = append(matchingProfilesByHostPort, conf)
		}
	}
	if len(matchingProfilesByHostPort) == 1 {
		// Did not find profile by name above, but found one (and only one) match by host:port.
		return matchingProfilesByHostPort[0], nil
	}

	errMsg := fmt.Sprintf("unable to get the profile with the name: %q (expected one of [%s])", profileName, strings.Join(allProfileNames, ", "))
	if urlMatchCount := len(matchingProfilesByHostPort); urlMatchCount > 1 {
		errMsg += fmt.Sprintf("; additionally, profile match by URL found multiple matches (%d): [%s]",
			urlMatchCount,
			strings.Join(lo.Map(matchingProfilesByHostPort, func(c *nbictlpb.Config, _ int) string { return c.GetName() }), ", "))
	}
	return nil, fmt.Errorf("%s", errMsg)
}

func readConfigs(confFilePath string) (*nbictlpb.AppConfig, error) {
	confProto := &nbictlpb.AppConfig{}
	confBytes, err := os.ReadFile(confFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("unable to read file: %w.\nSee `%s config -h` to learn how to configure the tool.", err, appName)
		}
		return nil, fmt.Errorf("unable to read file: %w", err)
	}

	if err := prototext.Unmarshal(confBytes, confProto); err != nil {
		return nil, fmt.Errorf("invalid file content: %w", err)
	}
	return confProto, nil
}

func getConfFileForContext(appCtx *cli.Context) (string, error) {
	confDir, err := getAppConfDir(appCtx)
	if err != nil {
		return "", fmt.Errorf("unable to obtain the default config directory: %w", err)
	}

	return filepath.Join(confDir, confFileName), nil
}

func ListConfigs(appCtx *cli.Context) error {
	confFile, err := getConfFileForContext(appCtx)
	if err != nil {
		return err
	}
	confProto, err := readConfigs(confFile)
	if err != nil {
		return err
	}

	for _, profile := range confProto.GetConfigs() {
		fmt.Fprintln(appCtx.App.Writer, profile.GetName())
	}

	return nil
}

func GetConfig(appCtx *cli.Context) error {
	confFile, err := getConfFileForContext(appCtx)
	if err != nil {
		return err
	}
	confProto, err := readConfigs(confFile)
	if err != nil {
		return err
	}

	confName := "DEFAULT"
	if appCtx.IsSet("context") {
		confName = appCtx.String("context")
	}

	for _, profile := range confProto.GetConfigs() {
		if profile.GetName() == confName {
			protoMessage, err := prototext.MarshalOptions{Multiline: true}.Marshal(profile)
			if err != nil {
				return err
			}
			fmt.Fprint(appCtx.App.Writer, string(protoMessage))
			return nil
		}
	}

	return fmt.Errorf("unable to find config %q in file %q.", confName, confFile)
}

func migrateDeprecatedAuthFields(conf *nbictlpb.Config) {
	if conf.GetAuthStrategy() != nil {
		return
	}
	if conf.GetEmail() == "" && conf.GetKeyId() == "" && conf.GetPrivKey() == "" {
		return
	}
	jwt := &nbictlpb.Config_AuthStrategy_Jwt{
		Email:        conf.GetEmail(),
		PrivateKeyId: conf.GetKeyId(),
	}
	if conf.GetPrivKey() != "" {
		jwt.SigningStrategy = &nbictlpb.Config_SigningStrategy{
			Type: &nbictlpb.Config_SigningStrategy_PrivateKeyFile{
				PrivateKeyFile: conf.GetPrivKey(),
			},
		}
	}
	conf.AuthStrategy = &nbictlpb.Config_AuthStrategy{
		Type: &nbictlpb.Config_AuthStrategy_Jwt_{Jwt: jwt},
	}
	conf.Email = ""
	conf.KeyId = ""
	conf.PrivKey = ""
}

// checkAuthStrategyIsComplete refuses a merged oidc_service_account strategy
// that is missing a field.
//
// The merge overwrites a field only when the new value is not empty, so a
// switch to this strategy from another one leaves every omitted field unset,
// and a switch back to it from another strategy would resurrect the key file
// of whichever service account the profile last named. nbictl would then sign
// an assertion with that key and post it to the newly named identity provider.
// Every field is reported at once, because an operator who fills them in one
// error at a time runs the command three times.
func checkAuthStrategyIsComplete(strategy *nbictlpb.Config_AuthStrategy) error {
	sa, ok := strategy.GetType().(*nbictlpb.Config_AuthStrategy_OidcServiceAccount_)
	if !ok {
		return nil
	}

	var missing []string
	if sa.OidcServiceAccount.GetIssuer() == "" {
		missing = append(missing, "--issuer")
	}
	if sa.OidcServiceAccount.GetProjectId() == "" {
		missing = append(missing, "--project_id")
	}
	if sa.OidcServiceAccount.GetServiceAccountKeyFile() == "" {
		missing = append(missing, "--service_account_key_file")
	}
	if len(missing) > 0 {
		return fmt.Errorf("the oidc_service_account strategy needs %s; give every flag when the profile does not already use this strategy", strings.Join(missing, ", "))
	}
	return nil
}

func mergeAuthStrategy(existing, incoming *nbictlpb.Config_AuthStrategy) *nbictlpb.Config_AuthStrategy {
	if _, ok := incoming.GetType().(*nbictlpb.Config_AuthStrategy_None); ok {
		return incoming
	}

	switch in := incoming.GetType().(type) {
	case *nbictlpb.Config_AuthStrategy_Jwt_:
		var base *nbictlpb.Config_AuthStrategy_Jwt
		if exJwt, ok := existing.GetType().(*nbictlpb.Config_AuthStrategy_Jwt_); ok {
			base = proto.Clone(exJwt.Jwt).(*nbictlpb.Config_AuthStrategy_Jwt)
		} else {
			base = &nbictlpb.Config_AuthStrategy_Jwt{}
		}

		if in.Jwt.GetEmail() != "" {
			base.Email = in.Jwt.GetEmail()
		}
		if in.Jwt.GetPrivateKeyId() != "" {
			base.PrivateKeyId = in.Jwt.GetPrivateKeyId()
		}
		if in.Jwt.GetSigningStrategy() != nil {
			base.SigningStrategy = in.Jwt.GetSigningStrategy()
		}

		return &nbictlpb.Config_AuthStrategy{
			Type: &nbictlpb.Config_AuthStrategy_Jwt_{Jwt: base},
		}

	case *nbictlpb.Config_AuthStrategy_OidcClientCredentials_:
		var base *nbictlpb.Config_AuthStrategy_OidcClientCredentials
		if exOIDC, ok := existing.GetType().(*nbictlpb.Config_AuthStrategy_OidcClientCredentials_); ok {
			base = proto.Clone(exOIDC.OidcClientCredentials).(*nbictlpb.Config_AuthStrategy_OidcClientCredentials)
		} else {
			base = &nbictlpb.Config_AuthStrategy_OidcClientCredentials{}
		}

		if in.OidcClientCredentials.GetClientId() != "" {
			base.ClientId = in.OidcClientCredentials.GetClientId()
		}
		if in.OidcClientCredentials.GetTokenUrl() != "" {
			base.TokenUrl = in.OidcClientCredentials.GetTokenUrl()
		}
		if in.OidcClientCredentials.GetPrivateKeyId() != "" {
			base.PrivateKeyId = in.OidcClientCredentials.GetPrivateKeyId()
		}
		if in.OidcClientCredentials.GetSigningStrategy() != nil {
			base.SigningStrategy = in.OidcClientCredentials.GetSigningStrategy()
		}

		return &nbictlpb.Config_AuthStrategy{
			Type: &nbictlpb.Config_AuthStrategy_OidcClientCredentials_{OidcClientCredentials: base},
		}

	case *nbictlpb.Config_AuthStrategy_OidcUser_:
		var base *nbictlpb.Config_AuthStrategy_OidcUser
		if exUser, ok := existing.GetType().(*nbictlpb.Config_AuthStrategy_OidcUser_); ok {
			base = proto.Clone(exUser.OidcUser).(*nbictlpb.Config_AuthStrategy_OidcUser)
		} else {
			base = &nbictlpb.Config_AuthStrategy_OidcUser{}
		}

		if in.OidcUser.GetIssuer() != "" {
			base.Issuer = in.OidcUser.GetIssuer()
		}
		if in.OidcUser.GetClientId() != "" {
			base.ClientId = in.OidcUser.GetClientId()
		}

		return &nbictlpb.Config_AuthStrategy{
			Type: &nbictlpb.Config_AuthStrategy_OidcUser_{OidcUser: base},
		}

	case *nbictlpb.Config_AuthStrategy_OidcServiceAccount_:
		var base *nbictlpb.Config_AuthStrategy_OidcServiceAccount
		if exSA, ok := existing.GetType().(*nbictlpb.Config_AuthStrategy_OidcServiceAccount_); ok {
			base = proto.Clone(exSA.OidcServiceAccount).(*nbictlpb.Config_AuthStrategy_OidcServiceAccount)
		} else {
			base = &nbictlpb.Config_AuthStrategy_OidcServiceAccount{}
		}

		if in.OidcServiceAccount.GetIssuer() != "" {
			base.Issuer = in.OidcServiceAccount.GetIssuer()
		}
		if in.OidcServiceAccount.GetProjectId() != "" {
			base.ProjectId = in.OidcServiceAccount.GetProjectId()
		}
		if in.OidcServiceAccount.GetServiceAccountKeyFile() != "" {
			base.ServiceAccountKeyFile = in.OidcServiceAccount.GetServiceAccountKeyFile()
		}

		return &nbictlpb.Config_AuthStrategy{
			Type: &nbictlpb.Config_AuthStrategy_OidcServiceAccount_{OidcServiceAccount: base},
		}

	default:
		return incoming
	}
}

func SetConfig(appCtx *cli.Context) error {
	confName := "DEFAULT"
	if appCtx.IsSet("context") {
		confName = appCtx.String("context")
	}
	privKey := appCtx.String("priv_key")
	keyID := appCtx.String("key_id")
	userID := appCtx.String("user_id")
	url := appCtx.String("url")
	transportSecurity := appCtx.String("transport_security")
	transport := appCtx.String("transport")
	authStrategy := appCtx.String("auth_strategy")
	endpointConfigStrategy := appCtx.String("endpoint_config")

	confPath, err := getConfFileForContext(appCtx)
	if err != nil {
		return err
	}

	var transportSecurityPB *nbictlpb.Config_TransportSecurity

	switch transportSecurity {
	case "insecure":
		transportSecurityPB = &nbictlpb.Config_TransportSecurity{
			Type: &nbictlpb.Config_TransportSecurity_Insecure{},
		}

	case "system_cert_pool":
		transportSecurityPB = &nbictlpb.Config_TransportSecurity{
			Type: &nbictlpb.Config_TransportSecurity_SystemCertPool{},
		}

	case "":
		transportSecurityPB = nil

	default:
		return fmt.Errorf("unexpected transport security selection: %s", transportSecurity)
	}

	var transportPB *nbictlpb.Config_Transport
	switch transport {
	case "quic":
		transportPB = &nbictlpb.Config_Transport{
			Type: &nbictlpb.Config_Transport_Quic{},
		}
	case "tcp":
		transportPB = &nbictlpb.Config_Transport{
			Type: &nbictlpb.Config_Transport_Tcp{},
		}
	case "":
		transportPB = nil
	default:
		return fmt.Errorf("unexpected transport selection: %s", transport)
	}

	var authStrategyPB *nbictlpb.Config_AuthStrategy
	switch authStrategy {
	case "none":
		authStrategyPB = &nbictlpb.Config_AuthStrategy{
			Type: &nbictlpb.Config_AuthStrategy_None{},
		}
	case "jwt":
		var signingStrategy *nbictlpb.Config_SigningStrategy
		if privKey != "" {
			signingStrategy = &nbictlpb.Config_SigningStrategy{
				Type: &nbictlpb.Config_SigningStrategy_PrivateKeyFile{
					PrivateKeyFile: privKey,
				},
			}
		}
		authStrategyPB = &nbictlpb.Config_AuthStrategy{
			Type: &nbictlpb.Config_AuthStrategy_Jwt_{
				Jwt: &nbictlpb.Config_AuthStrategy_Jwt{
					Email:           userID,
					PrivateKeyId:    keyID,
					SigningStrategy: signingStrategy,
				},
			},
		}
	case "":
		if cmp.Or(keyID, userID, privKey) != "" {
			jwt := &nbictlpb.Config_AuthStrategy_Jwt{
				Email:        userID,
				PrivateKeyId: keyID,
			}
			if privKey != "" {
				jwt.SigningStrategy = &nbictlpb.Config_SigningStrategy{
					Type: &nbictlpb.Config_SigningStrategy_PrivateKeyFile{
						PrivateKeyFile: privKey,
					},
				}
			}
			authStrategyPB = &nbictlpb.Config_AuthStrategy{
				Type: &nbictlpb.Config_AuthStrategy_Jwt_{Jwt: jwt},
			}
		}
	case "oidc":
		var signingStrategy *nbictlpb.Config_SigningStrategy
		if privKey != "" {
			signingStrategy = &nbictlpb.Config_SigningStrategy{
				Type: &nbictlpb.Config_SigningStrategy_PrivateKeyFile{
					PrivateKeyFile: privKey,
				},
			}
		}
		authStrategyPB = &nbictlpb.Config_AuthStrategy{
			Type: &nbictlpb.Config_AuthStrategy_OidcClientCredentials_{
				OidcClientCredentials: &nbictlpb.Config_AuthStrategy_OidcClientCredentials{
					ClientId:        appCtx.String("client_id"),
					TokenUrl:        appCtx.String("token_url"),
					PrivateKeyId:    keyID,
					SigningStrategy: signingStrategy,
				},
			},
		}
	case "oidc_user":
		authStrategyPB = &nbictlpb.Config_AuthStrategy{
			Type: &nbictlpb.Config_AuthStrategy_OidcUser_{
				OidcUser: &nbictlpb.Config_AuthStrategy_OidcUser{
					Issuer:   appCtx.String("issuer"),
					ClientId: appCtx.String("client_id"),
				},
			},
		}
	case "oidc_service_account":
		authStrategyPB = &nbictlpb.Config_AuthStrategy{
			Type: &nbictlpb.Config_AuthStrategy_OidcServiceAccount_{
				OidcServiceAccount: &nbictlpb.Config_AuthStrategy_OidcServiceAccount{
					Issuer:                appCtx.String("issuer"),
					ProjectId:             appCtx.String("project_id"),
					ServiceAccountKeyFile: appCtx.String("service_account_key_file"),
				},
			},
		}
	default:
		return fmt.Errorf("unexpected auth strategy: %s (allowed: none, jwt, oidc, oidc_user, oidc_service_account)", authStrategy)
	}

	var endpointConfigPB *nbictlpb.Config_EndpointConfig
	switch endpointConfigStrategy {
	case "subdomain":
		endpointConfigPB = &nbictlpb.Config_EndpointConfig{
			Strategy: &nbictlpb.Config_EndpointConfig_Subdomain{},
		}
	case "single_domain":
		se := &nbictlpb.Config_ServiceEndpoint{}
		if u := appCtx.String("url"); u != "" {
			se.Url = u
		}
		endpointConfigPB = &nbictlpb.Config_EndpointConfig{
			Strategy: &nbictlpb.Config_EndpointConfig_SingleDomain{
				SingleDomain: se,
			},
		}
	case "custom":
		custom := &nbictlpb.Config_CustomEndpoints{}
		if u := appCtx.String("model_url"); u != "" {
			custom.Model = &nbictlpb.Config_ServiceEndpoint{Url: u}
		}
		if u := appCtx.String("status_url"); u != "" {
			custom.Status = &nbictlpb.Config_ServiceEndpoint{Url: u}
		}
		if u := appCtx.String("provisioning_url"); u != "" {
			custom.Provisioning = &nbictlpb.Config_ServiceEndpoint{Url: u}
		}
		if u := appCtx.String("default_url"); u != "" {
			custom.Default = &nbictlpb.Config_ServiceEndpoint{Url: u}
		}
		endpointConfigPB = &nbictlpb.Config_EndpointConfig{
			Strategy: &nbictlpb.Config_EndpointConfig_Custom{
				Custom: custom,
			},
		}
	case "":
		endpointConfigPB = nil
	default:
		return fmt.Errorf("unexpected endpoint_config strategy: %s (allowed: subdomain, single_domain, custom)", endpointConfigStrategy)
	}

	contextToCreate := &nbictlpb.Config{
		Name:              confName,
		Url:               url,
		TransportSecurity: transportSecurityPB,
		Transport:         transportPB,
		AuthStrategy:      authStrategyPB,
		EndpointConfig:    endpointConfigPB,
	}

	return setConfig(appCtx.App.Writer, appCtx.App.ErrWriter, contextToCreate, confPath)
}

func setConfig(outWriter, errWriter io.Writer, confToCreate *nbictlpb.Config, confFile string) error {
	if confToCreate.GetName() == "" {
		return errors.New("missing required --context flag")
	}

	migrateDeprecatedAuthFields(confToCreate)
	// Whether this command names an auth strategy at all, recorded before the
	// merge below replaces confToCreate with the stored profile. A command that
	// names none must not be judged on the strategy the profile already holds.
	authStrategyGiven := confToCreate.GetAuthStrategy() != nil

	confProto, err := readConfigs(confFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			confProto = &nbictlpb.AppConfig{}
		} else {
			return fmt.Errorf("unable to get configs from file %s: %w", confFile, err)
		}
	}

	found := false
	for _, confProto := range confProto.GetConfigs() {
		if confProto.GetName() != confToCreate.GetName() {
			continue
		}
		migrateDeprecatedAuthFields(confProto)
		if confToCreate.GetUrl() != "" {
			confProto.Url = confToCreate.GetUrl()
		}
		if confToCreate.GetTransportSecurity() != nil {
			confProto.TransportSecurity = confToCreate.GetTransportSecurity()
		}
		if confToCreate.GetTransport() != nil {
			confProto.Transport = confToCreate.GetTransport()
		}
		if confToCreate.GetAuthStrategy() != nil {
			if confProto.GetAuthStrategy() != nil {
				confProto.AuthStrategy = mergeAuthStrategy(confProto.AuthStrategy, confToCreate.AuthStrategy)
			} else {
				confProto.AuthStrategy = confToCreate.AuthStrategy
			}
		}
		if confToCreate.GetEndpointConfig() != nil {
			confProto.EndpointConfig = confToCreate.GetEndpointConfig()
		}
		found = true
		confToCreate = confProto
		break
	}

	if authStrategyGiven {
		if err := checkAuthStrategyIsComplete(confToCreate.GetAuthStrategy()); err != nil {
			return err
		}
	}

	if !found {
		confProto.Configs = append(confProto.Configs, confToCreate)
	}

	nbiConfigTextProto, err := prototext.MarshalOptions{Multiline: true}.Marshal(confProto)
	if err != nil {
		return fmt.Errorf("unable to convert proto into textproto format: %w", err)
	}

	// The token store lives inside this directory, so neither the directory
	// nor the config file may be readable by other local users.
	contextDir := filepath.Dir(confFile)
	if err = os.MkdirAll(contextDir, 0o700); err != nil {
		return fmt.Errorf("unable to create directory: %w", err)
	}
	// MkdirAll leaves an existing directory's mode alone, so a directory made
	// by an earlier version keeps its wider mode without this.
	if err = os.Chmod(contextDir, 0o700); err != nil {
		return fmt.Errorf("unable to restrict the permissions of the configuration directory: %w", err)
	}

	if err = os.WriteFile(confFile, nbiConfigTextProto, 0o600); err != nil {
		return fmt.Errorf("unable to update the configuration information: %w", err)
	}
	// os.WriteFile applies the mode only when it creates the file, so a config
	// written by an earlier version keeps its old, wider mode without this.
	if err = os.Chmod(confFile, 0o600); err != nil {
		return fmt.Errorf("unable to restrict the permissions of the configuration file: %w", err)
	}

	protoMessage, err := prototext.MarshalOptions{Multiline: true}.Marshal(confToCreate)
	if err != nil {
		return fmt.Errorf("unable to convert the nbictl context into textproto format: %w", err)
	}
	fmt.Fprintf(errWriter, "configuration successfully updated; the configuration file is stored under: %s\n", confFile)
	fmt.Fprint(outWriter, string(protoMessage))
	return nil
}

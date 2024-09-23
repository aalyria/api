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
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"aalyria.com/spacetime/github/tools/nbictl/nbictlpb"
	"github.com/urfave/cli/v2"
	"google.golang.org/protobuf/encoding/prototext"
)

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

func readConfig(context, confFilePath string) (*nbictlpb.Config, error) {
	confs, err := readConfigs(confFilePath)
	if err != nil {
		return nil, fmt.Errorf("unable to get config contexts: %w", err)
	}

	// if the context name is not specified and there is only one context in the config file
	// the function will return that configuration context
	if context == "" {
		switch {
		case len(confs.GetConfigs()) == 1:
			return confs.GetConfigs()[0], nil
		default:
			return nil, errors.New("--context flag required because there are multiple contexts defined in the configuration.")
		}
	}

	var confNames []string
	for _, conf := range confs.GetConfigs() {
		if conf.GetName() == context {
			return conf, nil
		}
		confNames = append(confNames, conf.GetName())
	}
	return nil, fmt.Errorf("unable to get the context with the name: %q (expected one of [%s])", context, strings.Join(confNames, ", "))
}

func readConfigs(confFilePath string) (*nbictlpb.AppConfig, error) {
	confProto := &nbictlpb.AppConfig{}
	confBytes, err := os.ReadFile(confFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return confProto, nil
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

	confPath, err := getConfFileForContext(appCtx)
	if err != nil {
		return err
	}

	var transportSecurityPb *nbictlpb.Config_TransportSecurity

	switch transportSecurity {
	case "insecure":
		transportSecurityPb = &nbictlpb.Config_TransportSecurity{
			Type: &nbictlpb.Config_TransportSecurity_Insecure{},
		}

	case "system_cert_pool":
		transportSecurityPb = &nbictlpb.Config_TransportSecurity{
			Type: &nbictlpb.Config_TransportSecurity_SystemCertPool{},
		}

	case "":
		transportSecurityPb = nil

	default:
		return fmt.Errorf("unexpected transport security selection: %s", transportSecurity)
	}

	contextToCreate := &nbictlpb.Config{
		Name:              confName,
		KeyId:             keyID,
		Email:             userID,
		PrivKey:           privKey,
		Url:               url,
		TransportSecurity: transportSecurityPb,
	}

	return setConfig(appCtx.App.Writer, appCtx.App.ErrWriter, contextToCreate, confPath)
}

func setConfig(outWriter, errWriter io.Writer, confToCreate *nbictlpb.Config, confFile string) error {
	if confToCreate.GetName() == "" {
		return errors.New("missing required --context flag")
	}

	confProto, err := readConfigs(confFile)
	if err != nil {
		return fmt.Errorf("unable to get configs from file %s: %w", confFile, err)
	}

	found := false
	for _, confProto := range confProto.GetConfigs() {
		if confProto.GetName() != confToCreate.GetName() {
			continue
		}
		if confToCreate.GetEmail() != "" {
			confProto.Email = confToCreate.GetEmail()
		}
		if confToCreate.GetPrivKey() != "" {
			confProto.PrivKey = confToCreate.GetPrivKey()
		}
		if confToCreate.GetKeyId() != "" {
			confProto.KeyId = confToCreate.GetKeyId()
		}
		if confToCreate.GetUrl() != "" {
			confProto.Url = confToCreate.GetUrl()
		}
		if confToCreate.GetTransportSecurity() != nil {
			confProto.TransportSecurity = confToCreate.GetTransportSecurity()
		}
		found = true
		confToCreate = confProto
		break
	}

	if !found {
		confProto.Configs = append(confProto.Configs, confToCreate)
	}

	nbiConfigTextProto, err := prototext.Marshal(confProto)
	if err != nil {
		return fmt.Errorf("unable to convert proto into textproto format: %w", err)
	}

	contextDir := filepath.Dir(confFile)
	if err = os.MkdirAll(contextDir, 0o777); err != nil {
		return fmt.Errorf("unable to create directory: %w", err)
	}

	if err = os.WriteFile(confFile, nbiConfigTextProto, 0o777); err != nil {
		return fmt.Errorf("unable to update the configuration information: %w", err)
	}

	protoMessage, err := prototext.MarshalOptions{Multiline: true}.Marshal(confToCreate)
	if err != nil {
		return fmt.Errorf("unable to convert the nbictl context into textproto format: %w", err)
	}
	fmt.Fprintf(errWriter, "configuration successfully updated; the configuration file is stored under: %s\n", confFile)
	fmt.Fprint(outWriter, string(protoMessage))
	return nil
}

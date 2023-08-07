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
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	pb "aalyria.com/spacetime/github/tools/nbictl/resource"
	"google.golang.org/protobuf/encoding/prototext"
)

const confFileName = "config.textproto"

func GetContext(context, contextDir string) (*pb.Context, error) {
	configContexts, err := getContexts(contextDir)
	if err != nil {
		return nil, fmt.Errorf("unable to get config contexts: %w", err)
	}

	// if the context name is not specified and there is only one context in the config file
	// the function will return that configuration context
	if context == "" {
		switch {
		case len(configContexts.GetContexts()) == 1:
			return configContexts.GetContexts()[0], nil
		default:
			return nil, errors.New("--context flag required because there are multiple contexts defined in the configuration.")
		}
	}

	var contextNames []string
	for _, ctx := range configContexts.GetContexts() {
		if ctx.GetName() == context {
			return ctx, nil
		}
		contextNames = append(contextNames, ctx.GetName())
	}
	return nil, fmt.Errorf("unable to get the context with the name: %s. the list of available context names are the following: %v", context, contextNames)
}

func getContexts(contextFilePath string) (*pb.NbiCtlConfig, error) {
	configBytes, err := os.ReadFile(contextFilePath)
	configProto := &pb.NbiCtlConfig{}
	if err != nil {
		if os.IsNotExist(err) {
			return configProto, nil
		}
		return nil, fmt.Errorf("unable to read file: %w", err)
	}

	if err := prototext.Unmarshal(configBytes, configProto); err != nil {
		return nil, fmt.Errorf("invalid file content: %w", err)
	}
	return configProto, nil
}

func SetContext(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet(clientName+" set-context", flag.ExitOnError)
	contextName := fs.String("context", "DEFAULT", "context of NBI API environment")
	privKey := fs.String("priv_key", "", "path to your private key for authentication to NBI API")
	keyID := fs.String("key_id", "", "key id associated with the provate key provided by Aalyria")
	userID := fs.String("user_id", "", "user id address associated with the private key provided by Aalyria")
	url := fs.String("url", "", "url of NBI endpoint")
	transportSecurity := fs.String("transport_security", "", "transport security to use when connecting to NBI. Values: insecure, system_cert_pool")
	fs.Parse(args)

	configDir, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("unable to obtain the default root directory: %w", err)
	}
	filePath := filepath.Join(configDir, clientName, confFileName)

	var transportSecurityPb *pb.Context_TransportSecurity

	switch *transportSecurity {
	case "insecure":
		transportSecurityPb = &pb.Context_TransportSecurity{
			Type: &pb.Context_TransportSecurity_Insecure{},
		}

	case "system_cert_pool":
		transportSecurityPb = &pb.Context_TransportSecurity{
			Type: &pb.Context_TransportSecurity_SystemCertPool{},
		}

	case "":
		transportSecurityPb = nil

	default:
		return fmt.Errorf("unexpected transport security selection: %s", *transportSecurity)
	}

	contextToCreate := &pb.Context{
		Name:              *contextName,
		KeyId:             *keyID,
		Email:             *userID,
		PrivKey:           *privKey,
		Url:               *url,
		TransportSecurity: transportSecurityPb,
	}

	return setContext(contextToCreate, filePath)
}

func setContext(contextToCreate *pb.Context, contextFile string) error {
	if contextToCreate.GetName() == "" {
		return errors.New("--context required")
	}

	configProto, err := getContexts(contextFile)
	if err != nil {
		return fmt.Errorf("unable to get contexts: %w", err)
	}

	found := false
	for _, confProto := range configProto.GetContexts() {
		if confProto.GetName() != contextToCreate.GetName() {
			continue
		}
		if contextToCreate.GetEmail() != "" {
			confProto.Email = contextToCreate.GetEmail()
		}
		if contextToCreate.GetPrivKey() != "" {
			confProto.PrivKey = contextToCreate.GetPrivKey()
		}
		if contextToCreate.GetKeyId() != "" {
			confProto.KeyId = contextToCreate.GetKeyId()
		}
		if contextToCreate.GetUrl() != "" {
			confProto.Url = contextToCreate.GetUrl()
		}
		if contextToCreate.GetTransportSecurity() != nil {
			confProto.TransportSecurity = contextToCreate.GetTransportSecurity()
		}
		found = true
		contextToCreate = confProto
		break
	}

	if !found {
		configProto.Contexts = append(configProto.Contexts, contextToCreate)
	}

	nbiConfigTextProto, err := prototext.Marshal(configProto)
	if err != nil {
		return fmt.Errorf("unable to convert proto into textproto format: %w", err)
	}

	contextDir := filepath.Dir(contextFile)
	if err = os.MkdirAll(contextDir, 0o777); err != nil {
		return fmt.Errorf("unable to create directory: %w", err)
	}

	if err = os.WriteFile(contextFile, nbiConfigTextProto, 0o777); err != nil {
		return fmt.Errorf("unable to update the configuration information: %w", err)
	}

	protoMessage, err := prototext.MarshalOptions{Multiline: true}.Marshal(contextToCreate)
	if err != nil {
		return fmt.Errorf("unable to convert the nbictl context into textproto format: %w", err)
	}
	fmt.Printf("context successfully set. the configuration file is stored under: %s\n", contextFile)
	fmt.Printf("context: %v\n", string(protoMessage))
	return nil
}

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
	"fmt"
	"io"
	"os"
	"sort"

	fedpb "aalyria.com/spacetime/api/federation/interconnect/v1alpha"
	modelpb "aalyria.com/spacetime/api/model/v1"
	nbipb "aalyria.com/spacetime/api/nbi/v1alpha"
	provpb "aalyria.com/spacetime/api/provisioning/v1alpha"
	schedpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	telemetrypb "aalyria.com/spacetime/api/telemetry/v1alpha"
	"github.com/fullstorydev/grpcurl"
	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/grpcreflect"
	"github.com/urfave/cli/v2"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

func GRPCDescribe(appCtx *cli.Context) error {
	switch args := appCtx.Args(); args.Len() {
	case 0:
		return GRPCDescribeServices(appCtx)
	case 1:
		return GRPCDescribeSymbol(appCtx, args.First())
	default:
		return fmt.Errorf("expected 0 or 1 arguments, got %d", args.Len())
	}
}

func GRPCDescribeServices(appCtx *cli.Context) error {
	conn, err := openConnection(appCtx)
	if err != nil {
		return err
	}
	defer conn.Close()
	refClient := grpcreflect.NewClientAuto(appCtx.Context, conn)

	svcs, err := refClient.ListServices()
	if err != nil {
		return err
	}

	for _, svc := range svcs {
		if err := describeSymbol(appCtx, refClient, svc); err != nil {
			return err
		}
	}
	return nil
}

func GRPCDescribeSymbol(appCtx *cli.Context, symbol string) error {
	conn, err := openConnection(appCtx)
	if err != nil {
		return err
	}
	defer conn.Close()
	refClient := grpcreflect.NewClientAuto(appCtx.Context, conn)
	return describeSymbol(appCtx, refClient, symbol)
}

func describeSymbol(appCtx *cli.Context, refClient *grpcreflect.Client, symbol string) error {
	f, err := refClient.FileContainingSymbol(symbol)
	if err != nil {
		return err
	}
	dsc := f.FindSymbol(symbol)
	if dsc == nil {
		return fmt.Errorf("couldn't find symbol %q", symbol)
	}

	elementType := ""
	switch d := dsc.(type) {
	case *desc.MessageDescriptor:
		elementType = "a message"
		if parent, ok := d.GetParent().(*desc.MessageDescriptor); ok {
			if d.IsMapEntry() {
				for _, f := range parent.GetFields() {
					if f.IsMap() && f.GetMessageType() == d {
						// found it: describe the map field instead
						elementType = "the entry type for a map field"
						dsc = f
						break
					}
				}
			} else {
				// see if it's a group
				for _, f := range parent.GetFields() {
					if f.GetType() == descriptorpb.FieldDescriptorProto_TYPE_GROUP && f.GetMessageType() == d {
						// found it: describe the group field instead
						elementType = "the type of a group field"
						dsc = f
						break
					}
				}
			}
		}
	case *desc.FieldDescriptor:
		switch {
		case d.GetType() == descriptorpb.FieldDescriptorProto_TYPE_GROUP:
			elementType = "a group field"
		case d.IsExtension():
			elementType = "an extension"
		default:
			elementType = "a field"
		}

	case *desc.OneOfDescriptor:
		elementType = "a one-of"
	case *desc.EnumDescriptor:
		elementType = "an enum"
	case *desc.EnumValueDescriptor:
		elementType = "an enum value"
	case *desc.ServiceDescriptor:
		elementType = "a service"
	case *desc.MethodDescriptor:
		elementType = "a method"
	default:
		return fmt.Errorf("descriptor has unrecognized type %T", dsc)
	}

	fmt.Fprintf(appCtx.App.Writer, "%s is %s\n", dsc.GetFullyQualifiedName(), elementType)
	return nil
}

func GRPCList(appCtx *cli.Context) error {
	switch args := appCtx.Args(); args.Len() {
	case 0:
		return GRPCListServices(appCtx)
	case 1:
		return GRPCListMethods(appCtx, args.First())
	default:
		return fmt.Errorf("expected 0 or 1 arguments, got %d", args.Len())
	}
}

func GRPCListServices(appCtx *cli.Context) error {
	conn, err := openConnection(appCtx)
	if err != nil {
		return err
	}
	defer conn.Close()
	refClient := grpcreflect.NewClientAuto(appCtx.Context, conn)

	svcs, err := refClient.ListServices()
	if err != nil {
		return err
	}

	for _, svc := range svcs {
		fmt.Fprintln(appCtx.App.Writer, svc)
	}
	return nil
}

func GRPCListMethods(appCtx *cli.Context, svc string) error {
	conn, err := openConnection(appCtx)
	if err != nil {
		return err
	}
	defer conn.Close()
	refClient := grpcreflect.NewClientAuto(appCtx.Context, conn)

	serviceDesc, err := refClient.ResolveService(svc)
	if err != nil {
		return fmt.Errorf("resolving service %q: %w", svc, err)
	}

	methods := make([]string, 0, len(serviceDesc.GetMethods()))
	for _, meth := range serviceDesc.GetMethods() {
		methods = append(methods, meth.GetFullyQualifiedName())
	}
	sort.Strings(methods)
	for _, meth := range methods {
		fmt.Fprintln(appCtx.App.Writer, meth)
	}
	return nil
}

func GRPCCall(appCtx *cli.Context) error {
	if ln := appCtx.Args().Len(); ln != 1 {
		return fmt.Errorf("call expects exactly 1 argument, got %d", ln)
	}
	method := appCtx.Args().First()

	var r io.Reader
	switch reqFile := appCtx.String("request"); reqFile {
	case "-", "":
		r = appCtx.App.Reader
	default:
		rdr, err := os.Open(reqFile)
		if err != nil {
			return err
		}
		defer rdr.Close()
		r = rdr
	}

	conn, err := openConnection(appCtx)
	if err != nil {
		return err
	}
	defer conn.Close()

	svcDescriptors, err := desc.WrapFiles([]protoreflect.FileDescriptor{
		fedpb.File_api_federation_interconnect_v1alpha_interconnect_proto,
		modelpb.File_api_model_v1_model_proto,
		nbipb.File_api_nbi_v1alpha_nbi_proto,
		nbipb.File_api_nbi_v1alpha_signal_propagation_proto,
		provpb.File_api_provisioning_v1alpha_provisioning_proto,
		schedpb.File_api_scheduling_v1alpha_scheduling_proto,
		telemetrypb.File_api_telemetry_v1alpha_telemetry_proto,
	})
	if err != nil {
		return err
	}

	descSrc, err := grpcurl.DescriptorSourceFromFileDescriptors(svcDescriptors...)
	if err != nil {
		return err
	}

	var format grpcurl.Format
	if appCtx.IsSet("format") {
		format = grpcurl.Format(appCtx.String("format"))
	} else {
		format = grpcurl.Format("json")
	}

	reqParser, formatter, err := grpcurl.RequestParserAndFormatter(format, descSrc, r, grpcurl.FormatOptions{
		IncludeTextSeparator: true,
	})
	if err != nil {
		return err
	}

	return grpcurl.InvokeRPC(
		appCtx.Context,
		descSrc,
		conn,
		method,
		[]string{},
		&grpcurl.DefaultEventHandler{
			Out:            appCtx.App.Writer,
			Formatter:      formatter,
			VerbosityLevel: 0,
		},
		reqParser.Next,
	)
}

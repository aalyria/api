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
	"slices"
	"strings"

	"github.com/fullstorydev/grpcurl"
	"github.com/jhump/protoreflect/v2/grpcreflect"
	"github.com/jhump/protoreflect/v2/protoresolve"
	"github.com/urfave/cli/v2"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	auditpb "aalyria.com/spacetime/api/audit/v1alpha"
	fedpb "aalyria.com/spacetime/api/federation/interconnect/v1alpha"
	modelpb "aalyria.com/spacetime/api/model/v1"
	permissionspb "aalyria.com/spacetime/api/permissions/v1alpha"
	provpb "aalyria.com/spacetime/api/provisioning/v1alpha"
	schedpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	solutionpb "aalyria.com/spacetime/api/solution/v1alpha"
	statuspb "aalyria.com/spacetime/api/status/v1"
	telemetrypb "aalyria.com/spacetime/api/telemetry/v1alpha"
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
	conn, err := openAPIConnection(appCtx, serviceKey(appCtx.String("api")))
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
		if err := describeSymbol(appCtx, refClient, string(svc)); err != nil {
			return err
		}
	}
	return nil
}

func GRPCDescribeSymbol(appCtx *cli.Context, symbol string) error {
	conn, err := openAPIConnection(appCtx, serviceKey(appCtx.String("api")))
	if err != nil {
		return err
	}
	defer conn.Close()
	refClient := grpcreflect.NewClientAuto(appCtx.Context, conn)
	return describeSymbol(appCtx, refClient, symbol)
}

// resolveSymbol asks the server for the file that declares symbol and returns
// the symbol's descriptor.
func resolveSymbol(refClient *grpcreflect.Client, symbol string) (protoreflect.Descriptor, error) {
	name := protoreflect.FullName(strings.TrimPrefix(symbol, "."))
	f, err := refClient.FileContainingSymbol(name)
	if err != nil {
		return nil, err
	}
	return findSymbol(f, name), nil
}

// findSymbol returns the descriptor named symbol in file f, or in a file that f
// imports publicly, or nil if there is none.
func findSymbol(f protoreflect.FileDescriptor, symbol protoreflect.FullName) protoreflect.Descriptor {
	if d := protoresolve.FindDescriptorByNameInFile(f, symbol); d != nil {
		return d
	}
	imports := f.Imports()
	for i := range imports.Len() {
		if imp := imports.Get(i); imp.IsPublic {
			if d := findSymbol(imp.FileDescriptor, symbol); d != nil {
				return d
			}
		}
	}
	return nil
}

func describeSymbol(appCtx *cli.Context, refClient *grpcreflect.Client, symbol string) error {
	dsc, err := resolveSymbol(refClient, symbol)
	if err != nil {
		return err
	}
	if dsc == nil {
		return fmt.Errorf("couldn't find symbol %q", symbol)
	}

	elementType := ""
	switch d := dsc.(type) {
	case protoreflect.MessageDescriptor:
		elementType = "a message"
		if parent, ok := d.Parent().(protoreflect.MessageDescriptor); ok {
			if d.IsMapEntry() {
				if f := fieldOfType(parent, d, protoreflect.FieldDescriptor.IsMap); f != nil {
					// found it: describe the map field instead
					elementType = "the entry type for a map field"
					dsc = f
				}
			} else {
				// see if it's a group
				if f := fieldOfType(parent, d, isGroup); f != nil {
					// found it: describe the group field instead
					elementType = "the type of a group field"
					dsc = f
				}
			}
		}
	case protoreflect.FieldDescriptor:
		switch {
		case isGroup(d):
			elementType = "a group field"
		case d.IsExtension():
			elementType = "an extension"
		default:
			elementType = "a field"
		}

	case protoreflect.OneofDescriptor:
		elementType = "a one-of"
	case protoreflect.EnumDescriptor:
		elementType = "an enum"
	case protoreflect.EnumValueDescriptor:
		elementType = "an enum value"
	case protoreflect.ServiceDescriptor:
		elementType = "a service"
	case protoreflect.MethodDescriptor:
		elementType = "a method"
	default:
		return fmt.Errorf("descriptor has unrecognized type %T", dsc)
	}

	fmt.Fprintf(appCtx.App.Writer, "%s is %s\n", dsc.FullName(), elementType)
	return nil
}

// fieldOfType returns the first field of parent whose type is msg and that
// matches, or nil if there is none.
func fieldOfType(parent, msg protoreflect.MessageDescriptor, matches func(protoreflect.FieldDescriptor) bool) protoreflect.FieldDescriptor {
	fields := parent.Fields()
	for i := range fields.Len() {
		if f := fields.Get(i); matches(f) && f.Message() == msg {
			return f
		}
	}
	return nil
}

func isGroup(f protoreflect.FieldDescriptor) bool {
	return f.Kind() == protoreflect.GroupKind
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
	conn, err := openAPIConnection(appCtx, serviceKey(appCtx.String("api")))
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
	conn, err := openAPIConnection(appCtx, serviceKey(appCtx.String("api")))
	if err != nil {
		return err
	}
	defer conn.Close()
	refClient := grpcreflect.NewClientAuto(appCtx.Context, conn)

	dsc, err := resolveSymbol(refClient, svc)
	if err != nil {
		return fmt.Errorf("resolving service %q: %w", svc, err)
	}
	serviceDesc, ok := dsc.(protoreflect.ServiceDescriptor)
	if !ok {
		return fmt.Errorf("resolving service %q: Service not found: %s", svc, svc)
	}

	methodDescs := serviceDesc.Methods()
	methods := make([]string, 0, methodDescs.Len())
	for i := range methodDescs.Len() {
		methods = append(methods, string(methodDescs.Get(i).FullName()))
	}
	slices.Sort(methods)
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

	conn, err := openAPIConnection(appCtx, serviceKey(appCtx.String("api")))
	if err != nil {
		return err
	}
	defer conn.Close()

	descSrc, err := grpcurl.DescriptorSourceFromFileDescriptorSet(fileDescriptorSet(
		fedpb.File_api_federation_interconnect_v1alpha_interconnect_proto,
		modelpb.File_api_model_v1_model_proto,
		permissionspb.File_api_permissions_v1alpha_permissions_proto,
		provpb.File_api_provisioning_v1alpha_provisioning_proto,
		schedpb.File_api_scheduling_v1alpha_scheduling_proto,
		solutionpb.File_api_solution_v1alpha_solution_proto,
		telemetrypb.File_api_telemetry_v1alpha_telemetry_proto,
		statuspb.File_api_status_v1_status_proto,
		auditpb.File_api_audit_v1alpha_audit_logs_proto,
	))
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

	handler := &grpcurl.DefaultEventHandler{
		Out:            appCtx.App.Writer,
		Formatter:      formatter,
		VerbosityLevel: 0,
	}
	if err := grpcurl.InvokeRPC(
		appCtx.Context,
		descSrc,
		conn,
		method,
		[]string{},
		handler,
		reqParser.Next,
	); err != nil {
		return err
	}
	if err := handler.Status.Err(); err != nil {
		return fmt.Errorf("rpc failed: %w", err)
	}
	return nil
}

// fileDescriptorSet returns files and everything they import, transitively.
func fileDescriptorSet(files ...protoreflect.FileDescriptor) *descriptorpb.FileDescriptorSet {
	set := &descriptorpb.FileDescriptorSet{}
	seen := map[string]bool{}
	var add func(protoreflect.FileDescriptor)
	add = func(f protoreflect.FileDescriptor) {
		if seen[f.Path()] {
			return
		}
		seen[f.Path()] = true
		imports := f.Imports()
		for i := range imports.Len() {
			add(imports.Get(i).FileDescriptor)
		}
		set.File = append(set.File, protodesc.ToFileDescriptorProto(f))
	}
	for _, f := range files {
		add(f)
	}
	return set
}

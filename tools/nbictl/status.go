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
	"fmt"

	"github.com/urfave/cli/v2"

	statuspb "aalyria.com/spacetime/api/status/v1"
)

const statusAPISubDomain = "status-v1"

func StatusGetVersion(appCtx *cli.Context) error {
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	conn, err := openAPIConnection(appCtx, statusAPISubDomain)
	if err != nil {
		return err
	}
	defer conn.Close()
	statusClient := statuspb.NewStatusServiceClient(conn)

	response, err := statusClient.GetVersion(
		appCtx.Context, &statuspb.GetVersionRequest{})
	if err != nil {
		return err
	}

	marshalled, err := marshaller.marshal(response)
	if err != nil {
		return err
	}
	fmt.Fprint(appCtx.App.Writer, string(marshalled))
	return nil
}

func StatusGetMetrics(appCtx *cli.Context) error {
	marshaller, err := marshallerForFormat(appCtx.String("format"))
	if err != nil {
		return err
	}

	conn, err := openAPIConnection(appCtx, statusAPISubDomain)
	if err != nil {
		return err
	}
	defer conn.Close()

	statusClient := statuspb.NewStatusServiceClient(conn)

	response, err := statusClient.GetMetrics(
		appCtx.Context, &statuspb.GetMetricsRequest{})
	if err != nil {
		return err
	}

	marshalled, err := marshaller.marshal(response)
	if err != nil {
		return err
	}
	fmt.Fprint(appCtx.App.Writer, string(marshalled))
	return nil
}

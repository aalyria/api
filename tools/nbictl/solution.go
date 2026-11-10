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
	"context"
	"fmt"
	"time"

	"github.com/urfave/cli/v2"
	intervalpb "google.golang.org/genproto/googleapis/type/interval"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	solutionpb "aalyria.com/spacetime/api/solution/v1alpha"
)

// solutionQueryFlags are the filters shared by every solution query
// subcommand.
var solutionQueryFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "timestamp",
		Usage: "Only match solutions that exist at this RFC3339 timestamp (e.g. 2026-08-05T12:00:00Z).",
	},
	&cli.StringFlag{
		Name:  "interval-start",
		Usage: "Only match solutions that exist at or after this RFC3339 timestamp (e.g. 2026-08-05T12:00:00Z), inclusive: solutions overlapping the timestamp are included. May be combined with --interval-end.",
	},
	&cli.StringFlag{
		Name:  "interval-end",
		Usage: "Only match solutions that exist before this RFC3339 timestamp (e.g. 2026-08-05T13:00:00Z), exclusive: solutions that only begin at the timestamp are not included. May be combined with --interval-start.",
	},
}

func timestampFromFlags(appCtx *cli.Context) (*timestamppb.Timestamp, error) {
	ts := appCtx.String("timestamp")
	if ts == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return nil, fmt.Errorf("parsing --timestamp %q as RFC3339: %w", ts, err)
	}
	return timestamppb.New(t), nil
}

// intervalFromFlags builds a query interval from the --interval-start and
// --interval-end flags. It returns nil if neither flag is set; either bound
// may be omitted for a half-open interval.
func intervalFromFlags(appCtx *cli.Context) (*intervalpb.Interval, error) {
	start, end := appCtx.String("interval-start"), appCtx.String("interval-end")
	interval := &intervalpb.Interval{}
	if start != "" {
		t, err := time.Parse(time.RFC3339, start)
		if err != nil {
			return nil, fmt.Errorf("parsing --interval-start %q as RFC3339: %w", start, err)
		}
		interval.StartTime = timestamppb.New(t)
	}
	if end != "" {
		t, err := time.Parse(time.RFC3339, end)
		if err != nil {
			return nil, fmt.Errorf("parsing --interval-end %q as RFC3339: %w", end, err)
		}
		interval.EndTime = timestamppb.New(t)
	}
	return interval, nil
}

// runSolutionRPC opens a Solution API client, invokes call, and marshals its
// response to the app writer, centralizing the connection lifecycle shared by
// every solution subcommand.
func runSolutionRPC[R proto.Message](appCtx *cli.Context, call func(context.Context, solutionpb.SolutionClient) (R, error)) error {
	conn, err := openAPIConnection(appCtx, serviceSolution)
	if err != nil {
		return err
	}
	defer conn.Close()

	resp, err := call(appCtx.Context, solutionpb.NewSolutionClient(conn))
	if err != nil {
		return err
	}
	return marshalToAppWriter(appCtx, resp)
}

func SolutionGetBeam(appCtx *cli.Context) error {
	name, err := requireOneResourceName(appCtx, "beams/{beam}")
	if err != nil {
		return err
	}
	return runSolutionRPC(appCtx, func(ctx context.Context, c solutionpb.SolutionClient) (*solutionpb.Beam, error) {
		return c.GetBeam(ctx, &solutionpb.GetBeamRequest{Name: name})
	})
}

func SolutionQueryBeams(appCtx *cli.Context) error {
	timestamp, err := timestampFromFlags(appCtx)
	if err != nil {
		return err
	}
	interval, err := intervalFromFlags(appCtx)
	if err != nil {
		return err
	}
	return runSolutionRPC(appCtx, func(ctx context.Context, c solutionpb.SolutionClient) (*solutionpb.QueryBeamsResponse, error) {
		return c.QueryBeams(ctx, &solutionpb.QueryBeamsRequest{
			Queries: []*solutionpb.BeamQuery{{
				Timestamp:            timestamp,
				Interval:             interval,
				ProvisioningResource: appCtx.String("provisioning-resource"),
			}},
		})
	})
}

func SolutionGetP2PSrTePolicyCandidatePath(appCtx *cli.Context) error {
	name, err := requireOneResourceName(appCtx, "p2pSrTePolicyCandidatePaths/{path}")
	if err != nil {
		return err
	}
	return runSolutionRPC(appCtx, func(ctx context.Context, c solutionpb.SolutionClient) (*solutionpb.P2PSrTePolicyCandidatePath, error) {
		return c.GetP2PSrTePolicyCandidatePath(ctx, &solutionpb.GetP2PSrTePolicyCandidatePathRequest{Name: name})
	})
}

func SolutionQueryP2PSrTePolicyCandidatePaths(appCtx *cli.Context) error {
	timestamp, err := timestampFromFlags(appCtx)
	if err != nil {
		return err
	}
	interval, err := intervalFromFlags(appCtx)
	if err != nil {
		return err
	}
	return runSolutionRPC(appCtx, func(ctx context.Context, c solutionpb.SolutionClient) (*solutionpb.QueryP2PSrTePolicyCandidatePathsResponse, error) {
		return c.QueryP2PSrTePolicyCandidatePaths(ctx, &solutionpb.QueryP2PSrTePolicyCandidatePathsRequest{
			Queries: []*solutionpb.P2PSrTePolicyCandidatePathQuery{{
				Timestamp:                              timestamp,
				Interval:                               interval,
				ProvisioningP2PSrTePolicyCandidatePath: appCtx.String("provisioning-p2p-candidate-path"),
			}},
		})
	})
}

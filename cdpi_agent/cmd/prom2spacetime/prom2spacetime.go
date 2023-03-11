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

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	cpb "aalyria.com/spacetime/api/common"
	"aalyria.com/spacetime/cdpi_agent/telemetry/prometheus"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

func writeProtoAsText(w io.Writer, m proto.Message) error {
	_, err := w.Write(append([]byte(prototext.Format(m)), []byte{'\r', '\n'}...))
	return err
}

func writeProtoAsJSON(w io.Writer, m proto.Message) error {
	_, err := w.Write(append([]byte(protojson.MarshalOptions{Indent: "  "}.Format(m)), []byte{'\r', '\n'}...))
	return err
}

func writeProtoAsWire(w io.Writer, m proto.Message) error {
	// TODO: replace with the encoding/protodelim package once a new
	// version of the protobuf package gets cut
	// https://github.com/protocolbuffers/protobuf-go/commit/fb0abd915897428ccfdd6b03b48ad8219751ee54
	msgBytes, err := proto.Marshal(m)
	if err != nil {
		return err
	}

	sizeBytes := protowire.AppendVarint(nil, uint64(len(msgBytes)))
	if _, err = w.Write(msgBytes); err != nil {
		return err
	}
	if _, err = w.Write(sizeBytes); err != nil {
		return err
	}
	return nil
}

func run(ctx context.Context) error {
	fs := flag.NewFlagSet("prom2spacetime", flag.ContinueOnError)
	var (
		reportFormat   = fs.String("format", "text", "The format to print the NetworkStatsReport in (text, wire, or json)")
		exporterURL    = fs.String("exporter-url", "", "The full URL of the prometheus exporter's metrics page (typically /metrics)")
		nodeID         = fs.String("node-id", "", "The node ID to use in the NetworkStatsReport")
		scrapeInterval = fs.Duration("scrape-interval", 10*time.Second, "The frequency in which NetworkStatsReports are generated")
	)

	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}
	if *exporterURL == "" {
		return errors.New("missing exporter-url")
	}
	if *nodeID == "" {
		return errors.New("missing node-id")
	}
	var writeTo func(io.Writer, proto.Message) error
	switch rf := *reportFormat; rf {
	case "text":
		writeTo = writeProtoAsText
	case "json":
		writeTo = writeProtoAsJSON
	case "wire":
		writeTo = writeProtoAsWire
	default:
		return fmt.Errorf("unknown format: %q", rf)
	}

	return prometheus.NewScraper(prometheus.ScraperConfig{
		ExporterURL:    *exporterURL,
		NodeID:         *nodeID,
		ScrapeInterval: *scrapeInterval,
		Callback: func(r *cpb.NetworkStatsReport) error {
			return writeTo(os.Stdout, r)
		},
	}).Start(ctx)
}

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "fatal error: %s\n", err)
		os.Exit(2)
	}
}

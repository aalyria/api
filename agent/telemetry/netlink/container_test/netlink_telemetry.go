// Copyright 2024 Aalyria Technologies, Inc., and its affiliates.
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

// This test does a quick sanity check that the byte and packet counters provided by a netlink
// telemetry backend go up when traffic flows over an interface.
//
// It does this by:
// 1. creates an echo server that listens on the loopback address
// 2. generates a telemetry report for the loopback interface
// 3. hits the echo server to generate some traffic on the loopback interface
// 4. generates another telemetry report for the loopback interface
// 5. checks that numbers went up
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/jonboulle/clockwork"
	vnl "github.com/vishvananda/netlink"
	"golang.org/x/sync/errgroup"

	"aalyria.com/spacetime/agent/telemetry/netlink"
)

type echoServer struct {
	ln        net.Listener
	listening chan struct{}
	closer    chan struct{}
	closed    chan struct{}
}

func newEchoServer(listenAddr string) (*echoServer, error) {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("net.Listen: %w", err)
	}
	return &echoServer{
		ln:        ln,
		listening: make(chan struct{}),
		closer:    make(chan struct{}),
		closed:    make(chan struct{}),
	}, nil
}

func (es *echoServer) listen() {
	close(es.listening)
	defer close(es.closed)

	for {
		conn, err := es.ln.Accept()
		if err != nil {
			select {
			case <-es.closer:
				return
			default:
				log.Fatalf("echo server ln.Accept: %s", err)
			}
		}
		func() {
			defer conn.Close()

			if _, err = io.Copy(conn, conn); err != nil {
				log.Fatalf("echo server io.Copy: %s", err)
			}
		}()
	}
}

func (es *echoServer) run(ctx context.Context) error {
	go es.listen()

	var err error
	select {
	case <-ctx.Done():
		err = ctx.Err()
	case <-es.closer:
	}

	es.ln.Close()
	<-es.closed
	return err
}

func (es *echoServer) close() {
	close(es.closer)
}

// genTraffic writes an arbitrary string to the given address
func genTraffic(addr string) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("Dial: %w", err)
	}
	defer conn.Close()

	if _, err = conn.Write([]byte("Lions and tigers and bears, oh my!")); err != nil {
		return fmt.Errorf("Write: %w", err)
	}

	return nil
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)

	func() {
		echoAddr := "127.0.0.1:8080"
		echoServer, err := newEchoServer(echoAddr)
		if err != nil {
			log.Fatalf("newEchoServer: %v", err)
		}
		g.Go(func() error {
			return echoServer.run(ctx)
		})
		defer echoServer.close()

		select {
		case <-echoServer.listening:
		case <-ctx.Done():
			log.Fatalf("echo server not ready in time")
		}

		interfaceIDs := []string{"lo"}
		driver := netlink.New(clockwork.NewRealClock(), interfaceIDs, vnl.LinkByName)

		firstReport, err := driver.GenerateReport(ctx, "node_id")
		if err != nil {
			log.Fatalf("tb.GenerateReport failed with: %s", err)
		}
		log.Printf("first report: %v", firstReport)

		if err := genTraffic(echoAddr); err != nil {
			log.Fatalf("genTraffic: %s", err)
		}

		secondReport, err := driver.GenerateReport(ctx, "node_id")
		if err != nil {
			log.Fatalf("tb.GenerateReport failed with: %s", err)
		}
		log.Printf("second report: %v", secondReport)

		firstStats, ok := firstReport.InterfaceStatsById["lo"]
		if !ok {
			log.Fatalf("firstReport missing 'lo' interface stats")
		}
		secondStats, ok := secondReport.InterfaceStatsById["lo"]
		if !ok {
			log.Fatalf("secondReport missing 'lo' interface stats")
		}

		if *secondStats.TxPackets > *firstStats.TxPackets &&
			*secondStats.RxPackets > *firstStats.RxPackets &&
			*secondStats.TxBytes > *firstStats.TxBytes &&
			*secondStats.RxBytes > *firstStats.RxBytes {
			fmt.Printf("PASS: Byte and packet counters all went up")
		} else {
			fmt.Printf("FAIL: Byte and packet counters did not all go up")
		}
	}()

	g.Wait()
}

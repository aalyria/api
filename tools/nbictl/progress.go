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
	"io"
	"os"
	"time"

	"golang.org/x/text/message"

	"github.com/gosuri/uiprogress"
	"github.com/mattn/go-isatty"
	"github.com/urfave/cli/v2"
)

func shouldShowProgress(appCtx *cli.Context) bool {
	switch appCtx.String("progress") {
	case "on":
		return true
	case "off":
		return false
	default:
		return isatty.IsTerminal(os.Stderr.Fd())
	}
}

type syncProgress struct {
	p            *uiprogress.Progress
	printer      *message.Printer
	enabled      bool
	counterWidth int
}

func newSyncProgress(enabled bool) *syncProgress {
	sp := &syncProgress{enabled: enabled, printer: message.NewPrinter(message.MatchLanguage(os.Getenv("LANGUAGE"), "en"))}
	if enabled {
		sp.p = uiprogress.New()
		sp.p.SetOut(os.Stderr)
	}
	return sp
}

func (sp *syncProgress) Start() {
	if sp.enabled {
		sp.p.Start()
	}
}

func (sp *syncProgress) Stop() {
	if sp.enabled {
		sp.p.Stop()
	}
}

func (sp *syncProgress) Writer() io.Writer {
	if sp.enabled {
		return sp.p.Bypass()
	}
	return os.Stderr
}

func (sp *syncProgress) AddBar(label string, total int) *progressBar {
	if !sp.enabled || total == 0 {
		return &progressBar{}
	}
	bar := sp.p.AddBar(total)
	bar.AppendCompleted()
	bar.AppendFunc(func(b *uiprogress.Bar) string {
		took := b.TimeElapsed()
		if bar.Current() < b.Total {
			took = time.Since(b.TimeStarted)
		}
		return took.Truncate(time.Millisecond).String()
	})
	if w := len(sp.printer.Sprintf("%d", total)); w > sp.counterWidth {
		sp.counterWidth = w
	}
	bar.PrependFunc(func(b *uiprogress.Bar) string {
		return sp.printer.Sprintf("%-30s %*d/%-*d", label, sp.counterWidth, b.Current(), sp.counterWidth, b.Total)
	})
	bar.TimeStarted = time.Now()
	return &progressBar{bar: bar}
}

type progressBar struct {
	bar *uiprogress.Bar
}

func (pb *progressBar) Incr() {
	if pb.bar != nil {
		pb.bar.Incr()
	}
}

func (pb *progressBar) SetTotal(total int) {
	if pb.bar != nil {
		pb.bar.Total = total
	}
}

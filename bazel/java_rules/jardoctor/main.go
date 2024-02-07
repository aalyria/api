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

// Package main provides a tool that attempts to fix shortcomings of the bazel
// uberjar build process. It handles combining flags.xml declarations which are
// created by the Google flags library we're using, as well as combinig
// Log4j2Plugins.dat files which are required to configure the log4j handlers.
// The default Bazel build process does not merge these correctly, instead it
// chooses a winner and only includes that version.
package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

const (
	log4jDataPath = "META-INF/org/apache/logging/log4j/core/config/plugins/Log4j2Plugins.dat"
	flagDataPath  = "flags.xml"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s ARGS.JSON\n", os.Args[0])
		os.Exit(1)
	}

	if err := run(context.Background(), os.Args[1]); err != nil {
		fmt.Fprintf(os.Stderr, "fatal error: %v\n", err)
		os.Exit(1)
	}
}

type Args struct {
	InputJar      string   `json:"input_jar"`
	OutputJar     string   `json:"output_jar"`
	ClasspathJars []string `json:"classpath_jars"`
}

func run(_ context.Context, argFile string) error {
	args, err := readArgs(argFile)
	if err != nil {
		return err
	}

	logConf, err := mergeLogConfigs(gatherLogConfigs(args.ClasspathJars))
	if err != nil {
		return err
	}
	flagDoc := mergeDocs(gatherFlagDocuments(args.ClasspathJars))

	r, err := zip.OpenReader(args.InputJar)
	if err != nil {
		return err
	}
	defer r.Close()

	out, err := os.Create(args.OutputJar)
	if err != nil {
		return err
	}
	defer out.Close()

	return patchZip(logConf, flagDoc, out, r)
}

func readArgs(argFile string) (Args, error) {
	f, err := os.Open(argFile)
	if err != nil {
		return Args{}, err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	a := Args{}
	if err := dec.Decode(&a); err != nil {
		return Args{}, err
	}
	return a, nil
}

func patchZip(logConf *LogConfig, doc *FlagDocument, out io.Writer, r *zip.ReadCloser) error {
	w := zip.NewWriter(out)
	for _, f := range r.File {
		if f.Name == flagDataPath || f.Name == log4jDataPath || strings.HasSuffix(f.Name, "/") {
			continue
		}

		if err := w.Copy(f); err != nil {
			return fmt.Errorf("copying %s to new jar: %w", f.Name, err)
		}
	}

	// copy flag data
	fw, err := w.Create(flagDataPath)
	if err != nil {
		return err
	}
	enc := xml.NewEncoder(fw)
	if err := enc.Encode(doc); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}

	// copy log4j data
	lw, err := w.Create(log4jDataPath)
	if err != nil {
		return err
	}
	logData, err := marshalLogConfig(logConf)
	if err != nil {
		return err
	}
	if _, err := lw.Write(logData); err != nil {
		return err
	}

	if err := w.Flush(); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return nil
}

// FlagDocument represents the contents of a flags.xml file.
//
// Example document:
//
//	<flags>
//		<flag>
//			<name>com.google.googlex.minkowski.link.weather.WeatherClient.defaultUrl</name>
//			<shortname>weather_server_url</shortname>
//			<doc>Base URL of the weather server</doc>
//			<doclevel>PUBLIC</doclevel>
//			<type>java.lang.String</type>
//		</flag>
//	</flags>
type FlagDocument struct {
	XMLName xml.Name `xml:"flags"`
	Flags   []struct {
		Body []byte `xml:",innerxml"`
	} `xml:"flag"`
}

func mergeDocs(docs []*FlagDocument) *FlagDocument {
	doc := &FlagDocument{}
	for _, d := range docs {
		if d != nil {
			doc.Flags = append(doc.Flags, d.Flags...)
		}
	}
	return doc
}

// Log4j data files are written using the PluginCache.writeCache function
// https://github.com/apache/logging-log4j2/blob/d7febdda98dd409a4e2f3c6c538ea7535e66e2c9/log4j-core/src/main/java/org/apache/logging/log4j/core/config/plugins/processor/PluginCache.java#L66.
// It uses a DataOutputStream to serialize information about a plugin to a
// binary file. The format looks roughly like this:
//
//	size (int32)
//	[
//		category (string)
//		num_plugins (int32)
//		[
//			key (string)
//			className (string)
//			name (string)
//			printable (bool)
//			defer (bool)
//		]
//	]
//
// Instead of decoding the inner portion, this code only handles rewriting the
// root `size` field and then merely concatenates the individual entries
// together (stored in the [LogConfig.Body] field.
type LogConfig struct {
	Src  string
	Data []byte
	Size int32
}

func mergeLogConfigs(confs []*LogConfig) (*LogConfig, error) {
	confs = slices.DeleteFunc(confs, func(lc *LogConfig) bool {
		return lc == nil
	})

	n := int32(0)
	for _, lc := range confs {
		n += lc.Size
	}

	buf := &bytes.Buffer{}
	for _, lc := range confs {
		if _, err := buf.Write(lc.Data); err != nil {
			return nil, fmt.Errorf("writing log config data from %s: %w", lc.Src, err)
		}
	}
	return &LogConfig{Src: "combined", Size: n, Data: buf.Bytes()}, nil
}

func marshalLogConfig(lc *LogConfig) ([]byte, error) {
	buf := &bytes.Buffer{}
	if err := binary.Write(buf, binary.BigEndian, &lc.Size); err != nil {
		return nil, fmt.Errorf("writing merged size (%d): %w", lc.Size, err)
	}
	if _, err := buf.Write(lc.Data); err != nil {
		return nil, fmt.Errorf("writing merged data: %w", err)
	}
	return buf.Bytes(), nil
}

func unmarshalLogConfig(src string, data []byte) (*LogConfig, error) {
	buf := bytes.NewBuffer(data)
	var size int32
	if err := binary.Read(buf, binary.BigEndian, &size); err != nil {
		return nil, fmt.Errorf("reading size: %w", err)
	}

	return &LogConfig{Src: src, Size: size, Data: buf.Bytes()}, nil
}

func gatherLogConfigs(jars []string) []*LogConfig {
	wg := &sync.WaitGroup{}

	results := make([]*LogConfig, len(jars))
	for i, jar := range jars {
		i, jar := i, jar
		wg.Add(1)

		go func() {
			defer wg.Done()

			doc, err := extractLogConfig(jar)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error fetching Log4j2Plugins.dat: %v\n", err)
				return
			}
			results[i] = doc
		}()
	}
	wg.Wait()

	return results
}

func gatherFlagDocuments(jars []string) []*FlagDocument {
	wg := &sync.WaitGroup{}

	results := make([]*FlagDocument, len(jars))
	for i, jar := range jars {
		i, jar := i, jar
		wg.Add(1)

		go func() {
			defer wg.Done()

			doc, err := extractFlagDocument(jar)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error fetching %s: %v\n", flagDataPath, err)
				return
			}
			results[i] = doc
		}()
	}
	wg.Wait()

	return results
}

func extractFlagDocument(jar string) (fd *FlagDocument, err error) {
	if jar, err = filepath.EvalSymlinks(jar); err != nil {
		return fd, err
	}
	r, err := zip.OpenReader(jar)
	if err != nil {
		return fd, fmt.Errorf("reading jar %s: %w", jar, err)
	}
	defer r.Close()

	f, err := r.Open(flagDataPath)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return fd, fmt.Errorf("opening %s in %s: %w", flagDataPath, jar, err)
	}
	defer f.Close()

	xmlData, err := io.ReadAll(f)
	if err != nil {
		return fd, fmt.Errorf("reading %s in %s: %v", flagDataPath, jar, err)
	}
	fd = &FlagDocument{}
	if err := xml.Unmarshal(xmlData, fd); err != nil {
		return fd, fmt.Errorf("unmarshalling %s in %s: %v", flagDataPath, jar, err)
	}

	return fd, nil
}

func extractLogConfig(jar string) (lc *LogConfig, err error) {
	if jar, err = filepath.EvalSymlinks(jar); err != nil {
		return nil, err
	}
	r, err := zip.OpenReader(jar)
	if err != nil {
		return nil, fmt.Errorf("reading jar %s: %w", jar, err)
	}
	defer r.Close()

	f, err := r.Open(log4jDataPath)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("opening %s in %s: %w", log4jDataPath, jar, err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", log4jDataPath, err)
	}

	return unmarshalLogConfig(jar, data)
}

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
	"io/fs"
	"os"
	"path/filepath"
)

func findAllFilesWithExtension(fileExt string, recursive bool, paths ...string) ([]string, error) {
	files := []string{}

	for _, path := range paths {
		fileInfo, err := os.Stat(path)
		if err != nil {
			return nil, err
		}

		mode := fileInfo.Mode()
		switch {
		case mode.IsRegular():
			if filepath.Ext(path) == "."+fileExt {
				files = append(files, path)
			}
		case mode.IsDir():
			// Process this directory's files and optionally recurse.
			walkDirFn := func(path string, dirEnt fs.DirEntry, err error) error {
				switch {
				case dirEnt.Type().IsRegular():
					if filepath.Ext(path) == "."+fileExt {
						files = append(files, path)
					}
				case dirEnt.IsDir():
					if !recursive {
						return filepath.SkipDir
					}
				}
				return nil
			}
			filepath.WalkDir(path, walkDirFn)
		default:
			return nil, fmt.Errorf("cannot assess %q for reading", path)
		}
	}

	return files, nil
}

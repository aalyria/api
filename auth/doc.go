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

// Package auth provides credential helpers for connecting to Spacetime APIs.
//
// Users will create a [Config] and call [NewCredentials] to create a
// [google.golang.org/grpc/credentials.PerRPCCredentials] instance that can be
// used with the [google.golang.org/grpc.WithPerRPCCredentials] dial option to
// authenticate RPCs. See the [auth documentation] for more information.
//
// [auth documentation]: https://docs.spacetime.aalyria.com/authentication
package auth

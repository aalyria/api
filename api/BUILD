# Copyright 2023 Aalyria Technologies, Inc., and its affiliates.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

load("@rules_proto_grpc_doc//:defs.bzl", "doc_markdown_compile", "doc_template_compile")

# bazel query 'kind("^proto_library", //api/...)'
_all_protos = [
    "//api/common:common_proto",
    "//api/federation:federation_proto",
    "//api/nbi/v1alpha:nbi_proto",
    "//api/nbi/v1alpha/resources:resources_proto",
    "//api/types:types_proto",
]

# The default HTML template that comes with the protoc-gen-doc tool adds a
# `<p></p>` to every newline in every description. This ends up causing weird,
# short line breaks. We use a custom template that just renders the description
# as-is inside a single `<p>` tag.
# https://github.com/pseudomuto/protoc-gen-doc/blob/df9dd4078971bb01d06bcd88130a7b5309348be0/resources/html.tmpl
doc_template_compile(
    name = "api.html",
    protos = _all_protos,
    template = "//api/resources:html.tmpl",
)

doc_markdown_compile(
    name = "api.md",
    protos = _all_protos,
)

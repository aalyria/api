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

load("@rules_java//java:defs.bzl", native_java_binary = "java_binary", native_java_library = "java_library", native_java_test = "java_test")

def java_library(javacopts = [], plugins = [], **kwargs):
    native_java_library(
        javacopts = _actual_javacopts(javacopts),
        plugins = _actual_plugins(plugins),
        **kwargs
    )

def java_binary(javacopts = [], plugins = [], **kwargs):
    native_java_binary(
        javacopts = _actual_javacopts(javacopts),
        plugins = _actual_plugins(plugins),
        **kwargs
    )

def java_test(javacopts = [], plugins = [], **kwargs):
    native_java_test(
        javacopts = _actual_javacopts(javacopts),
        plugins = _actual_plugins(plugins),
        **kwargs
    )

def _actual_javacopts(javacopts):
    return select({
        "@scip_java//semanticdb-javac:is_enabled": ["'-Xplugin:semanticdb -build-tool:bazel'"] + javacopts,
        "//conditions:default": javacopts,
    })

def _actual_plugins(plugins):
    return select({
        "@scip_java//semanticdb-javac:is_enabled": ["@scip_java//semanticdb-javac:plugin"] + plugins,
        "//conditions:default": plugins,
    })

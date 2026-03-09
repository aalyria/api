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

"""A set of small wrappers around the native Java rules that enable more
cohesive uberjar builds (see _java_deployable_jar) and code intelligence (see
_actual_javacopts)."""

load("@rules_java//java:defs.bzl", native_java_binary = "java_binary", native_java_library = "java_library", native_java_test = "java_test")

def _java_deployable_jar_impl(ctx):
    jars = [x for x in ctx.attr.binary[JavaInfo].compilation_info.runtime_classpath.to_list()]

    patched_jar = ctx.actions.declare_file("%s.jar" % ctx.label.name)
    args_json = ctx.actions.declare_file("%s_args.json" % ctx.label.name)
    ctx.actions.write(
        output = args_json,
        content = json.encode({
            "classpath_jars": [j.path for j in jars],
            "input_jar": ctx.file.deploy_jar.path,
            "output_jar": patched_jar.path,
        }),
    )

    ctx.actions.run(
        outputs = [patched_jar],
        inputs = [args_json, ctx.file.deploy_jar] + jars,
        arguments = [args_json.path],
        executable = ctx.executable._jardoctor,
    )

    return [
        DefaultInfo(files = depset([patched_jar])),
        ctx.attr.binary[JavaInfo],
    ]

_java_deployable_jar = rule(
    implementation = _java_deployable_jar_impl,
    attrs = {
        "binary": attr.label(
            mandatory = True,
            doc = "java_binary to produce a deployable jar for",
            providers = [JavaInfo],
        ),
        "deploy_jar": attr.label(
            mandatory = True,
            doc = "_deploy.jar artifact for given binary",
            allow_single_file = ["_deploy.jar"],
        ),
        "_jardoctor": attr.label(
            default = "//bazel/java_rules/jardoctor",
            executable = True,
            cfg = "exec",
        ),
    },
)

def java_library(javacopts = [], plugins = [], **kwargs):
    native_java_library(
        javacopts = _actual_javacopts(javacopts),
        plugins = _actual_plugins(plugins),
        **kwargs
    )

def java_binary(name, javacopts = [], plugins = [], **kwargs):
    native_java_binary(
        name = name,
        javacopts = _actual_javacopts(javacopts),
        plugins = _actual_plugins(plugins),
        **kwargs
    )
    _java_deployable_jar(
        name = "%s_deployable" % name,
        binary = ":%s" % name,
        deploy_jar = ":%s_deploy.jar" % name,
        testonly = kwargs.get("testonly", 0),
    )

def java_test(javacopts = [], plugins = [], **kwargs):
    native_java_test(
        javacopts = _actual_javacopts(javacopts),
        plugins = _actual_plugins(plugins),
        **kwargs
    )

def _actual_javacopts(javacopts):
    return select({
        "//conditions:default": javacopts,
    })

def _actual_plugins(plugins):
    return select({
        "//conditions:default": plugins,
    })

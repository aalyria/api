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
"""Bazel rules for testing compliance with google-java-format."""

load("@bazel_skylib//lib:shell.bzl", "shell")

def _java_format_test_impl(ctx):
    out_file = ctx.actions.declare_file(ctx.label.name + ".bash")
    runfiles = [ctx.executable.google_java_format]
    substitutions = {
        "@@GOOGLE_JAVA_FORMAT@@": shell.quote(ctx.executable.google_java_format.short_path),
        "@@SRCS@@": "",
    }
    if ctx.file.workspace != None:
        runfiles.append(ctx.file.workspace)
        substitutions["@@WORKSPACE@@"] = ctx.file.workspace.path
    else:
        for f in ctx.files.srcs:
            runfiles.append(f)
        substitutions["@@SRCS@@"] = " ".join([shell.quote(f.short_path) for f in ctx.files.srcs])

    ctx.actions.expand_template(
        template = ctx.file._runner,
        output = out_file,
        substitutions = substitutions,
        is_executable = True,
    )

    shell_runfiles = ctx.runfiles(files = runfiles)
    merged_runfiles = shell_runfiles.merge(ctx.attr.google_java_format[DefaultInfo].default_runfiles)

    return DefaultInfo(
        files = depset([out_file]),
        runfiles = merged_runfiles,
        executable = out_file,
    )

java_format_test = rule(
    implementation = _java_format_test_impl,
    test = True,
    attrs = {
        "srcs": attr.label_list(
            allow_files = [".java"],
            doc = "A list of Java files to check for formatting",
        ),
        "google_java_format": attr.label(
            default = "//third_party/java/google_java_format",
            cfg = "exec",
            executable = True,
        ),
        "workspace": attr.label(
            allow_single_file = True,
            doc = "Label of the WORKSPACE file",
        ),
        "_runner": attr.label(
            default = ":runner.bash.template",
            allow_single_file = True,
        ),
    },
)

# def _validation_aspect_impl(target, ctx):
#   validation_output = ctx.actions.declare_file(ctx.rule.attr.name + ".validation")
#   args = ctx.actions.args()
#   args.add(validation_output)
#   inputs = []
#   if hasattr(ctx.rule.attr, "srcs"):
#     for s in ctx.rule.attr.srcs:
#         for f in s.files.to_list():
#             if f.extension == "java" and not f.path.startswith("external"):
#                 inputs.append(f)
#                 args.add(f.path)

#   if
#   if not inputs:
#     ctx.actions.write(output = validation_output, content="")
#   else:
#     ctx.actions.run_shell(
#         inputs = inputs,
#         outputs = [validation_output],
#         arguments =  [args],
#         tools = [ctx.executable._google_java_format],
#         progress_message = "Checking %d files for formatting" % len(inputs),
#         command = 'set -x; outfile="$1"; shift; "%s" -n --set-exit-if-changed "$@" > "$outfile"' % ctx.executable._google_java_format.path,
#     )

#   validation_outputs_in_deps = []
#   for d in ctx.rule.attr.deps:
#     validation_outputs_in_deps.append(d[OutputGroupInfo]._validation)

#   return [
#     OutputGroupInfo(
#       _validation = depset(
#         [validation_output],
#         transitive = validation_outputs_in_deps)),
#   ]

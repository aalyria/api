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

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

# Bazel platforms
http_archive(
    name = "platforms",
    sha256 = "5308fc1d8865406a49427ba24a9ab53087f17f5266a7aabbfc28823f3916e1ca",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/platforms/releases/download/0.0.6/platforms-0.0.6.tar.gz",
        "https://github.com/bazelbuild/platforms/releases/download/0.0.6/platforms-0.0.6.tar.gz",
    ],
)

# Download rules_proto_grpc respository.
http_archive(
    name = "rules_proto_grpc",
    sha256 = "507e38c8d95c7efa4f3b1c0595a8e8f139c885cb41a76cab7e20e4e67ae87731",
    strip_prefix = "rules_proto_grpc-4.1.1",
    urls = ["https://github.com/rules-proto-grpc/rules_proto_grpc/archive/4.1.1.tar.gz"],
)

load("@rules_proto_grpc//:repositories.bzl", "rules_proto_grpc_toolchains")


# Download the bazelbuild rules_go repository.
http_archive(
    name = "io_bazel_rules_go",
    sha256 = "dd926a88a564a9246713a9c00b35315f54cbd46b31a26d5d8fb264c07045f05d",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/rules_go/releases/download/v0.38.1/rules_go-v0.38.1.zip",
        "https://github.com/bazelbuild/rules_go/releases/download/v0.38.1/rules_go-v0.38.1.zip",
    ],
)

load("@io_bazel_rules_go//go:deps.bzl", "go_register_toolchains", "go_rules_dependencies")

# Download bazel_gazelle Bazel repository.
http_archive(
    name = "bazel_gazelle",
    sha256 = "5982e5463f171da99e3bdaeff8c0f48283a7a5f396ec5282910b9e8a49c0dd7e",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/bazel-gazelle/releases/download/v0.25.0/bazel-gazelle-v0.25.0.tar.gz",
        "https://github.com/bazelbuild/bazel-gazelle/releases/download/v0.25.0/bazel-gazelle-v0.25.0.tar.gz",
    ],
)

load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies", "go_repository")

go_repository(
    name = "org_golang_google_protobuf",
    importpath = "google.golang.org/protobuf",
    sum = "h1:w43yiav+6bVFTBQFZX0r7ipe9JQ1QsbMgHwbBziscLw=",
    version = "v1.28.0",
)

go_repository(
    name = "org_golang_x_sys",
    importpath = "golang.org/x/sys",
    sum = "h1:w36l2Uw3dRan1K3TyXriXvY+6T56GNmlKGcqiQUJDfM=",
    version = "v0.0.0-20220517195934-5e4e11fc645e",
)

go_repository(
    name = "org_golang_x_net",
    importpath = "golang.org/x/net",
    sum = "h1:cCR+9mKLOGyX4Zx+uBZDXEDAQsvKQ/XbW4vreG5v1jU=",
    version = "v0.0.0-20220517181318-183a9ca12b87",
)

go_repository(
    name = "org_golang_x_text",
    importpath = "golang.org/x/text",
    sum = "h1:olpwvP2KacW1ZWvsR7uQhoyTYvKAupfQrRGBFM352Gk=",
    version = "v0.3.7",
)

go_repository(
    name = "com_google_cloud_go_compute",
    importpath = "cloud.google.com/go/compute",
    sum = "h1:b1zWmYuuHz7gO9kDcM/EpHGr06UgsYNRpNJzI2kFiLM=",
    version = "v1.5.0",
)

go_repository(
    name = "org_golang_x_oauth2",
    importpath = "golang.org/x/oauth2",
    sum = "h1:OSnWWcOd/CtWQC2cYSBgbTSJv3ciqd8r54ySIW2y3RE=",
    version = "v0.0.0-20220411215720-9780585627b5",
)

go_repository(
    name = "org_golang_google_grpc",

    # Ignore proto files in this library, per
    # https://github.com/bazelbuild/rules_go/blob/master/go/dependencies.rst#grpc-dependencies.
    build_file_proto_mode = "disable",
    importpath = "google.golang.org/grpc",
    sum = "h1:u+MLGgVf7vRdjEYZ8wDFhAVNmhkbJ5hmrA1LMWK1CAQ=",
    version = "v1.46.2",
)

go_repository(
    name = "org_golang_google_api",
    importpath = "google.golang.org/api",
    sum = "h1:IQWaGVCYnsm4MO3hh+WtSXMzMzuyFx/fuR8qkN3A0Qo=",
    version = "v0.80.0",
)

go_repository(
    name = "com_github_rs_zerolog",
    importpath = "github.com/rs/zerolog",
    sum = "h1:38k9hgtUBdxFwE34yS8rTHmHBa4eN16E4DJlv177LNs=",
    version = "v1.20.0",
)

go_repository(
    name = "com_github_google_go_cmp",
    importpath = "github.com/google/go-cmp",
    sum = "h1:Xye71clBPdm5HgqGwUkwhbynsUJZhDbS20FvLhQ2izg=",
    version = "v0.3.1",
)

go_repository(
    name = "com_github_golang_groupcache",
    importpath = "github.com/golang/groupcache",
    sum = "h1:oI5xCqsCo564l8iNU+DwB5epxmsaqB+rhGL0m5jtYqE=",
    version = "v0.0.0-20210331224755-41bb18bfe9da",
)

go_repository(
    name = "com_github_jonboulle_clockwork",
    importpath = "github.com/jonboulle/clockwork",
    sum = "h1:9BSCMi8C+0qdApAp4auwX0RkLGUjs956h0EkuQymUhg=",
    version = "v0.3.0",
)

go_repository(
    name = "org_golang_x_sync",
    importpath = "golang.org/x/sync",
    sum = "h1:w8s32wxx3sY+OjLlv9qltkLU5yvJzxjjgiHWLjdIcw4=",
    version = "v0.0.0-20220513210516-0976fa681c29",
)

go_repository(
    name = "io_opencensus_go",
    importpath = "go.opencensus.io",
    sum = "h1:gqCw0LfLxScz8irSi8exQc7fyQ0fKQU/qnC/X8+V/1M=",
    version = "v0.23.0",
)

go_repository(
    name = "com_github_peterbourgon_ff",
    importpath = "github.com/peterbourgon/ff/v3",
    sum = "h1:0GNhbRhO9yHA4CC27ymskOsuRpmX0YQxwxM9UPiP6JM=",
    version = "v3.1.2",
)

go_repository(
    name = "com_github_mattn_go_colorable",
    importpath = "github.com/mattn/go-colorable",
    sum = "h1:jF+Du6AlPIjs2BiUiQlKOX0rt3SujHxPnksPKZbaA40=",
    version = "v0.1.12",
)

go_repository(
    name = "com_github_mattn_go_isatty",
    importpath = "github.com/mattn/go-isatty",
    sum = "h1:yVuAays6BHfxijgZPzw+3Zlu5yQgKGP2/hcQbHb7S9Y=",
    version = "v0.0.14",
)

# Register external dependencies needed by the Go rules.
go_rules_dependencies()

# Install the Go toolchains. (https://github.com/bazelbuild/rules_go/blob/master/go/toolchains.rst#go-toolchain)
go_register_toolchains(
    version = "1.20",
)

gazelle_dependencies()

# Register the rules_proto_grpc toolchains
rules_proto_grpc_toolchains()

##
# Specify a newer gRPC version that what @rules_proto_grpc pulls in.
#
# This can be removed when the rules_proto_grpc VERSION is incremented past
# the version specified here, see:
#   - https://github.com/rules-proto-grpc/rules_proto_grpc/blob/master/repositories.bzl
#
# Alternatives include changing to a different ruleset, though rules_proto_grpc
# seems to have the broadest multilanguage support.  See also:
#
#   - https://bazel-contrib.github.io/SIG-rules-authors/proto-grpc.html
#   - https://rules-proto-grpc.com/en/latest/
#   - https://github.com/stackb/rules_proto
##
http_archive(
    name = "com_github_grpc_grpc",
    sha256 = "cdeb805385fba23242bf87073e68d590c446751e09089f26e5e0b3f655b0f089",
    strip_prefix = "grpc-1.49.2",
    urls = ["https://github.com/grpc/grpc/archive/refs/tags/v1.49.2.tar.gz"],
)

load("@rules_proto_grpc//go:repositories.bzl", rules_proto_grpc_go_repos = "go_repos")

# Download dependencies for rules_proto_grpc Go rules.
rules_proto_grpc_go_repos()

load("@rules_proto_grpc//cpp:repositories.bzl", rules_proto_grpc_cpp_repos = "cpp_repos")

# Download dependencies for rules_proto_grpc cpp rules.
rules_proto_grpc_cpp_repos()

load("@rules_proto_grpc//java:repositories.bzl", rules_proto_grpc_java_repos = "java_repos")

# Download dependencies for rules_proto_grpc Java rules.
rules_proto_grpc_java_repos()

load("@rules_jvm_external//:defs.bzl", "maven_install")
load("@io_grpc_grpc_java//:repositories.bzl", "IO_GRPC_GRPC_JAVA_ARTIFACTS", "IO_GRPC_GRPC_JAVA_OVERRIDE_TARGETS", "grpc_java_repositories")

# Resolve and fetch grpc-java dependencies from a Maven respository.
maven_install(
    artifacts = IO_GRPC_GRPC_JAVA_ARTIFACTS,
    generate_compat_repositories = True,
    override_targets = IO_GRPC_GRPC_JAVA_OVERRIDE_TARGETS,
    repositories = [
        "https://repo.maven.apache.org/maven2/",
    ],
)

load("@maven//:compat.bzl", "compat_repositories")

compat_repositories()

# Import the the grpc-java dependencies.
grpc_java_repositories()

# Download the googleapis repository.
http_archive(
    name = "com_google_googleapis",
    sha256 = "a2d6aa85990929af1f4a6f6455cbd6fb60f000cb38c9f04a275ec359b95c02d4",
    strip_prefix = "googleapis-9346947b1ce6173e9c82fe3267af02360517398c",
    urls = ["https://github.com/googleapis/googleapis/archive/9346947b1ce6173e9c82fe3267af02360517398c.zip"],
)

load("@com_google_googleapis//:repository_rules.bzl", "switched_rules_by_language")

# Enable googleapis rules for the relevant languages.
switched_rules_by_language(
    name = "com_google_googleapis_imports",
    cc = True,
    java = True,
)

load("@com_github_grpc_grpc//bazel:grpc_deps.bzl", "grpc_deps")

# Load dependencies needed to compile and test the grpc library.
grpc_deps()

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

http_archive(
    name = "rules_pkg",
    sha256 = "eea0f59c28a9241156a47d7a8e32db9122f3d50b505fae0f33de6ce4d9b61834",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/rules_pkg/releases/download/0.8.0/rules_pkg-0.8.0.tar.gz",
        "https://github.com/bazelbuild/rules_pkg/releases/download/0.8.0/rules_pkg-0.8.0.tar.gz",
    ],
)

load("@rules_pkg//:deps.bzl", "rules_pkg_dependencies")

rules_pkg_dependencies()

http_archive(
    name = "com_google_protobuf",
    sha256 = "930c2c3b5ecc6c9c12615cf5ad93f1cd6e12d0aba862b572e076259970ac3a53",
    strip_prefix = "protobuf-3.21.12",
    urls = ["https://github.com/protocolbuffers/protobuf/archive/v3.21.12.tar.gz"],
)

# Download rules_proto_grpc respository.
http_archive(
    name = "rules_proto_grpc",
    sha256 = "507e38c8d95c7efa4f3b1c0595a8e8f139c885cb41a76cab7e20e4e67ae87731",
    strip_prefix = "rules_proto_grpc-4.1.1",
    urls = ["https://github.com/rules-proto-grpc/rules_proto_grpc/archive/4.1.1.tar.gz"],
)

load("@rules_proto_grpc//:repositories.bzl", "rules_proto_grpc_toolchains")

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
    sum = "h1:Zes4hju04hjbvkVkOhdl2HpZa+0PmVwigmo8XoORE5w=",
    version = "v1.29.0",
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

go_repository(
    name = "io_opentelemetry_go_otel",
    importpath = "go.opentelemetry.io/otel",
    sum = "h1:/79Huy8wbf5DnIPhemGB+zEPVwnN6fuQybr/SRXa6hM=",
    version = "v1.14.0",
)

go_repository(
    name = "io_opentelemetry_go_otel_sdk",
    importpath = "go.opentelemetry.io/otel/sdk",
    sum = "h1:PDCppFRDq8A1jL9v6KMI6dYesaq+DFcDZvjsoGvxGzY=",
    version = "v1.14.0",
)

go_repository(
    name = "io_opentelemetry_go_otel_sdk_metric",
    importpath = "go.opentelemetry.io/otel/sdk/metric",
    sum = "h1:haYBBtZZxiI3ROwSmkZnI+d0+AVzBWeviuYQDeBWosU=",
    version = "v0.37.0",
)

go_repository(
    name = "io_opentelemetry_go_otel_metric",
    importpath = "go.opentelemetry.io/otel/metric",
    sum = "h1:pHDQuLQOZwYD+Km0eb657A25NaRzy0a+eLyKfDXedEs=",
    version = "v0.37.0",
)

go_repository(
    name = "io_opentelemetry_go_otel_trace",
    importpath = "go.opentelemetry.io/otel/trace",
    sum = "h1:ofxdnzsNrGBYXbP7t7zpUK281+go5rF7dvdIZXF8gdQ=",
    version = "v1.11.1",
)

go_repository(
    name = "com_github_go_logr_logr",
    importpath = "github.com/go-logr/logr",
    sum = "h1:2DntVwHkVopvECVRSlL5PSo9eG+cAkDCuckLubN+rq0=",
    version = "v1.2.3",
)

go_repository(
    name = "com_github_go_logr_stdr",
    importpath = "github.com/go-logr/stdr",
    sum = "h1:hSWxHoqTgW2S2qGc0LTAI563KZ5YKYRhT3MFKZMbjag=",
    version = "v1.2.2",
)

go_repository(
    name = "com_github_prometheus_client_model",
    importpath = "github.com/prometheus/client_model",
    sum = "h1:UBgGFHqYdG/TPFD1B1ogZywDqEkwp3fBMvqdiQ7Xew4=",
    version = "v0.3.0",
)

go_repository(
    name = "com_github_prometheus_prom2json",
    importpath = "github.com/prometheus/prom2json",
    sum = "h1:heRKAGHWqm8N3IaRDDNBBJNVS6a9mLdsTlFhvOaNGb0=",
    version = "v1.3.2",
)

go_repository(
    name = "com_github_prometheus_common",
    importpath = "github.com/prometheus/common",
    sum = "h1:Afz7EVRqGg2Mqqf4JuF9vdvp1pi220m55Pi9T2JnO4Q=",
    version = "v0.40.0",
)

go_repository(
    name = "com_github_matttproud_golang_protobuf_extensions",
    importpath = "github.com/matttproud/golang_protobuf_extensions",
    sum = "h1:mmDVorXM7PCGKw94cs5zkfA9PSy5pEvNWRP0ET0TIVo=",
    version = "v1.0.4",
)

go_repository(
    name = "com_github_json_iterator_go",
    importpath = "github.com/json-iterator/go",
    sum = "h1:PV8peI4a0ysnczrg+LtxykD8LfKY9ML6u2jnxaEnrnM=",
    version = "v1.1.12",
)

go_repository(
    name = "com_github_modern_go_reflect2",
    importpath = "github.com/modern-go/reflect2",
    sum = "h1:xBagoLtFs94CBntxluKeaWgTMpvLxC4ur3nMaC9Gz0M=",
    version = "v1.0.2",
)

go_repository(
    name = "com_github_modern_go_concurrent",
    importpath = "github.com/modern-go/concurrent",
    sum = "h1:TRLaZ9cD/w8PVh93nsPXa1VrQ6jlwL5oN8l14QlcNfg=",
    version = "v0.0.0-20180306012644-bacd9c7ef1dd",
)

go_repository(
    name = "io_opentelemetry_go_otel_exporters_otlp_otlptrace",
    importpath = "go.opentelemetry.io/otel/exporters/otlp/otlptrace",
    sum = "h1:TKf2uAs2ueguzLaxOCBXNpHxfO/aC7PAdDsSH0IbeRQ=",
    version = "v1.14.0",
)

go_repository(
    name = "io_opentelemetry_go_otel_exporters_otlp_otlptrace_otlptracegrpc",
    importpath = "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc",
    sum = "h1:ap+y8RXX3Mu9apKVtOkM6WSFESLM8K3wNQyOU8sWHcc=",
    version = "v1.14.0",
)

go_repository(
    name = "io_opentelemetry_go_otel_exporters_otlp_internal_retry",
    importpath = "go.opentelemetry.io/otel/exporters/otlp/internal/retry",
    sum = "h1:/fXHZHGvro6MVqV34fJzDhi7sHGpX3Ej/Qjmfn003ho=",
    version = "v1.14.0",
)

go_repository(
    name = "io_opentelemetry_go_proto_otlp",
    importpath = "go.opentelemetry.io/proto/otlp",
    sum = "h1:IVN6GR+mhC4s5yfcTbmzHYODqvWAp3ZedA2SJPI1Nnw=",
    version = "v0.19.0",
)

go_repository(
    name = "io_opentelemetry_go_contrib_instrumentation_google_golang_org_grpc_otelgrpc",
    importpath = "go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc",
    sum = "h1:5jD3teb4Qh7mx/nfzq4jO2WFFpvXD0vYWFDrdvNWmXk=",
    version = "v0.40.0",
)

go_repository(
    name = "io_opentelemetry_go_contrib_instrumentation_net_http_otelhttp",
    importpath = "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp",
    sum = "h1:lE9EJyw3/JhrjWH/hEy9FptnalDQgj7vpbgC2KCCCxE=",
    version = "v0.40.0",
)

go_repository(
    name = "com_github_felixge_httpsnoop",
    importpath = "github.com/felixge/httpsnoop",
    sum = "h1:s/nj+GCswXYzN5v2DpNMuMQYe+0DDwt5WVCU6CWBdXk=",
    version = "v1.0.3",
)

go_repository(
    name = "com_github_cenkalti_backoff_v4",
    importpath = "github.com/cenkalti/backoff/v4",
    sum = "h1:HN5dHm3WBOgndBH6E8V0q2jIYIR3s9yglV8k/+MN3u4=",
    version = "v4.2.0",
)

go_repository(
    name = "com_github_grpc_ecosystem_grpc_gateway_v2",
    importpath = "github.com/grpc-ecosystem/grpc-gateway/v2",
    sum = "h1:gDLXvp5S9izjldquuoAhDzccbskOL6tDC5jMSyx3zxE=",
    version = "v2.15.2",
)

go_repository(
    name = "com_github_golang_jwt_jwt_v5",
    importpath = "github.com/golang-jwt/jwt/v5",
    sum = "h1:hXPcSazn8wKOfSb9y2m1bdgUMlDxVDarxh3lJVbC6JE=",
    version = "v5.0.0-rc.2",
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

# need to pull in a newer version of the envoy srcs to match the newer versions
# of grpc we pull in below
http_archive(
    name = "envoy_api",
    sha256 = "0fe4c68dea4423f5880c068abbcbc90ac4b98496cf2af15a1fe3fbc0fdb050fd",
    strip_prefix = "data-plane-api-bf6154e482bbd5e6f64032993206e66b6116f2bd",
    urls = [
        "https://storage.googleapis.com/grpc-bazel-mirror/github.com/envoyproxy/data-plane-api/archive/bf6154e482bbd5e6f64032993206e66b6116f2bd.tar.gz",
        "https://github.com/envoyproxy/data-plane-api/archive/bf6154e482bbd5e6f64032993206e66b6116f2bd.tar.gz",
    ],
)

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

http_archive(
    name = "io_grpc_grpc_java",
    sha256 = "52265f645422837174feed4d8f8e9e29ec5631f7f2f9aa053652584e462c78c9",
    strip_prefix = "grpc-java-f0614e5a76538607436d600a77f79d15da75d3fe",
    urls = ["https://github.com/grpc/grpc-java/archive/f0614e5a76538607436d600a77f79d15da75d3fe.tar.gz"],
)

http_archive(
    name = "rules_jvm_external",
    sha256 = "b17d7388feb9bfa7f2fa09031b32707df529f26c91ab9e5d909eb1676badd9a6",
    strip_prefix = "rules_jvm_external-4.5",
    url = "https://github.com/bazelbuild/rules_jvm_external/archive/refs/tags/4.5.zip",
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
    artifacts = 
        IO_GRPC_GRPC_JAVA_ARTIFACTS + 
        ["com.google.code.gson:gson:2.10.1",
        "org.bouncycastle:bcprov-jdk15on:1.70"],
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
    python = True,
)

load("@com_github_grpc_grpc//bazel:grpc_deps.bzl", "grpc_deps")

# Load dependencies needed to compile and test the grpc library.
grpc_deps()

load("@rules_proto_grpc//python:repositories.bzl", rules_proto_grpc_python_repos = "python_repos")

rules_proto_grpc_python_repos()
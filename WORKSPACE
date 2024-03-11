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

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive", "http_jar")

# Download bazel_gazelle Bazel repository.
http_archive(
    name = "bazel_gazelle",
    sha256 = "32938bda16e6700063035479063d9d24c60eda8d79fd4739563f50d331cb3209",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/bazel-gazelle/releases/download/v0.35.0/bazel-gazelle-v0.35.0.tar.gz",
        "https://github.com/bazelbuild/bazel-gazelle/releases/download/v0.35.0/bazel-gazelle-v0.35.0.tar.gz",
    ],
)

http_archive(
    name = "com_github_grpc_grpc",
    patches = [
        # gRPC uses a local (non-hermetic) version of Python by default, this
        # patch allows us to specify a python_headers attr that overrides this
        # behavior.
        #
        # See https://github.com/grpc/grpc/issues/31892
        "//patches:com_github_grpc_grpc-hermetic-python.patch",
    ],
    sha256 = "916f88a34f06b56432611aaa8c55befee96d0a7b7d7457733b9deeacbc016f99",
    strip_prefix = "grpc-1.59.1",
    urls = ["https://github.com/grpc/grpc/archive/refs/tags/v1.59.1.tar.gz"],
)


http_archive(
    name = "com_google_googleapis",
    sha256 = "ab7b8e43838ad1d446d4d92b132908579f524519d48b1d4ad160c088409fe510",
    strip_prefix = "googleapis-e54ab8ce48a24e63cce44fd330e92c4ae096985f",
    urls = ["https://github.com/googleapis/googleapis/archive/e54ab8ce48a24e63cce44fd330e92c4ae096985f.tar.gz"],
)

http_archive(
    name = "com_google_protobuf",
    # TODO: remove this patch and upgrade the proto dep once a newer
    # version is available in the BCR and compatible with both rules_proto_grpc
    # and the rest of the ecosystem
    #
    # See:
    # * https://github.com/protocolbuffers/protobuf/issues/13618
    # * https://github.com/rules-proto-grpc/rules_proto_grpc/issues/299
    # * https://github.com/bazelbuild/rules_proto/issues/179
    patches = [
        "//patches:com_google_protobuf-rename-exec_tools-to-tools.patch",
    ],
    sha256 = "dc167b7d23ec0d6e4a3d4eae1798de6c8d162e69fa136d39753aaeb7a6e1289d",
    strip_prefix = "protobuf-23.1",
    urls = ["https://github.com/protocolbuffers/protobuf/archive/v23.1.tar.gz"],
)

http_archive(
    name = "build_bazel_rules_apple",
    sha256 = "34c41bfb59cdaea29ac2df5a2fa79e5add609c71bb303b2ebb10985f93fa20e7",
    url = "https://github.com/bazelbuild/rules_apple/releases/download/3.1.1/rules_apple.3.1.1.tar.gz",
)

http_archive(
    name = "build_bazel_apple_support",
    sha256 = "cf4d63f39c7ba9059f70e995bf5fe1019267d3f77379c2028561a5d7645ef67c",
    url = "https://github.com/bazelbuild/apple_support/releases/download/1.11.1/apple_support.1.11.1.tar.gz",
)

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "80a98277ad1311dacd837f9b16db62887702e9f1d1c4c9f796d0121a46c8e184",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/rules_go/releases/download/v0.46.0/rules_go-v0.46.0.zip",
        "https://github.com/bazelbuild/rules_go/releases/download/v0.46.0/rules_go-v0.46.0.zip",
    ],
)

http_archive(
    name = "io_grpc_grpc_java",
    sha256 = "5b934dedf42e3fe080fa8143619fddc7cfdc1ec0101a731d0b55c3f6e30aaaad",
    strip_prefix = "grpc-java-1.59.1",
    urls = ["https://github.com/grpc/grpc-java/archive/v1.59.1.tar.gz"],
)

http_archive(
    name = "rules_jvm_external",
    sha256 = "8ac1c5c2a8681c398883bb2cabc18f913337f165059f24e8c55714e05757761e",
    strip_prefix = "rules_jvm_external-5.3",
    url = "https://github.com/bazelbuild/rules_jvm_external/archive/5.3.tar.gz",
)

# Download rules_proto_grpc respository.
http_archive(
    name = "rules_proto_grpc",
    sha256 = "c0d718f4d892c524025504e67a5bfe83360b3a982e654bc71fed7514eb8ac8ad",
    strip_prefix = "rules_proto_grpc-4.6.0",
    urls = ["https://github.com/rules-proto-grpc/rules_proto_grpc/archive/4.6.0.tar.gz"],
)

http_archive(
    name = "rules_python",
    sha256 = "863ba0fa944319f7e3d695711427d9ad80ba92c6edd0b7c7443b84e904689539",
    strip_prefix = "rules_python-0.22.0",
    url = "https://github.com/bazelbuild/rules_python/releases/download/0.22.0/rules_python-0.22.0.tar.gz",
)

load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies", "go_repository")
load("@com_github_grpc_grpc//bazel:grpc_deps.bzl", "grpc_deps")
load("@com_google_googleapis//:repository_rules.bzl", "switched_rules_by_language")
load("@com_google_protobuf//:protobuf_deps.bzl", "protobuf_deps")
load("@io_bazel_rules_go//go:deps.bzl", "go_register_toolchains", "go_rules_dependencies")
load("@io_grpc_grpc_java//:repositories.bzl", "IO_GRPC_GRPC_JAVA_ARTIFACTS", "IO_GRPC_GRPC_JAVA_OVERRIDE_TARGETS", "grpc_java_repositories")
load("@rules_jvm_external//:defs.bzl", "maven_install")
load("@rules_jvm_external//:repositories.bzl", "rules_jvm_external_deps")
load("@rules_proto_grpc//:repositories.bzl", "rules_proto_grpc_toolchains")
load("@rules_proto_grpc//cpp:repositories.bzl", rules_proto_grpc_cpp_repos = "cpp_repos")
load("@rules_proto_grpc//doc:repositories.bzl", rules_proto_grpc_doc_repos = "doc_repos")
load("@rules_proto_grpc//go:repositories.bzl", rules_proto_grpc_go_repos = "go_repos")
load("@rules_proto_grpc//java:repositories.bzl", rules_proto_grpc_java_repos = "java_repos")
load("@rules_proto_grpc//python:repositories.bzl", rules_proto_grpc_python_repos = "python_repos")
load("@rules_python//python:pip.bzl", "pip_parse")
load("@rules_python//python:repositories.bzl", "py_repositories", "python_register_toolchains")

go_rules_dependencies()

# Install the Go toolchains. (https://github.com/bazelbuild/rules_go/blob/master/go/toolchains.rst#go-toolchain)
go_register_toolchains(
    version = "1.22.1",
)

gazelle_dependencies()

# For Go repository imports, we use the "module mode" form of the go_repository
# rule as encouraged by
# https://github.com/bazelbuild/bazel-gazelle/issues/549#issuecomment-516031215
#
# Downloads are not cached in version control mode.

go_repository(
    name = "com_github_bufbuild_protocompile",
    importpath = "github.com/bufbuild/protocompile",
    sum = "h1:Uu7WiSQ6Yj9DbkdnOe7U4mNKp58y9WDMKDn28/ZlunY=",
    version = "v0.6.0",
)

go_repository(
    name = "com_github_urfave_cli_v2",
    importpath = "github.com/urfave/cli/v2",
    sum = "h1:VAzn5oq403l5pHjc4OhD54+XGO9cdKVL/7lDjF+iKUs=",
    version = "v2.25.7",
)

go_repository(
    name = "com_github_xrash_smetrics",
    importpath = "github.com/xrash/smetrics",
    sum = "h1:bAn7/zixMGCfxrRTfdpNzjtPYqr8smhKouy9mxVdGPU=",
    version = "v0.0.0-20201216005158-039620a65673",
)

go_repository(
    name = "com_github_cpuguy83_go_md2man_v2",
    importpath = "github.com/cpuguy83/go-md2man/v2",
    sum = "h1:p1EgwI/C7NhT0JmVkwCD2ZBK8j4aeHQX2pMHHBfMQ6w=",
    version = "v2.0.2",
)

go_repository(
    name = "com_github_russross_blackfriday_v2",
    importpath = "github.com/russross/blackfriday/v2",
    sum = "h1:JIOH55/0cWyOuilr9/qlrm0BSXldqnqwMsf35Ld67mk=",
    version = "v2.1.0",
)

go_repository(
    name = "com_github_cenkalti_backoff_v4",
    importpath = "github.com/cenkalti/backoff/v4",
    sum = "h1:HN5dHm3WBOgndBH6E8V0q2jIYIR3s9yglV8k/+MN3u4=",
    version = "v4.2.0",
)

go_repository(
    name = "com_github_felixge_httpsnoop",
    importpath = "github.com/felixge/httpsnoop",
    sum = "h1:s/nj+GCswXYzN5v2DpNMuMQYe+0DDwt5WVCU6CWBdXk=",
    version = "v1.0.3",
)

go_repository(
    name = "com_github_fullstorydev_grpcurl",
    importpath = "github.com/fullstorydev/grpcurl",
    sum = "h1:xJWosq3BQovQ4QrdPO72OrPiWuGgEsxY8ldYsJbPrqI=",
    version = "v1.8.7",
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
    name = "com_github_golang_jwt_jwt_v5",
    importpath = "github.com/golang-jwt/jwt/v5",
    sum = "h1:hXPcSazn8wKOfSb9y2m1bdgUMlDxVDarxh3lJVbC6JE=",
    version = "v5.0.0-rc.2",
)

go_repository(
    name = "com_github_google_go_cmp",
    importpath = "github.com/google/go-cmp",
    sum = "h1:ofyhxvXcZhMsU5ulbFiLKl/XBFqE1GSq7atu8tAmTRI=",
    version = "v0.6.0",
)

go_repository(
    name = "org_golang_google_genproto_googleapis_api",
    importpath = "google.golang.org/genproto/googleapis/api",
    sum = "h1:s1w3X6gQxwrLEpxnLd/qXTVLgQE2yXwaOaoa6IlY/+o=",
    version = "v0.0.0-20231212172506-995d672761c0",
)

go_repository(
    name = "org_golang_google_genproto_googleapis_rpc",
    importpath = "google.golang.org/genproto/googleapis/rpc",
    sum = "h1:/jFB8jK5R3Sq3i/lmeZO0cATSzFfZaJq1J2Euan3XKU=",
    version = "v0.0.0-20231212172506-995d672761c0",
)

go_repository(
    name = "org_golang_google_genproto",
    importpath = "google.golang.org/genproto",
    sum = "h1:YJ5pD9rF8o9Qtta0Cmy9rdBwkSjrTCT6XTiUQVOtIos=",
    version = "v0.0.0-20231212172506-995d672761c0",
)

go_repository(
    name = "com_github_googleapis_gax_go",
    build_directives = [
        "gazelle:resolve proto go google/rpc/code.proto @org_golang_google_genproto_googleapis_rpc//code",  # keep
        "gazelle:resolve proto proto google/rpc/code.proto @com_google_googleapis//google/rpc:code_proto",  # keep
    ],
    importpath = "github.com/googleapis/gax-go/v2",
    sum = "h1:A+gCJKdRfqXkr+BIRGtZLibNXf0m1f9E4HG56etFpas=",
    version = "v2.12.0",
)

go_repository(
    name = "com_github_grpc_ecosystem_grpc_gateway_v2",
    importpath = "github.com/grpc-ecosystem/grpc-gateway/v2",
    sum = "h1:RtRsiaGvWxcwd8y3BiRZxsylPT8hLWZ5SPcfI+3IDNk=",
    version = "v2.18.0",
)

go_repository(
    name = "com_github_jhump_protoreflect",
    build_file_proto_mode = "legacy",
    importpath = "github.com/jhump/protoreflect",
    sum = "h1:7YppbATX94jEt9KLAc5hICx4h6Yt3SaavhQRsIUEHP0=",
    version = "v1.15.2",
)

go_repository(
    name = "com_github_jonboulle_clockwork",
    importpath = "github.com/jonboulle/clockwork",
    sum = "h1:p4Cf1aMWXnXAUh8lVfewRBx1zaTSYKrKMF2g3ST4RZ4=",
    version = "v0.4.0",
)

go_repository(
    name = "com_github_json_iterator_go",
    importpath = "github.com/json-iterator/go",
    sum = "h1:PV8peI4a0ysnczrg+LtxykD8LfKY9ML6u2jnxaEnrnM=",
    version = "v1.1.12",
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
    name = "com_github_matttproud_golang_protobuf_extensions",
    importpath = "github.com/matttproud/golang_protobuf_extensions",
    sum = "h1:mmDVorXM7PCGKw94cs5zkfA9PSy5pEvNWRP0ET0TIVo=",
    version = "v1.0.4",
)

go_repository(
    name = "com_github_modern_go_concurrent",
    importpath = "github.com/modern-go/concurrent",
    sum = "h1:TRLaZ9cD/w8PVh93nsPXa1VrQ6jlwL5oN8l14QlcNfg=",
    version = "v0.0.0-20180306012644-bacd9c7ef1dd",
)

go_repository(
    name = "com_github_modern_go_reflect2",
    importpath = "github.com/modern-go/reflect2",
    sum = "h1:xBagoLtFs94CBntxluKeaWgTMpvLxC4ur3nMaC9Gz0M=",
    version = "v1.0.2",
)

go_repository(
    name = "com_github_peterbourgon_ff",
    importpath = "github.com/peterbourgon/ff/v3",
    sum = "h1:0GNhbRhO9yHA4CC27ymskOsuRpmX0YQxwxM9UPiP6JM=",
    version = "v3.1.2",
)

go_repository(
    name = "com_github_prometheus_client_model",
    importpath = "github.com/prometheus/client_model",
    sum = "h1:UBgGFHqYdG/TPFD1B1ogZywDqEkwp3fBMvqdiQ7Xew4=",
    version = "v0.3.0",
)

go_repository(
    name = "com_github_prometheus_common",
    importpath = "github.com/prometheus/common",
    sum = "h1:Afz7EVRqGg2Mqqf4JuF9vdvp1pi220m55Pi9T2JnO4Q=",
    version = "v0.40.0",
)

go_repository(
    name = "com_github_prometheus_prom2json",
    importpath = "github.com/prometheus/prom2json",
    sum = "h1:heRKAGHWqm8N3IaRDDNBBJNVS6a9mLdsTlFhvOaNGb0=",
    version = "v1.3.2",
)

go_repository(
    name = "com_github_rs_zerolog",
    importpath = "github.com/rs/zerolog",
    sum = "h1:Zes4hju04hjbvkVkOhdl2HpZa+0PmVwigmo8XoORE5w=",
    version = "v1.29.0",
)

go_repository(
    name = "com_google_cloud_go",
    importpath = "cloud.google.com/go",
    sum = "h1:sdFPBr6xG9/wkBbfhmUz/JmZC7X6LavQgcrVINrKiVA=",
    version = "v0.110.2",
)

go_repository(
    name = "com_google_cloud_go_compute",
    importpath = "cloud.google.com/go/compute",
    sum = "h1:b1zWmYuuHz7gO9kDcM/EpHGr06UgsYNRpNJzI2kFiLM=",
    version = "v1.19.3",
)

go_repository(
    name = "io_opencensus_go",
    importpath = "go.opencensus.io",
    sum = "h1:y73uSU6J157QMP2kn2r30vwW1A2W2WFwSCGnAVxeaD0=",
    version = "v0.24.0",
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
    name = "io_opentelemetry_go_otel",
    importpath = "go.opentelemetry.io/otel",
    sum = "h1:/79Huy8wbf5DnIPhemGB+zEPVwnN6fuQybr/SRXa6hM=",
    version = "v1.14.0",
)

go_repository(
    name = "io_opentelemetry_go_otel_exporters_otlp_internal_retry",
    importpath = "go.opentelemetry.io/otel/exporters/otlp/internal/retry",
    sum = "h1:/fXHZHGvro6MVqV34fJzDhi7sHGpX3Ej/Qjmfn003ho=",
    version = "v1.14.0",
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
    name = "io_opentelemetry_go_otel_metric",
    importpath = "go.opentelemetry.io/otel/metric",
    sum = "h1:pHDQuLQOZwYD+Km0eb657A25NaRzy0a+eLyKfDXedEs=",
    version = "v0.37.0",
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
    name = "io_opentelemetry_go_otel_trace",
    importpath = "go.opentelemetry.io/otel/trace",
    sum = "h1:ofxdnzsNrGBYXbP7t7zpUK281+go5rF7dvdIZXF8gdQ=",
    version = "v1.11.1",
)

go_repository(
    name = "io_opentelemetry_go_proto_otlp",
    importpath = "go.opentelemetry.io/proto/otlp",
    sum = "h1:IVN6GR+mhC4s5yfcTbmzHYODqvWAp3ZedA2SJPI1Nnw=",
    version = "v0.19.0",
)

go_repository(
    name = "org_golang_google_api",
    importpath = "google.golang.org/api",
    sum = "h1:dP6Ef1VgOGqQ8eiv4GiY8RhmeyqzovcXBYPDUYG8Syo=",
    version = "v0.124.0",
)

go_repository(
    name = "org_golang_google_grpc",

    # Ignore proto files in this library, per
    # https://github.com/bazelbuild/rules_go/blob/master/go/dependencies.rst#grpc-dependencies.
    build_file_proto_mode = "disable",
    importpath = "google.golang.org/grpc",
    sum = "h1:TOvOcuXn30kRao+gfcvsebNEa5iZIiLkisYEkf7R7o0=",
    version = "v1.61.0",
)

go_repository(
    name = "org_golang_google_protobuf",
    importpath = "google.golang.org/protobuf",
    sum = "h1:kPPoIgf3TsEvrm0PFe15JQ+570QVxYzEvvHqChK+cng=",
    version = "v1.30.0",
)

go_repository(
    name = "org_golang_x_exp",
    importpath = "golang.org/x/exp",
    sum = "h1:Gvh4YaCaXNs6dKTlfgismwWZKyjVZXwOPfIyUaqU3No=",
    version = "v0.0.0-20231127185646-65229373498e",
)

go_repository(
    name = "org_golang_x_net",
    importpath = "golang.org/x/net",
    sum = "h1:AQyQV4dYCvJ7vGmJyKki9+PBdyvhkSd8EIx/qb0AYv4=",
    version = "v0.21.0",
)

go_repository(
    name = "org_golang_x_oauth2",
    importpath = "golang.org/x/oauth2",
    sum = "h1:6m3ZPmLEFdVxKKWnKq4VqZ60gutO35zm+zrAHVmHyDQ=",
    version = "v0.17.0",
)

go_repository(
    name = "org_golang_x_sync",
    importpath = "golang.org/x/sync",
    sum = "h1:w8s32wxx3sY+OjLlv9qltkLU5yvJzxjjgiHWLjdIcw4=",
    version = "v0.0.0-20220513210516-0976fa681c29",
)

go_repository(
    name = "org_golang_x_sys",
    importpath = "golang.org/x/sys",
    sum = "h1:25cE3gD+tdBA7lp7QfhuV+rJiE9YXTcS3VG1SqssI/Y=",
    version = "v0.17.0",
)

go_repository(
    name = "org_golang_x_text",
    importpath = "golang.org/x/text",
    sum = "h1:ScX5w1eTa3QqT8oi6+ziP7dTV1S2+ALU0bI+0zXKWiQ=",
    version = "v0.14.0",
)

go_repository(
    name = "com_github_vishvananda_netlink",
    importpath = "github.com/vishvananda/netlink",
    sum = "h1:FI3nwt5mKUM6g5S/DFJaE9/gL7977OSdddkUX0Avymg=",
    version = "v0.0.0-20231024175852-77df5d35f725",
)

go_repository(
    name = "com_github_vishvananda_netns",
    importpath = "github.com/vishvananda/netns",
    sum = "h1:mANKB5XLbNCInEF3mXxUb14rxYWTKd8JEbSP4KDJJWo=",
    version = "v0.0.0-20231013140742-fa017945dda9",
)

rules_jvm_external_deps()

load("@rules_jvm_external//:setup.bzl", "rules_jvm_external_setup")

rules_jvm_external_setup()

SCIP_JAVA_VERSION = "0.0.6"

http_archive(
    name = "scip_java",
    sha256 = "7013fa54a6c764999cb5dce0b8a04a983d6335d3fa73e474f4f229dcbf2ce30b",
    strip_prefix = "scip-java-{}".format(SCIP_JAVA_VERSION),
    url = "https://github.com/ciarand/scip-java/archive/refs/tags/v{}.zip".format(SCIP_JAVA_VERSION),
)

# When updating the artifacts or repositories in the maven_install, update
# maven_install.json along with it by running
#
# REPIN=1 bazel run @unpinned_maven//:pin
# Resolve and fetch grpc-java dependencies from a Maven respository.
maven_install(
    artifacts =
        IO_GRPC_GRPC_JAVA_ARTIFACTS +
        ["com.google.code.gson:gson:2.10.1",
        "com.google.googlejavaformat:google-java-format:1.17.0",
        "io.helidon.grpc:helidon-grpc-core:3.2.5",
        "org.bouncycastle:bcprov-jdk15on:1.70",
        "junit:junit:4.13.2",
        "org.mockito:mockito-core:4.5.1",
        ],
    fail_if_repin_required = True,
    generate_compat_repositories = True,
    override_targets = IO_GRPC_GRPC_JAVA_OVERRIDE_TARGETS,
    repositories = [
        "https://repo.maven.apache.org/maven2/",
    ],
)

load("@maven//:compat.bzl", "compat_repositories")

compat_repositories()

python_register_toolchains(
    name = "python3_9",
    python_version = "3.9",
)

load("@python3_9//:defs.bzl", "interpreter")

pip_parse(
    name = "python_deps",
    python_interpreter_target = interpreter,
    requirements_lock = "//:requirements.txt",
)

load("@python_deps//:requirements.bzl", "install_deps")

install_deps()

# Enable googleapis rules for the relevant languages.
switched_rules_by_language(
    name = "com_google_googleapis_imports",
    cc = True,
    java = True,
    python = True,
)

# Load dependencies needed to compile and test the grpc library.
grpc_deps(
    python_headers = "@python3_9//:python_headers",
)

load("@build_bazel_apple_support//lib:repositories.bzl", "apple_support_dependencies")
load("@build_bazel_rules_apple//apple:repositories.bzl", "apple_rules_dependencies")
load("@envoy_api//bazel:repositories.bzl", "api_dependencies")
load("@upb//bazel:workspace_deps.bzl", "upb_deps")

protobuf_deps()

upb_deps()

api_dependencies()

apple_rules_dependencies()

apple_support_dependencies()

grpc_java_repositories()

rules_proto_grpc_toolchains()

rules_proto_grpc_cpp_repos()

rules_proto_grpc_doc_repos()

rules_proto_grpc_go_repos()

rules_proto_grpc_java_repos()

rules_proto_grpc_python_repos()

py_repositories()

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
    version = "1.23.0",
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
    name = "com_github_google_uuid",
    importpath = "github.com/google/uuid",
    sum = "h1:NIvaJDMOsjHA8n1jAhLSgzrAzy1Hgr+hNrb57e+94F0=",
    version = "v1.6.0",
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
    sum = "h1:wu/KJm9KJwpfHWhkkZGohVC6KRrc1oJNr4jwtQMOQXw=",
    version = "v0.0.0-20240401170217-c3f982113cda",
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
    sum = "h1:fFA4WZxdEF4tXPZVKMLwD8oUnCTTo08duU7wxecdEvA=",
    version = "v0.1.13",
)

go_repository(
    name = "com_github_mattn_go_isatty",
    importpath = "github.com/mattn/go-isatty",
    sum = "h1:xfD0iDuEKnDkl03q4limB+vH+GxLEtL/jb4xVJSWWEY=",
    version = "v0.0.20",
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
    sum = "h1:1cU2KZkvPxNyfgEmhHAz/1A9Bz+llsdYzklWFzgp0r8=",
    version = "v1.33.0",
)

go_repository(
    name = "com_google_cloud_go",
    importpath = "cloud.google.com/go",
    sum = "h1:ZaGT6LiG7dBzi6zNOvVZwacaXlmf3lRqnC4DQzqyRQw=",
    version = "v0.112.2",
)

go_repository(
    name = "com_google_cloud_go_compute",
    importpath = "cloud.google.com/go/compute",
    sum = "h1:ZRpHJedLtTpKgr3RV1Fx23NuaAEN1Zfx9hw1u4aJdjU=",
    version = "v1.25.1",
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
    sum = "h1:4Pp6oUg3+e/6M4C0A/3kJ2VYa++dsWVTtGgLVj5xtHg=",
    version = "v0.49.0",
)

go_repository(
    name = "io_opentelemetry_go_contrib_instrumentation_net_http_otelhttp",
    importpath = "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp",
    sum = "h1:jq9TW8u3so/bN+JPT166wjOI6/vQPF6Xe7nMNIltagk=",
    version = "v0.49.0",
)

go_repository(
    name = "io_opentelemetry_go_otel",
    importpath = "go.opentelemetry.io/otel",
    sum = "h1:9BZoF3yMK/O1AafMiQTVu0YDj5Ea4hPhxCs7sGva+cg=",
    version = "v1.27.0",
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
    sum = "h1:t6wl9SPayj+c7lEIFgm4ooDBZVb01IhLB4InpomhRw8=",
    version = "v1.24.0",
)

go_repository(
    name = "io_opentelemetry_go_otel_exporters_otlp_otlptrace_otlptracegrpc",
    importpath = "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc",
    sum = "h1:Mw5xcxMwlqoJd97vwPxA8isEaIoxsta9/Q51+TTJLGE=",
    version = "v1.24.0",
)

go_repository(
    name = "io_opentelemetry_go_otel_metric",
    importpath = "go.opentelemetry.io/otel/metric",
    sum = "h1:hvj3vdEKyeCi4YaYfNjv2NUje8FqKqUY8IlF0FxV/ik=",
    version = "v1.27.0",
)

go_repository(
    name = "io_opentelemetry_go_otel_sdk",
    importpath = "go.opentelemetry.io/otel/sdk",
    sum = "h1:mlk+/Y1gLPLn84U4tI8d3GNJmGT/eXe3ZuOXN9kTWmI=",
    version = "v1.27.0",
)

go_repository(
    name = "io_opentelemetry_go_otel_sdk_metric",
    importpath = "go.opentelemetry.io/otel/sdk/metric",
    sum = "h1:5uGNOlpXi+Hbo/DRoI31BSb1v+OGcpv2NemcCrOL8gI=",
    version = "v1.27.0",
)

go_repository(
    name = "io_opentelemetry_go_otel_trace",
    importpath = "go.opentelemetry.io/otel/trace",
    sum = "h1:IqYb813p7cmbHk0a5y6pD5JPakbVfftRXABGt5/Rscw=",
    version = "v1.27.0",
)

go_repository(
    name = "io_opentelemetry_go_proto_otlp",
    importpath = "go.opentelemetry.io/proto/otlp",
    sum = "h1:2Di21piLrCqJ3U3eXGCTPHE9R8Nh+0uglSnOyxikMeI=",
    version = "v1.1.0",
)

go_repository(
    name = "org_golang_google_api",
    importpath = "google.golang.org/api",
    sum = "h1:/1OcMZGPmW1rX2LCu2CmGUD1KXK1+pfzxotxyRUCCdk=",
    version = "v0.172.0",
)

go_repository(
    name = "org_golang_google_grpc",
    # Ignore proto files in this library, per
    # https://github.com/bazelbuild/rules_go/blob/master/go/dependencies.rst#grpc-dependencies.
    build_file_proto_mode = "disable",
    importpath = "google.golang.org/grpc",
    sum = "h1:MUeiw1B2maTVZthpU5xvASfTh3LDbxHd6IJ6QQVU+xM=",
    version = "v1.63.2",
)

go_repository(
    name = "org_golang_google_protobuf",
    # Disable to prevent cyclical dependency error
    build_file_proto_mode = "disable",
    importpath = "google.golang.org/protobuf",
    sum = "h1:6xV6lTsCfpGD21XK49h7MhtcApnLqkfYgPcdHftf6hg=",
    version = "v1.34.2",
)

go_repository(
    name = "org_golang_x_exp",
    importpath = "golang.org/x/exp",
    sum = "h1:LoYXNGAShUG3m/ehNk4iFctuhGX/+R1ZpfJ4/ia80JM=",
    version = "v0.0.0-20240604190554-fc45aab8b7f8",
)

go_repository(
    name = "org_golang_x_net",
    importpath = "golang.org/x/net",
    sum = "h1:soB7SVo0PWrY4vPW/+ay0jKDNScG2X9wFeYlXIvJsOQ=",
    version = "v0.26.0",
)

go_repository(
    name = "org_golang_x_oauth2",
    importpath = "golang.org/x/oauth2",
    sum = "h1:09qnuIAgzdx1XplqJvW6CQqMCtGZykZWcXzPMPUusvI=",
    version = "v0.18.0",
)

go_repository(
    name = "org_golang_x_sync",
    importpath = "golang.org/x/sync",
    sum = "h1:YsImfSBoP9QPYL0xyKJPq0gcaJdG3rInoqxTWbfQu9M=",
    version = "v0.7.0",
)

go_repository(
    name = "org_golang_x_sys",
    importpath = "golang.org/x/sys",
    sum = "h1:rF+pYz3DAGSQAxAu1CbC7catZg4ebC4UIeIhKxBZvws=",
    version = "v0.21.0",
)

go_repository(
    name = "org_golang_x_text",
    importpath = "golang.org/x/text",
    sum = "h1:a94ExnEXNtEwYLGJSIUxnWoxoRz/ZcCsV63ROupILh4=",
    version = "v0.16.0",
)

go_repository(
    name = "com_github_vishvananda_netlink",
    importpath = "github.com/vishvananda/netlink",
    sum = "h1:Llsql0lnQEbHj0I1OuKyp8otXp0r3q0mPkuhwHfStVs=",
    version = "v1.2.1-beta.2",
)

go_repository(
    name = "com_github_vishvananda_netns",
    importpath = "github.com/vishvananda/netns",
    sum = "h1:4hwBBUfQCFe3Cym0ZtKyq7L16eZUtYKs+BaHDN6mAns=",
    version = "v0.0.0-20200728191858-db3c7e526aae",
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
        IO_GRPC_GRPC_JAVA_ARTIFACTS + [
            "com.google.code.gson:gson:2.10.1",
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
    name = "python3_11",
    python_version = "3.11",
)

load("@python3_11//:defs.bzl", "interpreter")

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
    python_headers = "@python3_11//:python_headers",
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

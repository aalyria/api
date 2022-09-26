# Copyright (c) Aalyria Technologies, Inc., and its affiliates.
# Confidential and Proprietary. All rights reserved.

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

# Download rules_proto_grpc respository.
http_archive(
    name = "rules_proto_grpc",
    sha256 = "507e38c8d95c7efa4f3b1c0595a8e8f139c885cb41a76cab7e20e4e67ae87731",
    strip_prefix = "rules_proto_grpc-4.1.1",
    urls = ["https://github.com/rules-proto-grpc/rules_proto_grpc/archive/4.1.1.tar.gz"],
)

load("@rules_proto_grpc//:repositories.bzl", "io_bazel_rules_go", "rules_proto_grpc_toolchains")

# Download the bazelbuild rules_go repository.
io_bazel_rules_go()

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
    name = "org_golang_google_grpc",

    # Ignore proto files in this library, per
    # https://github.com/bazelbuild/rules_go/blob/master/go/dependencies.rst#grpc-dependencies.
    build_file_proto_mode = "disable",
    importpath = "google.golang.org/grpc",
    sum = "h1:u+MLGgVf7vRdjEYZ8wDFhAVNmhkbJ5hmrA1LMWK1CAQ=",
    version = "v1.46.2",
)

# Register external dependencies needed by the Go rules.
go_rules_dependencies()

# Install the Go toolchains. (https://github.com/bazelbuild/rules_go/blob/master/go/toolchains.rst#go-toolchain)
go_register_toolchains(
    version = "1.17.1",
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
    sha256 = "d6cbf22cb5007af71b61c6be316a79397469c58c82a942552a62e708bce60964",
    strip_prefix = "grpc-1.46.3",
    urls = ["https://github.com/grpc/grpc/archive/refs/tags/v1.46.3.tar.gz"],
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

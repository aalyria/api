#!/usr/bin/env bash

set -euo pipefail

README_PATH=github/tools/nbictl/README.md

main() {
  local nbictl
  nbictl=$(readlink github/tools/nbictl/cmd/nbictl/nbictl_/nbictl)
  local readme
  readme=$(readlink "$README_PATH")

  if ! "$nbictl" readme | diff - "$readme";
  then
    echo >&2 "$README_PATH doesn't match the generated output from the nbictl."
    echo >&2 "Run the following command to regenerate:"
    echo >&2 "bazel run //github/tools/nbictl/cmd/nbictl -- readme > \$PWD/$README_PATH"
    return 1
  fi
}

main "$@"

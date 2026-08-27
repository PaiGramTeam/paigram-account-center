#!/usr/bin/env bash
# Regenerate Go and Python artifacts from the protobuf contracts.

set -Eeuo pipefail

CONTRACTS_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly CONTRACTS_ROOT
REPOSITORY_ROOT="$(cd "${CONTRACTS_ROOT}/.." && pwd)"
readonly REPOSITORY_ROOT

cd "${CONTRACTS_ROOT}"
buf lint
bash "${CONTRACTS_ROOT}/check-breaking.sh"
buf generate
find gen/go -type f -name '*.go' -print0 \
  | xargs -0 --no-run-if-empty gofmt -w
(cd gen/go && go mod tidy)
(cd "${REPOSITORY_ROOT}/sdks/python" && bash ./generate.sh)

#!/usr/bin/env bash
# Check contracts against the first committed platform v2 schema.

set -Eeuo pipefail

CONTRACTS_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly CONTRACTS_ROOT
REPOSITORY_ROOT="$(cd "${CONTRACTS_ROOT}/.." && pwd)"
readonly REPOSITORY_ROOT
baseline_commit="$(git -C "${REPOSITORY_ROOT}" log --diff-filter=A \
  --format=%H -- contracts/proto/platform/v2/types.proto | tail -n 1)"
readonly baseline_commit

if [[ -z "${baseline_commit}" ]]; then
  printf 'warning: the v2 contract is being bootstrapped; breaking checks start after its first commit.\n' >&2
  exit 0
fi

check_root="$(mktemp -d "${TMPDIR:-/tmp}/paigram-contract-breaking.XXXXXX")"
readonly check_root
cleanup() {
  local temp_root
  local resolved_check_root
  temp_root="$(realpath "${TMPDIR:-/tmp}")"
  resolved_check_root="$(realpath "${check_root}")"
  [[ "${resolved_check_root}" == "${temp_root}"/* ]] \
    || { printf 'error: unsafe temporary path: %s\n' "${resolved_check_root}" >&2; return; }
  rm -rf -- "${resolved_check_root}"
}
trap cleanup EXIT

git -C "${REPOSITORY_ROOT}" archive --format=tar "${baseline_commit}" contracts \
  | tar -xf - -C "${check_root}"
buf breaking "${CONTRACTS_ROOT}" \
  --against "${check_root}/contracts" \
  --config "${CONTRACTS_ROOT}/buf.breaking.yaml" \
  --against-config "${check_root}/contracts/buf.yaml"

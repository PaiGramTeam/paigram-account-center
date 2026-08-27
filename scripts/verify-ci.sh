#!/usr/bin/env bash
# Dispatch one isolated verification task for GitHub Actions.

set -Eeuo pipefail

if [[ "${CI:-}" != 'true' ]]; then
  printf 'error: verify-ci.sh is a CI-only entry point and requires CI=true\n' >&2
  exit 1
fi
if (( $# != 1 )); then
  printf 'usage: %s TASK\n' "$0" >&2
  exit 2
fi

REPOSITORY_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly REPOSITORY_ROOT
export REPOSITORY_ROOT

source "${REPOSITORY_ROOT}/scripts/ci/common.sh"
source "${REPOSITORY_ROOT}/scripts/ci/go.sh"
source "${REPOSITORY_ROOT}/scripts/ci/sdk.sh"
source "${REPOSITORY_ROOT}/scripts/ci/frontend.sh"
trap cleanup_temp_paths EXIT

case "$1" in
  contracts) verify_contracts ;;
  account-unit) verify_account_unit ;;
  platform-unit) verify_platform_unit ;;
  account-integration) verify_account_integration ;;
  platform-integration) verify_platform_integration ;;
  production-tracer) verify_production_tracer ;;
  sdk) verify_sdk ;;
  sdk-minimum) verify_sdk_minimum ;;
  frontend) verify_frontend ;;
  real-browser) verify_real_browser ;;
  *)
    printf 'error: unknown verification task: %s\n' "$1" >&2
    exit 2
    ;;
esac

printf "CI task '%s' completed successfully.\n" "$1"

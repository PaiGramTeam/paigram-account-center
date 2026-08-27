#!/usr/bin/env bash
# Go, contract, and repository verification tasks.

set -Eeuo pipefail

verify_contracts() {
  assert_go_formatted "${REPOSITORY_ROOT}/contracts/runtime/go"
  assert_go_formatted "${REPOSITORY_ROOT}/contracts/gen/go"
  bash "${REPOSITORY_ROOT}/contracts/generate.sh"
  (
    cd "${REPOSITORY_ROOT}/services/account-center"
    go run ./cmd/paigram openapi --out ../../contracts/openapi.json
  )
  (
    cd "${REPOSITORY_ROOT}/frontend"
    bun install --frozen-lockfile
    bun run openapi:gen
  )
  assert_tracked_paths_clean \
    contracts/gen/go \
    contracts/openapi.json \
    sdks/python/src/paigram_account_sdk/_generated \
    frontend/packages/shared-components/src/api/generated/schema.ts
  run_go_tests "${REPOSITORY_ROOT}/contracts/runtime/go" false '' ./...
  run_go_tests "${REPOSITORY_ROOT}/contracts/gen/go" true '' ./...
  assert_repository_clean
}

verify_account_unit() {
  if [[ -z "${PAI_TEST_DATABASE_DSN:-${PAI_DATABASE_DSN:-}}" ]]; then
    fail 'Account Center database tests require PAI_TEST_DATABASE_DSN or PAI_DATABASE_DSN'
  fi
  [[ "${PAI_REQUIRE_DATABASE_TESTS:-}" == 'true' ]] \
    || fail 'Account Center CI must set PAI_REQUIRE_DATABASE_TESTS=true'
  assert_go_formatted "${REPOSITORY_ROOT}/services/account-center"
  run_go_tests "${REPOSITORY_ROOT}/services/account-center" false '' -count=1 ./...
  (cd "${REPOSITORY_ROOT}/services/account-center" && go vet ./... && go build ./...)
  assert_repository_clean
}

verify_platform_unit() {
  assert_go_formatted "${REPOSITORY_ROOT}/services/platform-mihomo"
  run_go_tests "${REPOSITORY_ROOT}/services/platform-mihomo" false '' -count=1 ./...
  (cd "${REPOSITORY_ROOT}/services/platform-mihomo" && go vet ./... && go build ./...)
  assert_repository_clean
}

verify_account_integration() {
  run_go_tests "${REPOSITORY_ROOT}/services/account-center" false '' \
    -count=1 -tags=integration \
    '-skip=^(TestPythonSDKCallsProductionPlatformWithAccountIssuedTicket|TestPythonSDKDiscoversRuntimeRouteAcrossTLSListeners)$' \
    ./integration
  assert_repository_clean
}

verify_platform_integration() {
  run_go_tests "${REPOSITORY_ROOT}/services/platform-mihomo" false '' \
    -count=1 -tags=integration ./integration
  assert_repository_clean
}

verify_production_tracer() {
  local tests='TestPythonSDKCallsProductionPlatformWithAccountIssuedTicket,TestPythonSDKDiscoversRuntimeRouteAcrossTLSListeners'
  run_go_tests "${REPOSITORY_ROOT}/services/account-center" false "${tests}" \
    -count=1 -tags=integration \
    '-run=^(TestPythonSDKCallsProductionPlatformWithAccountIssuedTicket|TestPythonSDKDiscoversRuntimeRouteAcrossTLSListeners)$' \
    ./integration
  assert_repository_clean
}

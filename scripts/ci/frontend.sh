#!/usr/bin/env bash
# Frontend and real-browser verification tasks.

set -Eeuo pipefail

verify_frontend() {
  assert_no_skipped_test_markers "${REPOSITORY_ROOT}/frontend/tests"
  local report
  report="$(mktemp "${TMPDIR:-/tmp}/paigram-frontend-tests.XXXXXX.xml")"
  register_temp_path "${report}"
  (
    cd "${REPOSITORY_ROOT}/frontend"
    bun install --frozen-lockfile
    bun run format:check
    bun run lint
    bun run type-check
    bun test tests/unit --reporter=junit --reporter-outfile="${report}"
    bun run build:all
  )
  assert_junit_has_no_skipped_tests "${report}"
  assert_repository_clean
}

verify_real_browser() {
  assert_no_skipped_test_markers "${REPOSITORY_ROOT}/frontend/tests/e2e-real"
  local doubles
  local status
  set +e
  doubles="$(grep -R -n -E \
    'page\.route|route\.fulfill|setupWorker|setupServer|\bmsw\b' \
    "${REPOSITORY_ROOT}/frontend/tests/e2e-real" \
    "${REPOSITORY_ROOT}/frontend/playwright.real.config.ts")"
  status=$?
  set -e
  if (( status == 0 )); then
    printf '%s\n' "${doubles}" >&2
    fail 'Real-browser acceptance must not intercept or mock application requests'
  fi
  (( status == 1 )) || fail "Could not inspect real-browser test doubles (exit code ${status})"

  local browser="${PAI_E2E_BROWSER:-chromium}"
  [[ "${browser}" =~ ^(chromium|firefox|webkit)$ ]] \
    || fail 'PAI_E2E_BROWSER must be chromium, firefox, or webkit'
  local report
  report="$(mktemp "${TMPDIR:-/tmp}/paigram-real-browser.XXXXXX.xml")"
  register_temp_path "${report}"
  (
    cd "${REPOSITORY_ROOT}/frontend"
    bun install --frozen-lockfile
    bunx playwright install --with-deps "${browser}"
    PAI_E2E_JUNIT_PATH="${report}" bun run e2e:real
  )
  assert_junit_has_no_skipped_tests "${report}"
  assert_repository_clean
}

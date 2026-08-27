#!/usr/bin/env bash
# Shared verification helpers for CI and local checks.

set -Eeuo pipefail

declare -ag VERIFY_TEMP_PATHS=()

fail() {
  printf 'error: %s\n' "$*" >&2
  return 1
}

register_temp_path() {
  VERIFY_TEMP_PATHS+=("$1")
}

cleanup_temp_paths() {
  local path
  local temp_root
  temp_root="$(realpath "${TMPDIR:-/tmp}")"
  for path in "${VERIFY_TEMP_PATHS[@]}"; do
    [[ -e "${path}" ]] || continue
    path="$(realpath "${path}")"
    if [[ "${path}" != "${temp_root}"/* ]]; then
      printf 'error: refusing to remove path outside %s: %s\n' \
        "${temp_root}" "${path}" >&2
      continue
    fi
    rm -rf -- "${path}"
  done
}

assert_go_formatted() {
  local path="$1"
  local unformatted
  unformatted="$(find "${path}" -type f -name '*.go' -print0 \
    | xargs -0 --no-run-if-empty gofmt -l)"
  if [[ -n "${unformatted}" ]]; then
    printf '%s\n' "${unformatted}" >&2
    fail 'Go source files are not formatted'
  fi
}

assert_no_skipped_test_markers() {
  local matches
  local status
  set +e
  matches="$(grep -R -n -E \
    '(test|it|describe)\.(skip|todo)\(|pytest\.(skip|xfail)|pytest\.mark\.(skip|skipif|xfail)' \
    "$@")"
  status=$?
  set -e
  if (( status == 0 )); then
    printf '%s\n' "${matches}" >&2
    fail 'Committed tests must not be skipped or marked todo'
  fi
  (( status == 1 )) || fail "Could not inspect skipped tests (exit code ${status})"
}

assert_junit_has_no_skipped_tests() {
  local report="$1"
  uv run --no-project python - "${report}" <<'PY'
import sys
import xml.etree.ElementTree as ET

root = ET.parse(sys.argv[1]).getroot()
suites = [root] if root.tag == "testsuite" else list(root.iter("testsuite"))
if not suites:
    raise SystemExit("Test runner did not produce any JUnit test suites")
if sum(int(suite.get("tests", "0")) for suite in suites) == 0:
    raise SystemExit("Test runner did not execute any tests")
if any(int(suite.get(key, "0")) for suite in suites for key in ("skipped", "disabled")):
    raise SystemExit("Test runner reported skipped or disabled tests")
PY
}

run_go_tests() {
  local path="$1"
  local allow_no_tests="$2"
  local expected_tests="$3"
  shift 3

  local result_path
  local test_status
  result_path="$(mktemp "${TMPDIR:-/tmp}/paigram-go-test.XXXXXX.json")"
  register_temp_path "${result_path}"

  pushd "${path}" >/dev/null
  set +e
  go test -json "$@" 2>&1 | tee "${result_path}"
  test_status=${PIPESTATUS[0]}
  set -e
  popd >/dev/null
  (( test_status == 0 )) || fail "Go tests failed (exit code ${test_status})"

  local skipped_tests
  skipped_tests="$(grep -E '"Action":"skip".*"Test":"[^"]+"' "${result_path}" || true)"
  if [[ -n "${skipped_tests}" ]]; then
    printf '%s\n' "${skipped_tests}" >&2
    fail 'Go tests must not silently skip in CI'
  fi

  if [[ "${allow_no_tests}" != 'true' ]] \
    && ! grep -q -E '"Action":"run".*"Test":"[^"]+"' "${result_path}"; then
    fail 'Go test selection did not execute any tests'
  fi

  local expected_test
  IFS=',' read -r -a expected_array <<<"${expected_tests}"
  for expected_test in "${expected_array[@]}"; do
    [[ -z "${expected_test}" ]] && continue
    grep -q -F "\"Test\":\"${expected_test}\"" "${result_path}" \
      || fail "Expected Go test was not executed: ${expected_test}"
  done
}

assert_tracked_paths_clean() {
  local changes
  changes="$(git -C "${REPOSITORY_ROOT}" status --porcelain=v1 -- "$@")"
  if [[ -n "${changes}" ]]; then
    printf '%s\n' "${changes}" >&2
    fail 'Generated artifacts differ from the committed contract'
  fi
}

assert_repository_clean() {
  local changes
  changes="$(git -C "${REPOSITORY_ROOT}" status --porcelain=v1 --untracked-files=all)"
  if [[ -n "${changes}" ]]; then
    printf '%s\n' "${changes}" >&2
    fail 'Verification modified the checkout or the repository was not clean'
  fi
}

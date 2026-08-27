#!/usr/bin/env bash
# Python SDK verification tasks.

set -Eeuo pipefail

verify_sdk() {
  assert_no_skipped_test_markers "${REPOSITORY_ROOT}/sdks/python/tests"
  local report
  report="$(mktemp "${TMPDIR:-/tmp}/paigram-sdk-tests.XXXXXX.xml")"
  register_temp_path "${report}"
  (
    cd "${REPOSITORY_ROOT}/sdks/python"
    uv lock --check
    uv sync --all-groups --locked
    uv run ruff check .
    uv run ruff format --check .
    uv run mypy
    uv run pytest -q --junitxml="${report}"
    uv build
  )
  assert_junit_has_no_skipped_tests "${report}"
  assert_repository_clean
}

verify_sdk_minimum() {
  local environment_path
  local python_path
  local report
  local test_file
  local -a minimum_tests=()
  for test_file in "${REPOSITORY_ROOT}"/sdks/python/tests/test_*.py; do
    [[ "${test_file}" == */test_recovery_tracer.py ]] && continue
    minimum_tests+=("${test_file}")
  done
  (( ${#minimum_tests[@]} > 0 )) || fail 'No public SDK tests were selected'

  environment_path="$(mktemp -d "${TMPDIR:-/tmp}/paigram-sdk-min.XXXXXX")"
  register_temp_path "${environment_path}"
  python_path="${environment_path}/bin/python"
  report="$(mktemp "${TMPDIR:-/tmp}/paigram-sdk-min-tests.XXXXXX.xml")"
  register_temp_path "${report}"
  (
    cd "${REPOSITORY_ROOT}/sdks/python"
    uv venv --python 3.10 "${environment_path}"
    uv pip install --python "${python_path}" --no-deps .
    uv pip install --python "${python_path}" \
      'grpcio==1.81.1' 'httpx==0.28.0' 'protobuf==6.33.5' \
      'pytest==9.0.0' 'pytest-asyncio==1.3.0'
    uv run --no-project --python "${python_path}" \
      python -m pytest -q --junitxml="${report}" "${minimum_tests[@]}"
  )
  assert_junit_has_no_skipped_tests "${report}"
  uv run --no-project --python "${python_path}" python -c \
    "import asyncio; from paigram_account_sdk import PaiGramAccountClient; client = PaiGramAccountClient(account_http_url='https://account.invalid', account_grpc_target='localhost:50051', client_id='smoke', client_secret='smoke'); asyncio.run(client.close())"
  assert_repository_clean
}

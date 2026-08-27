#!/usr/bin/env bash
# Regenerate the Python protobuf modules.

set -Eeuo pipefail

SDK_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly SDK_ROOT
cd "${SDK_ROOT}"
uv run python tools/generate_contracts.py

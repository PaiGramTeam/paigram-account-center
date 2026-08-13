from __future__ import annotations

import asyncio
import base64
import hashlib
import hmac
import json
import logging
import math
import os
import struct
import sys
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any, cast
from urllib.parse import urlparse

import httpx

from .client import PaiGramAccountClient
from .models import CredentialStatus

logger = logging.getLogger(__name__)


@dataclass(frozen=True, slots=True)
class TracerConfig:
    account_grpc_server_name: str
    account_ca_file: Path
    platform_service_key: str
    platform_ca_file: Path
    user_email: str
    user_password: str
    totp_secret: str
    external_user_id: str
    client_id: str
    client_secret: str
    expected_binding_ref: str
    expected_account_key: str
    expected_profile_ref: str
    expected_authkey_prefix: str = ""
    timeout_seconds: float = 15.0


def _load_config(path: Path) -> TracerConfig:
    payload = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(payload, dict):
        raise ValueError("tracer config must be a JSON object")
    required = {
        "account_grpc_server_name",
        "account_ca_file",
        "platform_service_key",
        "platform_ca_file",
        "user_email",
        "user_password",
        "totp_secret",
        "external_user_id",
        "client_id",
        "client_secret",
        "expected_binding_ref",
        "expected_account_key",
        "expected_profile_ref",
    }
    allowed = required | {"expected_authkey_prefix", "timeout_seconds"}
    if set(payload) - allowed or not required.issubset(payload):
        raise ValueError("tracer config has missing or unknown fields")
    raw_timeout = payload.get("timeout_seconds", 15.0)
    if isinstance(raw_timeout, bool) or not isinstance(raw_timeout, (int, float)):
        raise ValueError("timeout_seconds must be a number")
    timeout = float(raw_timeout)
    if not math.isfinite(timeout) or timeout <= 0 or timeout > 60:
        raise ValueError("timeout_seconds must be between 0 and 60")
    string_values: dict[str, str] = {}
    for name in required | {"expected_authkey_prefix"}:
        value = payload.get(name, "")
        if not isinstance(value, str) or (name in required and not value.strip()):
            raise ValueError(f"tracer config field must be a non-empty string: {name}")
        string_values[name] = value

    account_ca_file = Path(string_values["account_ca_file"])
    platform_ca_file = Path(string_values["platform_ca_file"])
    certificate_paths = [account_ca_file, platform_ca_file]
    for certificate_path in certificate_paths:
        if not certificate_path.is_absolute():
            raise ValueError(f"CA certificate path must be absolute: {certificate_path}")
        if not certificate_path.is_file():
            raise ValueError(f"CA certificate file does not exist: {certificate_path}")

    return TracerConfig(
        account_grpc_server_name=string_values["account_grpc_server_name"],
        account_ca_file=account_ca_file,
        platform_service_key=string_values["platform_service_key"],
        platform_ca_file=platform_ca_file,
        user_email=string_values["user_email"],
        user_password=string_values["user_password"],
        totp_secret=string_values["totp_secret"],
        external_user_id=string_values["external_user_id"],
        client_id=string_values["client_id"],
        client_secret=string_values["client_secret"],
        expected_binding_ref=string_values["expected_binding_ref"],
        expected_account_key=string_values["expected_account_key"],
        expected_profile_ref=string_values["expected_profile_ref"],
        expected_authkey_prefix=string_values["expected_authkey_prefix"],
        timeout_seconds=timeout,
    )


def _totp_code(secret: str, now: int | None = None) -> str:
    normalized = secret.strip().replace(" ", "").upper()
    padding = "=" * ((8 - len(normalized) % 8) % 8)
    key = base64.b32decode(normalized + padding, casefold=True)
    counter = int(time.time() if now is None else now) // 30
    digest = hmac.new(key, struct.pack(">Q", counter), hashlib.sha1).digest()
    offset = digest[-1] & 0x0F
    value = struct.unpack(">I", digest[offset : offset + 4])[0] & 0x7FFFFFFF
    return f"{value % 1_000_000:06d}"


def _response_data(response: httpx.Response) -> dict[str, Any]:
    response.raise_for_status()
    payload = response.json()
    if not isinstance(payload, dict) or not isinstance(payload.get("data"), dict):
        raise ValueError("Account Center returned an invalid response envelope")
    return cast(dict[str, Any], payload["data"])


def _trusted_account_url() -> str:
    raw = os.environ.get("PAI_RECOVERY_ACCOUNT_HTTP_URL", "")
    parsed = urlparse(raw)
    if (
        parsed.scheme not in {"http", "https"}
        or parsed.hostname not in {"127.0.0.1", "localhost", "::1"}
        or not parsed.port
        or parsed.username is not None
        or parsed.password is not None
        or parsed.path not in {"", "/"}
        or parsed.query
        or parsed.fragment
    ):
        raise ValueError("trusted Account HTTP URL must use loopback with an explicit port")
    return raw.rstrip("/")


def _trusted_account_grpc_target() -> str:
    target = os.environ.get("PAI_RECOVERY_ACCOUNT_GRPC_TARGET", "")
    if not target.startswith(("127.0.0.1:", "localhost:", "[::1]:")):
        raise ValueError("trusted Account gRPC target must use loopback")
    return target


async def _verify_restored_login(
    config: TracerConfig,
    account_http_url: str,
    transport: httpx.AsyncBaseTransport | None = None,
) -> None:
    async with httpx.AsyncClient(
        base_url=account_http_url,
        timeout=config.timeout_seconds,
        transport=transport,
    ) as client:
        challenge = _response_data(
            await client.post("/api/v1/auth/login", json={"email": config.user_email, "password": config.user_password})
        )
        if challenge.get("requires_totp") is not True:
            raise ValueError("restored account did not require TOTP")

        login = _response_data(
            await client.post(
                "/api/v1/auth/login",
                json={
                    "email": config.user_email,
                    "password": config.user_password,
                    "totp_code": _totp_code(config.totp_secret),
                },
            )
        )
        access_token = str(login.get("access_token", ""))
        if not access_token:
            raise ValueError("restored account login did not return an access token")
        overview = _response_data(
            await client.get("/api/v1/me/security/overview", headers={"Authorization": f"Bearer {access_token}"})
        )
        if overview.get("two_factor_enabled") is not True:
            raise ValueError("restored account no longer reports TOTP as enabled")


async def _verify_restored_sdk_path(config: TracerConfig, account_http_url: str, account_grpc_target: str) -> None:
    async with PaiGramAccountClient(
        account_http_url=account_http_url,
        account_grpc_target=account_grpc_target,
        account_grpc_server_name=config.account_grpc_server_name,
        account_root_certificates=config.account_ca_file.read_bytes(),
        platform_root_certificates={config.platform_service_key: config.platform_ca_file.read_bytes()},
        client_id=config.client_id,
        client_secret=config.client_secret,
        timeout=config.timeout_seconds,
    ) as client:
        bindings = await client.list_bindings(config.external_user_id, platform="mihomo")
        binding = next((item for item in bindings if item.binding_ref == config.expected_binding_ref), None)
        if binding is None or binding.account_key != config.expected_account_key:
            raise ValueError("restored binding was not returned by the SDK")

        status = await client.get_credential_status(external_user_id=config.external_user_id, binding=binding)
        if status.status is not CredentialStatus.ACTIVE:
            raise ValueError("restored credential is not active")

        profiles = await client.list_profiles(external_user_id=config.external_user_id, binding=binding)
        profile = next((item for item in profiles if item.profile_ref == config.expected_profile_ref), None)
        if profile is None:
            raise ValueError("restored profile was not returned by the SDK")

        authkey = await client.get_auth_key(
            external_user_id=config.external_user_id,
            binding=binding,
            profile_ref=profile.profile_ref,
        )
        if not authkey.value or (
            config.expected_authkey_prefix and not authkey.value.startswith(config.expected_authkey_prefix)
        ):
            raise ValueError("restored credential did not issue the expected AuthKey")


async def main(config_path: Path) -> None:
    config = _load_config(config_path)
    account_http_url = _trusted_account_url()
    account_grpc_target = _trusted_account_grpc_target()
    await _verify_restored_login(config, account_http_url)
    await _verify_restored_sdk_path(config, account_http_url, account_grpc_target)
    print(json.dumps({"status": "passed", "binding_ref": config.expected_binding_ref}))


def cli() -> None:
    if len(sys.argv) != 2:
        raise SystemExit("usage: recovery_tracer.py <private-config.json>")
    try:
        asyncio.run(main(Path(sys.argv[1])))
    except Exception:
        logger.exception("Recovery tracer failed")
        raise


if __name__ == "__main__":
    cli()

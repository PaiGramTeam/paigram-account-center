import json
from pathlib import Path
from types import SimpleNamespace
from typing import Any

import httpx
import pytest

import paigram_account_sdk.recovery_tracer as recovery_tracer
from paigram_account_sdk import CredentialStatus
from paigram_account_sdk.recovery_tracer import (
    _load_config,
    _response_data,
    _totp_code,
    _trusted_account_grpc_target,
    _trusted_account_url,
    _verify_restored_login,
    _verify_restored_sdk_path,
)


def tracer_payload(tmp_path: Path) -> dict[str, object]:
    account_ca = tmp_path / "account-ca.pem"
    platform_ca = tmp_path / "platform-ca.pem"
    account_ca.write_text("account-ca", encoding="utf-8")
    platform_ca.write_text("platform-ca", encoding="utf-8")
    return {
        "account_grpc_server_name": "account-bot.internal",
        "account_ca_file": str(account_ca),
        "platform_service_key": "platform-mihomo-service",
        "platform_ca_file": str(platform_ca),
        "user_email": "recovery@example.test",
        "user_password": "private-password",
        "totp_secret": "JBSWY3DPEHPK3PXP",
        "external_user_id": "external-recovery-user",
        "client_id": "recovery-bot",
        "client_secret": "private-client-secret",
        "expected_binding_ref": "binding-ref",
        "expected_account_key": "account-key",
        "expected_profile_ref": "profile-ref",
    }


def write_config(path: Path, payload: object) -> None:
    path.write_text(json.dumps(payload), encoding="utf-8")


def test_load_config_accepts_optional_defaults(tmp_path: Path) -> None:
    config_path = tmp_path / "tracer.json"
    write_config(config_path, tracer_payload(tmp_path))

    config = _load_config(config_path)

    assert config.expected_authkey_prefix == ""
    assert config.timeout_seconds == 15.0


@pytest.mark.parametrize(
    "mutate",
    [
        lambda payload: payload.pop("client_secret"),
        lambda payload: payload.update({"unknown": "value"}),
        lambda payload: payload.update({"client_id": 42}),
        lambda payload: payload.update({"timeout_seconds": 0}),
        lambda payload: payload.update({"timeout_seconds": True}),
        lambda payload: payload.update({"timeout_seconds": float("nan")}),
    ],
)
def test_load_config_rejects_invalid_contract(tmp_path: Path, mutate) -> None:  # type: ignore[no-untyped-def]
    config_path = tmp_path / "tracer.json"
    payload = tracer_payload(tmp_path)
    mutate(payload)
    write_config(config_path, payload)

    with pytest.raises(ValueError):
        _load_config(config_path)


def test_load_config_requires_existing_ca_files(tmp_path: Path) -> None:
    config_path = tmp_path / "tracer.json"
    payload = tracer_payload(tmp_path)
    payload["account_ca_file"] = str(tmp_path / "missing.pem")
    write_config(config_path, payload)

    with pytest.raises(ValueError, match="CA certificate file does not exist"):
        _load_config(config_path)


def test_totp_matches_rfc_6238_vector_truncated_to_six_digits() -> None:
    assert _totp_code("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", now=59) == "287082"


def test_trusted_targets_require_explicit_loopback(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("PAI_RECOVERY_ACCOUNT_HTTP_URL", "http://127.0.0.1:18443")
    monkeypatch.setenv("PAI_RECOVERY_ACCOUNT_GRPC_TARGET", "127.0.0.1:15051")

    assert _trusted_account_url() == "http://127.0.0.1:18443"
    assert _trusted_account_grpc_target() == "127.0.0.1:15051"


@pytest.mark.parametrize(
    ("name", "value", "loader"),
    [
        ("PAI_RECOVERY_ACCOUNT_HTTP_URL", "https://account.example.test:443", _trusted_account_url),
        ("PAI_RECOVERY_ACCOUNT_GRPC_TARGET", "account.example.test:15051", _trusted_account_grpc_target),
    ],
)
def test_trusted_targets_reject_remote_or_plaintext(
    monkeypatch: pytest.MonkeyPatch, name: str, value: str, loader
) -> None:  # type: ignore[no-untyped-def]
    monkeypatch.setenv(name, value)

    with pytest.raises(ValueError):
        loader()


def test_response_data_rejects_invalid_envelope() -> None:
    request = httpx.Request("GET", "https://account.example.test/api/v1/me")
    response = httpx.Response(200, json={"data": []}, request=request)

    with pytest.raises(ValueError, match="invalid response envelope"):
        _response_data(response)


@pytest.mark.asyncio
async def test_restored_login_verifies_totp_decryption_and_security_overview(tmp_path: Path) -> None:
    config_path = tmp_path / "tracer.json"
    write_config(config_path, tracer_payload(tmp_path))
    config = _load_config(config_path)
    observed_totp = ""

    def handle(request: httpx.Request) -> httpx.Response:
        nonlocal observed_totp
        if request.url.path == "/api/v1/auth/login":
            body = json.loads(request.content)
            if "totp_code" not in body:
                return httpx.Response(200, json={"data": {"requires_totp": True}})
            observed_totp = body["totp_code"]
            return httpx.Response(200, json={"data": {"access_token": "restored-access-token"}})
        assert request.headers["authorization"] == "Bearer restored-access-token"
        return httpx.Response(200, json={"data": {"two_factor_enabled": True}})

    await _verify_restored_login(config, "http://127.0.0.1:18080", httpx.MockTransport(handle))

    assert len(observed_totp) == 6


@pytest.mark.asyncio
async def test_restored_sdk_path_reads_binding_profile_and_issues_authkey(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    config_path = tmp_path / "tracer.json"
    payload = tracer_payload(tmp_path)
    payload["expected_authkey_prefix"] = "recovery-authkey-"
    write_config(config_path, payload)
    config = _load_config(config_path)
    calls: list[str] = []

    class FakeClient:
        def __init__(self, **kwargs: Any) -> None:
            assert kwargs["account_http_url"] == "http://127.0.0.1:18080"
            assert kwargs["account_grpc_target"] == "127.0.0.1:15051"

        async def __aenter__(self) -> "FakeClient":
            return self

        async def __aexit__(self, *_args: object) -> None:
            return None

        async def list_bindings(self, external_user_id: str, *, platform: str):
            calls.append("bindings")
            assert external_user_id == config.external_user_id
            assert platform == "mihomo"
            return (SimpleNamespace(binding_ref=config.expected_binding_ref, account_key=config.expected_account_key),)

        async def get_credential_status(self, **_kwargs: object):
            calls.append("status")
            return SimpleNamespace(status=CredentialStatus.ACTIVE)

        async def list_profiles(self, **_kwargs: object):
            calls.append("profiles")
            return (SimpleNamespace(profile_ref=config.expected_profile_ref),)

        async def get_auth_key(self, **kwargs: object):
            calls.append("authkey")
            assert kwargs["profile_ref"] == config.expected_profile_ref
            return SimpleNamespace(value="recovery-authkey-issued")

    monkeypatch.setattr(recovery_tracer, "PaiGramAccountClient", FakeClient)

    await _verify_restored_sdk_path(config, "http://127.0.0.1:18080", "127.0.0.1:15051")

    assert calls == ["bindings", "status", "profiles", "authkey"]

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

import grpc
import httpx
import pytest

from paigram_account_sdk import (
    AuthenticationError,
    CredentialError,
    CredentialStatus,
    NotFoundError,
    PaiGramAccountClient,
    PlatformAccountStatus,
    PlatformEndpoint,
    ServiceUnavailableError,
)
from paigram_account_sdk._generated.account.v1 import bot_access_pb2, bot_access_pb2_grpc
from paigram_account_sdk._generated.platform.v1 import platform_pb2, platform_pb2_grpc


class BotAccessService(bot_access_pb2_grpc.BotAccessServiceServicer):
    async def ResolveBotUser(self, request, context):  # type: ignore[no-untyped-def, no-untyped-call]
        metadata = dict(context.invocation_metadata())
        if metadata.get("authorization") != "Bearer machine-token":
            await context.abort(grpc.StatusCode.UNAUTHENTICATED, "missing machine token")
        if metadata.get("x-request-id") != "request-123":
            await context.abort(grpc.StatusCode.INVALID_ARGUMENT, "missing request ID")
        if request.external_user_id == "missing":
            await context.abort(grpc.StatusCode.NOT_FOUND, "user not found")
        if request.external_user_id == "inactive":
            await context.abort(grpc.StatusCode.FAILED_PRECONDITION, "binding is inactive")
        return bot_access_pb2.ResolveBotUserResponse(
            user_id=42,
            bot_id="paigram",
            external_user_id=request.external_user_id,
            external_username="traveler",
        )

    async def ListAccessibleBindings(self, request, context):  # type: ignore[no-untyped-def, no-untyped-call]
        await require_account_metadata(context)
        return bot_access_pb2.ListAccessibleBindingsResponse(bindings=[binding_proto()])

    async def IssueServiceTicket(self, request, context):  # type: ignore[no-untyped-def, no-untyped-call]
        await require_account_metadata(context)
        if request.audience != "mihomo-service" or list(request.requested_scopes) not in (
            ["mihomo.credential.read_meta"],
            ["mihomo.credential.refresh"],
            ["mihomo.credential.delete"],
        ):
            await context.abort(grpc.StatusCode.INVALID_ARGUMENT, "unexpected ticket request")
        return bot_access_pb2.IssueServiceTicketResponse(
            ticket="service-ticket",
            audience=request.audience,
            binding=binding_proto(),
        )


class PlatformService(platform_pb2_grpc.PlatformServiceServicer):
    async def DescribePlatform(self, request, context):  # type: ignore[no-untyped-def, no-untyped-call]
        await require_request_id(context)
        return platform_pb2.DescribePlatformResponse(
            platform_key="mihomo",
            display_name="Mihomo",
            service_audience="mihomo-service",
            supported_actions=[
                "mihomo.credential.read_meta",
                "mihomo.credential.refresh",
                "mihomo.credential.delete",
            ],
            version="v1",
        )

    async def GetCredentialSummary(self, request, context):  # type: ignore[no-untyped-def, no-untyped-call]
        await require_request_id(context)
        if request.service_ticket != "service-ticket" or request.platform_account_id != "binding_7_10001":
            await context.abort(grpc.StatusCode.PERMISSION_DENIED, "invalid service ticket")
        return platform_pb2.GetCredentialSummaryResponse(
            platform_account_id=request.platform_account_id,
            status=platform_pb2.CREDENTIAL_STATUS_ACTIVE,
            devices=[
                platform_pb2.DeviceSummary(
                    device_id="device-id",
                    device_name="Phone",
                    is_valid=True,
                )
            ],
            profiles=[
                platform_pb2.ProfileSummary(
                    id=99,
                    platform_account_id=request.platform_account_id,
                    game_biz="hk4e_global",
                    player_id="10001",
                    nickname="Traveler",
                    level=60,
                    is_default=True,
                )
            ],
        )

    async def RefreshCredential(self, request, context):  # type: ignore[no-untyped-def, no-untyped-call]
        await require_request_id(context)
        return platform_pb2.RefreshCredentialResponse(status=platform_pb2.CREDENTIAL_STATUS_ACTIVE)

    async def DeleteCredential(self, request, context):  # type: ignore[no-untyped-def, no-untyped-call]
        await require_request_id(context)
        return platform_pb2.DeleteCredentialResponse(success=True)


class PreconditionPlatformService(PlatformService):
    async def GetCredentialSummary(self, request, context):  # type: ignore[no-untyped-def, no-untyped-call]
        await require_request_id(context)
        await context.abort(grpc.StatusCode.FAILED_PRECONDITION, "credential is inactive")


async def require_request_id(context):  # type: ignore[no-untyped-def, no-untyped-call]
    metadata = dict(context.invocation_metadata())
    if metadata.get("x-request-id") != "request-123":
        await context.abort(grpc.StatusCode.INVALID_ARGUMENT, "missing request ID")


async def require_account_metadata(context):  # type: ignore[no-untyped-def, no-untyped-call]
    metadata = dict(context.invocation_metadata())
    if metadata.get("authorization") != "Bearer machine-token":
        await context.abort(grpc.StatusCode.UNAUTHENTICATED, "missing machine token")
    await require_request_id(context)


def binding_proto():  # type: ignore[no-untyped-def]
    return bot_access_pb2.PlatformAccountBinding(
        id=7,
        user_id=42,
        platform="mihomo",
        platform_service_key="mihomo",
        platform_account_id="binding_7_10001",
        display_name="Traveler",
        status=bot_access_pb2.PLATFORM_ACCOUNT_STATUS_ACTIVE,
    )


@asynccontextmanager
async def account_server() -> AsyncIterator[str]:
    server = grpc.aio.server()
    bot_access_pb2_grpc.add_BotAccessServiceServicer_to_server(BotAccessService(), server)
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    try:
        yield f"127.0.0.1:{port}"
    finally:
        await server.stop(grace=None)


@asynccontextmanager
async def platform_server(service: PlatformService | None = None) -> AsyncIterator[str]:
    server = grpc.aio.server()
    platform_pb2_grpc.add_PlatformServiceServicer_to_server(service or PlatformService(), server)
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    try:
        yield f"127.0.0.1:{port}"
    finally:
        await server.stop(grace=None)


def token_transport() -> httpx.MockTransport:
    def issue_token(request: httpx.Request) -> httpx.Response:
        assert request.headers["content-type"].startswith("application/x-www-form-urlencoded")
        assert request.headers["x-request-id"] == "request-123"
        assert b"client_secret=secret" in request.content
        return httpx.Response(
            200,
            json={
                "access_token": "machine-token",
                "token_type": "Bearer",
                "expires_in": 3600,
                "scope": "bot.access.read bot.access.issue_ticket",
            },
        )

    return httpx.MockTransport(issue_token)


@pytest.mark.asyncio
async def test_resolve_user_returns_public_model() -> None:
    async with (
        account_server() as target,
        PaiGramAccountClient(
            account_http_url="https://account.example.test",
            account_grpc_target=target,
            account_grpc_secure=False,
            client_id="paigram",
            client_secret="secret",
            platform_endpoints={"mihomo": PlatformEndpoint(target="127.0.0.1:1", secure=False)},
            http_transport=token_transport(),
        ) as client,
    ):
        user = await client.resolve_user("telegram:10001", request_id="request-123")

    assert user.user_id == 42
    assert user.bot_id == "paigram"
    assert user.external_user_id == "telegram:10001"
    assert user.external_username == "traveler"


@pytest.mark.asyncio
async def test_resolve_user_maps_grpc_not_found() -> None:
    async with (
        account_server() as target,
        PaiGramAccountClient(
            account_http_url="https://account.example.test",
            account_grpc_target=target,
            account_grpc_secure=False,
            client_id="paigram",
            client_secret="secret",
            platform_endpoints={},
            http_transport=token_transport(),
        ) as client,
    ):
        with pytest.raises(NotFoundError, match="user not found"):
            await client.resolve_user("missing", request_id="request-123")


@pytest.mark.asyncio
async def test_list_bindings_returns_stable_public_models() -> None:
    async with (
        account_server() as target,
        PaiGramAccountClient(
            account_http_url="https://account.example.test",
            account_grpc_target=target,
            account_grpc_secure=False,
            client_id="paigram",
            client_secret="secret",
            platform_endpoints={},
            http_transport=token_transport(),
        ) as client,
    ):
        bindings = await client.list_bindings(
            "telegram:10001",
            platform="mihomo",
            request_id="request-123",
        )

    assert len(bindings) == 1
    assert bindings[0].platform_service_key == "mihomo"
    assert bindings[0].platform_account_id == "binding_7_10001"
    assert bindings[0].status is PlatformAccountStatus.ACTIVE


@pytest.mark.asyncio
async def test_get_credential_summary_orchestrates_service_ticket() -> None:
    async with (
        account_server() as account_target,
        platform_server() as platform_target,
        PaiGramAccountClient(
            account_http_url="https://account.example.test",
            account_grpc_target=account_target,
            account_grpc_secure=False,
            client_id="paigram",
            client_secret="secret",
            platform_endpoints={"mihomo": PlatformEndpoint(target=platform_target, secure=False)},
            http_transport=token_transport(),
        ) as client,
    ):
        binding = (await client.list_bindings("telegram:10001", request_id="request-123"))[0]
        summary = await client.get_credential_summary(
            external_user_id="telegram:10001",
            binding=binding,
            request_id="request-123",
        )

    assert summary.platform_account_id == "binding_7_10001"
    assert summary.status is CredentialStatus.ACTIVE
    assert summary.devices[0].device_name == "Phone"
    assert summary.profiles[0].nickname == "Traveler"


@pytest.mark.asyncio
async def test_refresh_and_delete_credentials_use_authorized_platform_calls() -> None:
    async with (
        account_server() as account_target,
        platform_server() as platform_target,
        PaiGramAccountClient(
            account_http_url="https://account.example.test",
            account_grpc_target=account_target,
            account_grpc_secure=False,
            client_id="paigram",
            client_secret="secret",
            platform_endpoints={"mihomo": PlatformEndpoint(target=platform_target, secure=False)},
            http_transport=token_transport(),
        ) as client,
    ):
        binding = (await client.list_bindings("telegram:10001", request_id="request-123"))[0]
        refreshed = await client.refresh_credential(
            external_user_id="telegram:10001",
            binding=binding,
            request_id="request-123",
        )
        deleted = await client.delete_credential(
            external_user_id="telegram:10001",
            binding=binding,
            request_id="request-123",
        )

    assert refreshed.status is CredentialStatus.ACTIVE
    assert deleted is True


@pytest.mark.asyncio
async def test_invalid_client_maps_oauth_error() -> None:
    transport = httpx.MockTransport(
        lambda request: httpx.Response(
            401,
            json={"error": "invalid_client", "error_description": "client authentication failed"},
        )
    )
    async with PaiGramAccountClient(
        account_http_url="https://account.example.test",
        account_grpc_target="127.0.0.1:1",
        account_grpc_secure=False,
        client_id="paigram",
        client_secret="wrong",
        platform_endpoints={},
        http_transport=transport,
    ) as client:
        with pytest.raises(AuthenticationError, match="client authentication failed"):
            await client.resolve_user("telegram:10001", request_id="request-123")


@pytest.mark.asyncio
async def test_oauth_server_failure_is_retryable() -> None:
    transport = httpx.MockTransport(
        lambda request: httpx.Response(
            503,
            json={"error": "server_error", "error_description": "temporarily unavailable"},
        )
    )
    async with PaiGramAccountClient(
        account_http_url="https://account.example.test",
        account_grpc_target="127.0.0.1:1",
        account_grpc_secure=False,
        client_id="paigram",
        client_secret="secret",
        platform_endpoints={},
        http_transport=transport,
    ) as client:
        with pytest.raises(ServiceUnavailableError, match="temporarily unavailable"):
            await client.resolve_user("telegram:10001", request_id="request-123")


@pytest.mark.asyncio
async def test_account_precondition_maps_service_unavailable() -> None:
    async with (
        account_server() as target,
        PaiGramAccountClient(
            account_http_url="https://account.example.test",
            account_grpc_target=target,
            account_grpc_secure=False,
            client_id="paigram",
            client_secret="secret",
            platform_endpoints={},
            http_transport=token_transport(),
        ) as client,
    ):
        with pytest.raises(ServiceUnavailableError, match="binding is inactive"):
            await client.resolve_user("inactive", request_id="request-123")


@pytest.mark.asyncio
async def test_platform_precondition_maps_credential_state() -> None:
    async with (
        account_server() as account_target,
        platform_server(PreconditionPlatformService()) as platform_target,
        PaiGramAccountClient(
            account_http_url="https://account.example.test",
            account_grpc_target=account_target,
            account_grpc_secure=False,
            client_id="paigram",
            client_secret="secret",
            platform_endpoints={"mihomo": PlatformEndpoint(target=platform_target, secure=False)},
            http_transport=token_transport(),
        ) as client,
    ):
        binding = (await client.list_bindings("telegram:10001", request_id="request-123"))[0]
        with pytest.raises(CredentialError, match="credential is inactive"):
            await client.get_credential_summary(
                external_user_id="telegram:10001",
                binding=binding,
                request_id="request-123",
            )

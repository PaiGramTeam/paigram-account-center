import asyncio
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from urllib.parse import parse_qs

import grpc
import httpx
import pytest

import paigram_account_sdk.client as client_module
from paigram_account_sdk import (
    AuthenticationError,
    CredentialError,
    CredentialStatus,
    InvalidRequestError,
    NotFoundError,
    PaiGramAccountClient,
    PlatformAccountStatus,
    RateLimitError,
    ServiceUnavailableError,
    TransportError,
)
from paigram_account_sdk._generated.account.v1 import bot_access_pb2, bot_access_pb2_grpc
from paigram_account_sdk._generated.mihomo.v2 import runtime_pb2, runtime_pb2_grpc
from paigram_account_sdk._generated.platform.v2 import types_pb2


class BotAccessService(bot_access_pb2_grpc.BotAccessServiceServicer):
    def __init__(self, runtime_target: str = "127.0.0.1:1", runtime_server_name: str = "runtime.internal") -> None:
        self.runtime_target = runtime_target
        self.runtime_server_name = runtime_server_name

    async def StartEntryIdentityLink(self, request, context):  # type: ignore[no-untyped-def, no-untyped-call]
        await require_account_metadata(context)
        if request.external_subject != "external-42" or request.external_username != "traveler":
            await context.abort(grpc.StatusCode.INVALID_ARGUMENT, "unexpected identity")
        return bot_access_pb2.StartEntryIdentityLinkResponse(
            approval_url="https://account.example.test/entry-identity-link#challenge=opaque",
            issuer="urn:paigram:entry:telegram",
            masked_subject="ex*******42",
            bot_id="paigram",
            bot_display_name="PaiGram",
        )

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

    async def GetPlatformRuntimeRoute(self, request, context):  # type: ignore[no-untyped-def, no-untyped-call]
        await require_account_metadata(context)
        if request.platform_service_key == "unconfigured":
            await context.abort(grpc.StatusCode.FAILED_PRECONDITION, "runtime route is not configured")
        if request.platform_service_key != "platform-mihomo-service":
            await context.abort(grpc.StatusCode.NOT_FOUND, "runtime route not found")
        return bot_access_pb2.GetPlatformRuntimeRouteResponse(
            platform_key="mihomo",
            platform_service_key=request.platform_service_key,
            runtime_endpoint=self.runtime_target,
            runtime_server_name=self.runtime_server_name,
            service_audience="platform-mihomo-service",
            supported_actions=[
                "mihomo.authkey.issue",
                "mihomo.credential.validate",
                "mihomo.device.read",
                "mihomo.profile.read",
                "mihomo.status.read",
            ],
        )

    async def IssueServiceTicket(self, request, context):  # type: ignore[no-untyped-def, no-untyped-call]
        await require_account_metadata(context)
        allowed_actions = {
            "mihomo.authkey.issue",
            "mihomo.credential.validate",
            "mihomo.device.read",
            "mihomo.profile.read",
            "mihomo.status.read",
        }
        if request.binding_ref != "binding-7" or request.requested_action not in allowed_actions:
            await context.abort(grpc.StatusCode.INVALID_ARGUMENT, "unexpected ticket request")
        if request.requested_action == "mihomo.authkey.issue" and request.profile_ref != "profile-99":
            await context.abort(grpc.StatusCode.INVALID_ARGUMENT, "profile scope is required")
        return bot_access_pb2.IssueServiceTicketResponse(
            ticket="service-ticket",
            audience="platform-mihomo-service",
            binding=binding_proto(),
        )


class MihomoRuntimeService(runtime_pb2_grpc.MihomoRuntimeServiceServicer):
    async def DescribePlatform(self, request, context):  # type: ignore[no-untyped-def, no-untyped-call]
        await require_request_id(context)
        return runtime_pb2.DescribePlatformResponse(
            platform_key="mihomo",
            display_name="Mihomo",
            service_audience="platform-mihomo-service",
            supported_actions=[
                "mihomo.authkey.issue",
                "mihomo.credential.validate",
                "mihomo.device.read",
                "mihomo.profile.read",
                "mihomo.status.read",
            ],
            contract_version="v2",
        )

    async def GetStatus(self, request, context):  # type: ignore[no-untyped-def, no-untyped-call]
        await require_platform_request(request.resource, context)
        return runtime_pb2.GetStatusResponse(status=types_pb2.CREDENTIAL_STATUS_ACTIVE)

    async def ValidateCredential(self, request, context):  # type: ignore[no-untyped-def, no-untyped-call]
        await require_platform_request(request.resource, context)
        return runtime_pb2.ValidateCredentialResponse(status=types_pb2.CREDENTIAL_STATUS_ACTIVE)

    async def ListProfiles(self, request, context):  # type: ignore[no-untyped-def, no-untyped-call]
        await require_platform_request(request.resource, context)
        return runtime_pb2.ListProfilesResponse(
            snapshot=types_pb2.ProfileSnapshot(profiles=[profile_proto(request.resource.account_key)], complete=True)
        )

    async def GetPrimaryProfile(self, request, context):  # type: ignore[no-untyped-def, no-untyped-call]
        await require_platform_request(request.resource, context)
        return runtime_pb2.GetPrimaryProfileResponse(profile=profile_proto(request.resource.account_key))

    async def GetAuthKey(self, request, context):  # type: ignore[no-untyped-def, no-untyped-call]
        await require_platform_request(request.resource, context)
        if request.profile_ref != "profile-99":
            await context.abort(grpc.StatusCode.INVALID_ARGUMENT, "unexpected profile_ref")
        return runtime_pb2.GetAuthKeyResponse(authkey="authkey-profile-99")

    async def GetDevice(self, request, context):  # type: ignore[no-untyped-def, no-untyped-call]
        await require_platform_request(request.resource, context)
        if request.device_ref != "device-1":
            await context.abort(grpc.StatusCode.INVALID_ARGUMENT, "unexpected device_ref")
        return runtime_pb2.GetDeviceResponse(
            device=types_pb2.DeviceSummary(device_ref="device-1", device_name="Phone", is_valid=True)
        )


class PreconditionMihomoRuntimeService(MihomoRuntimeService):
    async def GetStatus(self, request, context):  # type: ignore[no-untyped-def, no-untyped-call]
        await require_platform_request(request.resource, context)
        await context.abort(grpc.StatusCode.FAILED_PRECONDITION, "credential is inactive")


class RateLimitedMihomoRuntimeService(MihomoRuntimeService):
    async def GetStatus(self, request, context):  # type: ignore[no-untyped-def, no-untyped-call]
        await require_platform_request(request.resource, context)
        await context.abort(grpc.StatusCode.RESOURCE_EXHAUSTED, "platform rate limit exceeded")


class MismatchedDescriptorRuntimeService(MihomoRuntimeService):
    async def DescribePlatform(self, request, context):  # type: ignore[no-untyped-def, no-untyped-call]
        await require_request_id(context)
        return runtime_pb2.DescribePlatformResponse(
            platform_key="mihomo",
            display_name="Mihomo",
            service_audience="unexpected-audience",
            supported_actions=["mihomo.status.read"],
            contract_version="v2",
        )


async def require_request_id(context):  # type: ignore[no-untyped-def, no-untyped-call]
    metadata = dict(context.invocation_metadata())
    if metadata.get("x-request-id") != "request-123":
        await context.abort(grpc.StatusCode.INVALID_ARGUMENT, "missing request ID")


async def require_account_metadata(context):  # type: ignore[no-untyped-def, no-untyped-call]
    metadata = dict(context.invocation_metadata())
    if metadata.get("authorization") != "Bearer machine-token":
        await context.abort(grpc.StatusCode.UNAUTHENTICATED, "missing machine token")
    await require_request_id(context)


async def require_platform_request(resource, context):  # type: ignore[no-untyped-def, no-untyped-call]
    metadata = dict(context.invocation_metadata())
    if metadata.get("authorization") != "Bearer service-ticket":
        await context.abort(grpc.StatusCode.UNAUTHENTICATED, "missing service ticket")
    await require_request_id(context)
    if resource.binding_ref != "binding-7" or resource.account_key != "account-10001":
        await context.abort(grpc.StatusCode.PERMISSION_DENIED, "invalid binding resource")


def binding_proto() -> bot_access_pb2.PlatformAccountBinding:
    return bot_access_pb2.PlatformAccountBinding(
        binding_ref="binding-7",
        platform="mihomo",
        platform_service_key="platform-mihomo-service",
        account_key="account-10001",
        display_name="Traveler",
        status=bot_access_pb2.PLATFORM_ACCOUNT_STATUS_ACTIVE,
        generation=4,
    )


def profile_proto(account_key: str) -> types_pb2.ProfileSummary:
    return types_pb2.ProfileSummary(
        profile_ref="profile-99",
        account_key=account_key,
        game_biz="hk4e_global",
        player_id="10001",
        nickname="Traveler",
        level=60,
        is_default=True,
    )


@asynccontextmanager
async def account_server(
    platform_target: str = "127.0.0.1:1", runtime_server_name: str = "runtime.internal"
) -> AsyncIterator[str]:
    server = grpc.aio.server()
    bot_access_pb2_grpc.add_BotAccessServiceServicer_to_server(
        BotAccessService(platform_target, runtime_server_name), server
    )
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    try:
        yield f"127.0.0.1:{port}"
    finally:
        await server.stop(grace=None)


@asynccontextmanager
async def platform_server(service: MihomoRuntimeService | None = None) -> AsyncIterator[str]:
    server = grpc.aio.server()
    runtime_pb2_grpc.add_MihomoRuntimeServiceServicer_to_server(service or MihomoRuntimeService(), server)
    port = server.add_insecure_port("127.0.0.1:0")
    await server.start()
    try:
        yield f"127.0.0.1:{port}"
    finally:
        await server.stop(grace=None)


def token_transport(requested_scopes: list[str] | None = None) -> httpx.MockTransport:
    def issue_token(request: httpx.Request) -> httpx.Response:
        assert request.headers["content-type"].startswith("application/x-www-form-urlencoded")
        assert request.headers["x-request-id"] == "request-123"
        assert b"client_secret=secret" in request.content
        scope = parse_qs(request.content.decode("ascii"))["scope"][0]
        assert scope in {"bot.access.read bot.access.issue_ticket", "bot.access.link_identity"}
        if requested_scopes is not None:
            requested_scopes.append(scope)
        return httpx.Response(
            200,
            json={
                "access_token": "machine-token",
                "token_type": "Bearer",
                "expires_in": 3600,
                "scope": "bot.access.read bot.access.issue_ticket bot.access.link_identity",
            },
        )

    return httpx.MockTransport(issue_token)


def client_for(account_target: str) -> PaiGramAccountClient:
    return PaiGramAccountClient(
        account_http_url="https://account.example.test",
        account_grpc_target=account_target,
        client_id="paigram",
        client_secret="secret",
        http_transport=token_transport(),
    )


@pytest.mark.asyncio
async def test_start_entry_identity_link_returns_public_domain_model() -> None:
    requested_scopes: list[str] = []
    async with (
        account_server() as account_target,
        PaiGramAccountClient(
            account_http_url="https://account.example.test",
            account_grpc_target=account_target,
            client_id="paigram",
            client_secret="secret",
            http_transport=token_transport(requested_scopes),
        ) as client,
    ):
        link = await client.start_entry_identity_link(
            "external-42", external_username="traveler", request_id="request-123"
        )

    assert link.issuer == "urn:paigram:entry:telegram"
    assert link.masked_subject == "ex*******42"
    assert link.bot_id == "paigram"
    assert link.approval_url.endswith("#challenge=opaque")
    assert requested_scopes == ["bot.access.link_identity"]


@pytest.fixture(autouse=True)
def use_insecure_unit_test_channels(monkeypatch: pytest.MonkeyPatch) -> None:
    def create_channel(target: str, *, root_certificates: bytes | None, server_name: str | None) -> grpc.aio.Channel:
        del root_certificates, server_name
        return grpc.aio.insecure_channel(target)

    monkeypatch.setattr(client_module, "_create_secure_channel", create_channel)


@pytest.mark.asyncio
async def test_platform_route_rejects_missing_server_name() -> None:
    async with account_server(runtime_server_name="") as account_target, client_for(account_target) as client:
        with pytest.raises(ServiceUnavailableError, match="invalid platform runtime route"):
            await client.describe_platform("platform-mihomo-service", request_id="request-123")


@pytest.mark.asyncio
async def test_platform_route_precondition_is_service_configuration_failure() -> None:
    async with account_server() as account_target, client_for(account_target) as client:
        with pytest.raises(ServiceUnavailableError, match="runtime route is not configured"):
            await client.describe_platform("unconfigured", request_id="request-123")


@pytest.mark.asyncio
async def test_close_waits_for_inflight_route_creation(monkeypatch: pytest.MonkeyPatch) -> None:
    client = PaiGramAccountClient(
        account_http_url="https://account.example.test",
        account_grpc_target="127.0.0.1:1",
        client_id="paigram",
        client_secret="secret",
        http_transport=token_transport(),
    )
    route_started = asyncio.Event()
    release_route = asyncio.Event()

    async def delayed_route(*_args: object, **_kwargs: object) -> object:
        route_started.set()
        await release_route.wait()
        return bot_access_pb2.GetPlatformRuntimeRouteResponse(
            platform_key="mihomo",
            platform_service_key="platform-mihomo-service",
            runtime_endpoint="127.0.0.1:1",
            runtime_server_name="runtime.internal",
            service_audience="platform-mihomo-service",
            supported_actions=["mihomo.status.read"],
        )

    monkeypatch.setattr(client, "_account_call", delayed_route)
    route_task = asyncio.create_task(client._platform_stub("platform-mihomo-service", "request-123"))
    await route_started.wait()
    close_task = asyncio.create_task(client.close())
    await asyncio.sleep(0)
    assert not close_task.done()

    release_route.set()
    await route_task
    await close_task
    with pytest.raises(TransportError, match="client is closed"):
        await client._platform_stub("platform-mihomo-service", "request-123")


@pytest.mark.asyncio
async def test_platform_descriptor_must_match_authenticated_route() -> None:
    async with (
        platform_server(MismatchedDescriptorRuntimeService()) as platform_target,
        account_server(platform_target) as account_target,
        client_for(account_target) as client,
    ):
        with pytest.raises(ServiceUnavailableError, match="descriptor does not match registered route"):
            await client.describe_platform("platform-mihomo-service", request_id="request-123")


def test_client_requires_account_grpc_target() -> None:
    with pytest.raises(InvalidRequestError, match="gRPC target is required"):
        PaiGramAccountClient(
            account_http_url="https://account.example.test",
            account_grpc_target="",
            client_id="paigram",
            client_secret="secret",
            http_transport=token_transport(),
        )


@pytest.mark.asyncio
async def test_resolve_user_returns_public_model() -> None:
    async with account_server() as target, client_for(target) as client:
        user = await client.resolve_user("telegram:10001", request_id="request-123")

    assert user.user_id == 42
    assert user.bot_id == "paigram"
    assert user.external_user_id == "telegram:10001"
    assert user.external_username == "traveler"


@pytest.mark.asyncio
async def test_resolve_user_maps_grpc_not_found() -> None:
    async with account_server() as target, client_for(target) as client:
        with pytest.raises(NotFoundError, match="user not found"):
            await client.resolve_user("missing", request_id="request-123")


@pytest.mark.asyncio
async def test_list_bindings_returns_stable_public_models() -> None:
    async with account_server() as target, client_for(target) as client:
        bindings = await client.list_bindings("telegram:10001", platform="mihomo", request_id="request-123")

    assert len(bindings) == 1
    assert bindings[0].binding_ref == "binding-7"
    assert bindings[0].platform_service_key == "platform-mihomo-service"
    assert bindings[0].account_key == "account-10001"
    assert bindings[0].generation == 4
    assert bindings[0].status is PlatformAccountStatus.ACTIVE


@pytest.mark.asyncio
async def test_platform_runtime_methods_use_v2_resources_and_public_models() -> None:
    async with (
        platform_server() as platform_target,
        account_server(platform_target) as account_target,
        client_for(account_target) as client,
    ):
        binding = (await client.list_bindings("telegram:10001", request_id="request-123"))[0]
        descriptor = await client.describe_platform(binding.platform_service_key, request_id="request-123")
        status_result = await client.get_credential_status(
            external_user_id="telegram:10001", binding=binding, request_id="request-123"
        )
        validation = await client.validate_credential(
            external_user_id="telegram:10001", binding=binding, request_id="request-123"
        )
        profiles = await client.list_profiles(
            external_user_id="telegram:10001", binding=binding, request_id="request-123"
        )
        primary = await client.get_primary_profile(
            external_user_id="telegram:10001", binding=binding, request_id="request-123"
        )
        authkey = await client.get_auth_key(
            external_user_id="telegram:10001",
            binding=binding,
            profile_ref="profile-99",
            request_id="request-123",
        )
        device = await client.get_device(
            external_user_id="telegram:10001",
            binding=binding,
            device_ref="device-1",
            request_id="request-123",
        )

    assert descriptor.contract_version == "v2"
    assert status_result.status is CredentialStatus.ACTIVE
    assert validation.status is CredentialStatus.ACTIVE
    assert profiles[0].profile_ref == "profile-99"
    assert profiles[0].nickname == "Traveler"
    assert primary is not None and primary.is_default
    assert authkey.value == "authkey-profile-99"
    assert device.device_ref == "device-1"
    assert device.device_name == "Phone"


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
        client_id="paigram",
        client_secret="wrong",
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
        client_id="paigram",
        client_secret="secret",
        http_transport=transport,
    ) as client:
        with pytest.raises(ServiceUnavailableError, match="temporarily unavailable"):
            await client.resolve_user("telegram:10001", request_id="request-123")


@pytest.mark.asyncio
async def test_oauth_rate_limit_has_distinct_error() -> None:
    transport = httpx.MockTransport(
        lambda request: httpx.Response(
            429,
            json={"error": "temporarily_unavailable", "error_description": "token rate limit exceeded"},
        )
    )
    async with PaiGramAccountClient(
        account_http_url="https://account.example.test",
        account_grpc_target="127.0.0.1:1",
        client_id="paigram",
        client_secret="secret",
        http_transport=transport,
    ) as client:
        with pytest.raises(RateLimitError, match="token rate limit exceeded"):
            await client.resolve_user("telegram:10001", request_id="request-123")


@pytest.mark.asyncio
async def test_account_precondition_maps_credential_state() -> None:
    async with account_server() as target, client_for(target) as client:
        with pytest.raises(CredentialError, match="binding is inactive"):
            await client.resolve_user("inactive", request_id="request-123")


@pytest.mark.asyncio
async def test_platform_precondition_maps_credential_state() -> None:
    async with (
        platform_server(PreconditionMihomoRuntimeService()) as platform_target,
        account_server(platform_target) as account_target,
        client_for(account_target) as client,
    ):
        binding = (await client.list_bindings("telegram:10001", request_id="request-123"))[0]
        with pytest.raises(CredentialError, match="credential is inactive"):
            await client.get_credential_status(
                external_user_id="telegram:10001",
                binding=binding,
                request_id="request-123",
            )


@pytest.mark.asyncio
async def test_platform_rate_limit_has_distinct_error() -> None:
    async with (
        platform_server(RateLimitedMihomoRuntimeService()) as platform_target,
        account_server(platform_target) as account_target,
        client_for(account_target) as client,
    ):
        binding = (await client.list_bindings("telegram:10001", request_id="request-123"))[0]
        with pytest.raises(RateLimitError, match="platform rate limit exceeded"):
            await client.get_credential_status(
                external_user_id="telegram:10001",
                binding=binding,
                request_id="request-123",
            )

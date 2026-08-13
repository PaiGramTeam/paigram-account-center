from __future__ import annotations

import asyncio
import logging
from collections.abc import Awaitable, Callable, Mapping
from datetime import datetime, timezone
from types import TracebackType
from typing import TypeVar, cast
from uuid import uuid4

import grpc
import httpx
from google.protobuf.json_format import MessageToDict
from google.protobuf.timestamp_pb2 import Timestamp

from paigram_account_sdk._generated.account.v1 import bot_access_pb2, bot_access_pb2_grpc
from paigram_account_sdk._generated.mihomo.v2 import runtime_pb2, runtime_pb2_grpc
from paigram_account_sdk._generated.platform.v2 import types_pb2

from ._auth import _ClientCredentialsTokenProvider
from .errors import (
    AccountSDKError,
    AuthenticationError,
    AuthorizationError,
    ConflictError,
    CredentialError,
    DeadlineExceededError,
    InvalidRequestError,
    NotFoundError,
    RateLimitError,
    ServiceUnavailableError,
    TransportError,
)
from .models import (
    AuthKey,
    BotUser,
    CredentialStatus,
    CredentialStatusResult,
    DeviceSummary,
    EntryIdentityLink,
    PlatformAccountStatus,
    PlatformBinding,
    PlatformDescriptor,
    ProfileSummary,
    ServiceTicket,
    ValidationResult,
)

logger = logging.getLogger(__name__)
T = TypeVar("T")


class PaiGramAccountClient:
    def __init__(
        self,
        *,
        account_http_url: str,
        account_grpc_target: str,
        account_grpc_server_name: str | None = None,
        client_id: str,
        client_secret: str,
        account_root_certificates: bytes | None = None,
        platform_root_certificates: Mapping[str, bytes | None] | None = None,
        timeout: float = 10.0,
        http_transport: httpx.AsyncBaseTransport | None = None,
        request_id_factory: Callable[[], str] | None = None,
    ) -> None:
        if timeout <= 0:
            logger.warning("PaiGram Account SDK rejected a non-positive timeout")
            raise InvalidRequestError("timeout must be greater than zero")
        if not account_grpc_target:
            logger.warning("PaiGram Account SDK rejected an empty account gRPC target")
            raise InvalidRequestError("gRPC target is required")
        self._timeout = timeout
        self._platform_root_certificates = dict(platform_root_certificates or {})
        self._http_client = httpx.AsyncClient(
            base_url=account_http_url.rstrip("/"),
            timeout=timeout,
            transport=http_transport,
        )
        self._tokens = _ClientCredentialsTokenProvider(self._http_client, client_id, client_secret, timeout)
        self._account_channel = _create_secure_channel(
            account_grpc_target,
            root_certificates=account_root_certificates,
            server_name=account_grpc_server_name,
        )
        self._account = bot_access_pb2_grpc.BotAccessServiceStub(self._account_channel)  # type: ignore[no-untyped-call]
        self._platform_channels: dict[str, grpc.aio.Channel] = {}
        self._platform_stubs: dict[str, runtime_pb2_grpc.MihomoRuntimeServiceStub] = {}
        self._platform_routes: dict[str, bot_access_pb2.GetPlatformRuntimeRouteResponse] = {}
        self._platform_route_lock = asyncio.Lock()
        self._request_id_factory = request_id_factory or (lambda: uuid4().hex)
        self._closed = False

    async def __aenter__(self) -> PaiGramAccountClient:
        return self

    async def __aexit__(
        self,
        exc_type: type[BaseException] | None,
        exc_value: BaseException | None,
        traceback: TracebackType | None,
    ) -> None:
        await self.close()

    async def close(self) -> None:
        async with self._platform_route_lock:
            if self._closed:
                return
            self._closed = True
            resources = (self._http_client, self._account_channel, *self._platform_channels.values())
        first_error: BaseException | None = None
        for resource in resources:
            try:
                await resource.aclose() if isinstance(resource, httpx.AsyncClient) else await resource.close()
            except BaseException as error:
                logger.error("PaiGram Account SDK failed to close a transport", exc_info=error)
                if first_error is None:
                    first_error = error
        if first_error is not None:
            raise first_error

    async def resolve_user(self, external_user_id: str, *, request_id: str | None = None) -> BotUser:
        self._ensure_open()
        if not external_user_id:
            logger.warning("PaiGram Account SDK rejected an empty external_user_id")
            raise InvalidRequestError("external_user_id is required")
        resolved_request_id = self._resolve_request_id(request_id)
        response = cast(
            bot_access_pb2.ResolveBotUserResponse,
            await self._account_call(
                self._account.ResolveBotUser,
                bot_access_pb2.ResolveBotUserRequest(external_user_id=external_user_id),
                resolved_request_id,
            ),
        )
        return BotUser(
            user_id=response.user_id,
            bot_id=response.bot_id,
            external_user_id=response.external_user_id,
            external_username=response.external_username,
        )

    async def start_entry_identity_link(
        self,
        external_subject: str,
        external_username: str = "",
        *,
        request_id: str | None = None,
    ) -> EntryIdentityLink:
        self._ensure_open()
        if not external_subject:
            logger.warning("PaiGram Account SDK rejected an empty external_subject")
            raise InvalidRequestError("external_subject is required")
        resolved_request_id = self._resolve_request_id(request_id)
        response = cast(
            bot_access_pb2.StartEntryIdentityLinkResponse,
            await self._account_call(
                self._account.StartEntryIdentityLink,
                bot_access_pb2.StartEntryIdentityLinkRequest(
                    external_subject=external_subject,
                    external_username=external_username,
                ),
                resolved_request_id,
                required_scopes=("bot.access.link_identity",),
            ),
        )
        return EntryIdentityLink(
            approval_url=response.approval_url,
            issuer=response.issuer,
            masked_subject=response.masked_subject,
            bot_id=response.bot_id,
            bot_display_name=response.bot_display_name,
            expires_at=_datetime_from_timestamp(response.expires_at),
        )

    async def list_bindings(
        self,
        external_user_id: str,
        platform: str = "",
        *,
        request_id: str | None = None,
    ) -> tuple[PlatformBinding, ...]:
        self._ensure_open()
        if not external_user_id:
            logger.warning("PaiGram Account SDK rejected an empty external_user_id")
            raise InvalidRequestError("external_user_id is required")
        resolved_request_id = self._resolve_request_id(request_id)
        response = cast(
            bot_access_pb2.ListAccessibleBindingsResponse,
            await self._account_call(
                self._account.ListAccessibleBindings,
                bot_access_pb2.ListAccessibleBindingsRequest(
                    external_user_id=external_user_id,
                    platform=platform,
                ),
                resolved_request_id,
            ),
        )
        return tuple(_binding_from_proto(binding) for binding in response.bindings)

    async def _issue_service_ticket(
        self,
        *,
        external_user_id: str,
        binding_ref: str,
        requested_action: str,
        profile_ref: str,
        request_id: str,
    ) -> ServiceTicket:
        response = cast(
            bot_access_pb2.IssueServiceTicketResponse,
            await self._account_call(
                self._account.IssueServiceTicket,
                bot_access_pb2.IssueServiceTicketRequest(
                    external_user_id=external_user_id,
                    binding_ref=binding_ref,
                    requested_action=requested_action,
                    profile_ref=profile_ref,
                ),
                request_id,
            ),
        )
        return ServiceTicket(
            token=response.ticket,
            audience=response.audience,
            expires_at=_datetime_from_timestamp(response.expires_at),
            binding=_binding_from_proto(response.binding),
        )

    async def describe_platform(self, service_key: str, *, request_id: str | None = None) -> PlatformDescriptor:
        return await self._describe_platform(service_key, self._resolve_request_id(request_id))

    async def _describe_platform(self, service_key: str, request_id: str) -> PlatformDescriptor:
        stub = await self._platform_stub(service_key, request_id)
        response = await _grpc_call(
            stub.DescribePlatform(
                runtime_pb2.DescribePlatformRequest(),
                metadata=_correlation_metadata(request_id),
                timeout=self._timeout,
            ),
            request_id,
            failed_precondition_error=ServiceUnavailableError,
        )
        route = self._platform_routes[service_key]
        if (
            response.platform_key != route.platform_key
            or response.service_audience != route.service_audience
            or len(response.supported_actions) != len(route.supported_actions)
            or set(response.supported_actions) != set(route.supported_actions)
        ):
            logger.error("Platform descriptor does not match the authenticated Account Center route")
            raise ServiceUnavailableError("platform descriptor does not match registered route")
        schema = MessageToDict(response.credential_schema, preserving_proto_field_name=True)
        return PlatformDescriptor(
            platform_key=response.platform_key,
            display_name=response.display_name,
            service_audience=response.service_audience,
            supported_actions=tuple(response.supported_actions),
            credential_schema=schema,
            contract_version=response.contract_version,
        )

    async def get_credential_status(
        self,
        *,
        external_user_id: str,
        binding: PlatformBinding,
        request_id: str | None = None,
    ) -> CredentialStatusResult:
        resolved_request_id = self._resolve_request_id(request_id)
        stub, ticket = await self._authorize_platform_action(
            external_user_id=external_user_id,
            binding=binding,
            action="mihomo.status.read",
            request_id=resolved_request_id,
        )
        response = await _grpc_call(
            stub.GetStatus(
                runtime_pb2.GetStatusRequest(resource=_binding_resource(binding)),
                metadata=_service_ticket_metadata(ticket.token, resolved_request_id),
                timeout=self._timeout,
            ),
            resolved_request_id,
            failed_precondition_error=CredentialError,
        )
        return CredentialStatusResult(
            status=_credential_status(response.status),
            last_validated_at=_datetime_from_timestamp(response.last_validated_at),
        )

    async def validate_credential(
        self,
        *,
        external_user_id: str,
        binding: PlatformBinding,
        request_id: str | None = None,
    ) -> ValidationResult:
        resolved_request_id = self._resolve_request_id(request_id)
        stub, ticket = await self._authorize_platform_action(
            external_user_id=external_user_id,
            binding=binding,
            action="mihomo.credential.validate",
            request_id=resolved_request_id,
        )
        response = await _grpc_call(
            stub.ValidateCredential(
                runtime_pb2.ValidateCredentialRequest(resource=_binding_resource(binding)),
                metadata=_service_ticket_metadata(ticket.token, resolved_request_id),
                timeout=self._timeout,
            ),
            resolved_request_id,
            failed_precondition_error=CredentialError,
        )
        return ValidationResult(status=_credential_status(response.status), reason_code=response.reason_code)

    async def list_profiles(
        self,
        *,
        external_user_id: str,
        binding: PlatformBinding,
        request_id: str | None = None,
    ) -> tuple[ProfileSummary, ...]:
        resolved_request_id = self._resolve_request_id(request_id)
        stub, ticket = await self._authorize_platform_action(
            external_user_id=external_user_id,
            binding=binding,
            action="mihomo.profile.read",
            request_id=resolved_request_id,
        )
        response = await _grpc_call(
            stub.ListProfiles(
                runtime_pb2.ListProfilesRequest(resource=_binding_resource(binding)),
                metadata=_service_ticket_metadata(ticket.token, resolved_request_id),
                timeout=self._timeout,
            ),
            resolved_request_id,
            failed_precondition_error=CredentialError,
        )
        return tuple(_profile_summary_from_proto(profile) for profile in response.snapshot.profiles)

    async def get_primary_profile(
        self,
        *,
        external_user_id: str,
        binding: PlatformBinding,
        request_id: str | None = None,
    ) -> ProfileSummary | None:
        resolved_request_id = self._resolve_request_id(request_id)
        stub, ticket = await self._authorize_platform_action(
            external_user_id=external_user_id,
            binding=binding,
            action="mihomo.profile.read",
            request_id=resolved_request_id,
        )
        response = await _grpc_call(
            stub.GetPrimaryProfile(
                runtime_pb2.GetPrimaryProfileRequest(resource=_binding_resource(binding)),
                metadata=_service_ticket_metadata(ticket.token, resolved_request_id),
                timeout=self._timeout,
            ),
            resolved_request_id,
            failed_precondition_error=CredentialError,
        )
        return _profile_summary_from_proto(response.profile) if response.HasField("profile") else None

    async def get_auth_key(
        self,
        *,
        external_user_id: str,
        binding: PlatformBinding,
        profile_ref: str,
        request_id: str | None = None,
    ) -> AuthKey:
        if not profile_ref:
            raise InvalidRequestError("profile_ref is required")
        resolved_request_id = self._resolve_request_id(request_id)
        stub, ticket = await self._authorize_platform_action(
            external_user_id=external_user_id,
            binding=binding,
            action="mihomo.authkey.issue",
            profile_ref=profile_ref,
            request_id=resolved_request_id,
        )
        response = await _grpc_call(
            stub.GetAuthKey(
                runtime_pb2.GetAuthKeyRequest(
                    resource=_binding_resource(binding),
                    profile_ref=profile_ref,
                ),
                metadata=_service_ticket_metadata(ticket.token, resolved_request_id),
                timeout=self._timeout,
            ),
            resolved_request_id,
            failed_precondition_error=CredentialError,
        )
        return AuthKey(value=response.authkey, expires_at=_datetime_from_timestamp(response.expires_at))

    async def get_device(
        self,
        *,
        external_user_id: str,
        binding: PlatformBinding,
        device_ref: str,
        request_id: str | None = None,
    ) -> DeviceSummary:
        if not device_ref:
            raise InvalidRequestError("device_ref is required")
        resolved_request_id = self._resolve_request_id(request_id)
        stub, ticket = await self._authorize_platform_action(
            external_user_id=external_user_id,
            binding=binding,
            action="mihomo.device.read",
            request_id=resolved_request_id,
        )
        response = await _grpc_call(
            stub.GetDevice(
                runtime_pb2.GetDeviceRequest(
                    resource=_binding_resource(binding),
                    device_ref=device_ref,
                ),
                metadata=_service_ticket_metadata(ticket.token, resolved_request_id),
                timeout=self._timeout,
            ),
            resolved_request_id,
            failed_precondition_error=CredentialError,
        )
        return _device_summary_from_proto(response.device)

    async def _account_call(
        self,
        method: Callable[..., Awaitable[object]],
        request: object,
        request_id: str,
        *,
        failed_precondition_error: type[AccountSDKError] = CredentialError,
        required_scopes: tuple[str, ...] = ("bot.access.read", "bot.access.issue_ticket"),
    ) -> object:
        token = await self._tokens.get(request_id, required_scopes)
        call = method(
            request,
            metadata=(("authorization", f"Bearer {token}"), *_correlation_metadata(request_id)),
            timeout=self._timeout,
        )
        return await _grpc_call(
            call,
            request_id,
            failed_precondition_error=failed_precondition_error,
        )

    async def _authorize_platform_action(
        self,
        *,
        external_user_id: str,
        binding: PlatformBinding,
        action: str,
        request_id: str,
        profile_ref: str = "",
    ) -> tuple[runtime_pb2_grpc.MihomoRuntimeServiceStub, ServiceTicket]:
        ticket = await self._issue_service_ticket(
            external_user_id=external_user_id,
            binding_ref=binding.binding_ref,
            requested_action=action,
            profile_ref=profile_ref,
            request_id=request_id,
        )
        return await self._platform_stub(binding.platform_service_key, request_id), ticket

    async def _platform_stub(self, service_key: str, request_id: str) -> runtime_pb2_grpc.MihomoRuntimeServiceStub:
        self._ensure_open()
        existing = self._platform_stubs.get(service_key)
        if existing is not None:
            return existing
        async with self._platform_route_lock:
            self._ensure_open()
            existing = self._platform_stubs.get(service_key)
            if existing is not None:
                return existing
            route = cast(
                bot_access_pb2.GetPlatformRuntimeRouteResponse,
                await self._account_call(
                    self._account.GetPlatformRuntimeRoute,
                    bot_access_pb2.GetPlatformRuntimeRouteRequest(platform_service_key=service_key),
                    request_id,
                    failed_precondition_error=ServiceUnavailableError,
                ),
            )
            self._ensure_open()
            if route.platform_service_key != service_key:
                logger.error("Account Center returned a runtime route for the wrong platform service")
                raise ServiceUnavailableError("platform runtime route does not match requested service")
            if not _valid_grpc_target(route.runtime_endpoint) or not _valid_tls_server_name(route.runtime_server_name):
                logger.error("Account Center returned an invalid platform runtime route")
                raise ServiceUnavailableError(f"invalid platform runtime route: {service_key}")
            channel = _create_secure_channel(
                route.runtime_endpoint,
                root_certificates=self._platform_root_certificates.get(service_key),
                server_name=route.runtime_server_name,
            )
            stub = runtime_pb2_grpc.MihomoRuntimeServiceStub(channel)  # type: ignore[no-untyped-call]
            self._platform_channels[service_key] = channel
            self._platform_stubs[service_key] = stub
            self._platform_routes[service_key] = route
            return stub

    def _ensure_open(self) -> None:
        if self._closed:
            logger.warning("PaiGram Account SDK rejected an operation after close")
            raise TransportError("client is closed")

    def _resolve_request_id(self, request_id: str | None) -> str:
        resolved = request_id or self._request_id_factory()
        if not resolved:
            logger.error("PaiGram Account SDK request ID factory returned an empty value")
            raise InvalidRequestError("request_id must not be empty")
        return resolved


def _create_secure_channel(
    target: str,
    *,
    root_certificates: bytes | None,
    server_name: str | None,
) -> grpc.aio.Channel:
    if not target:
        logger.warning("PaiGram Account SDK rejected an empty gRPC target")
        raise InvalidRequestError("gRPC target is required")
    credentials = grpc.ssl_channel_credentials(root_certificates=root_certificates)
    options: tuple[tuple[str, str | int], ...] = (("grpc.enable_http_proxy", 0),)
    if server_name is not None:
        options += (
            ("grpc.ssl_target_name_override", server_name),
            ("grpc.default_authority", server_name),
        )
    return grpc.aio.secure_channel(target, credentials, options=options)


def _valid_tls_server_name(server_name: str | None) -> bool:
    if server_name is None or not server_name or server_name.strip() != server_name:
        return False
    return not any(character.isspace() or character in "/:?#" for character in server_name)


def _valid_grpc_target(target: str) -> bool:
    if not target or target.strip() != target or "://" in target:
        return False
    if any(character.isspace() or character in "/?#" for character in target):
        return False
    host, separator, port = target.rpartition(":")
    return bool(separator and host and port.isdecimal() and 0 < int(port) <= 65535)


async def _grpc_call(
    call: Awaitable[T],
    request_id: str,
    *,
    failed_precondition_error: type[AccountSDKError],
) -> T:
    try:
        return await call
    except grpc.aio.AioRpcError as error:
        mapped = _map_grpc_error(error, failed_precondition_error)
        logger.warning(
            "gRPC request %s failed with %s: %s",
            request_id,
            error.code().name,
            error.details(),
        )
        raise mapped from error


def _map_grpc_error(
    error: grpc.aio.AioRpcError,
    failed_precondition_error: type[AccountSDKError],
) -> AccountSDKError:
    message = error.details() or error.code().name
    error_type: type[AccountSDKError]
    if error.code() == grpc.StatusCode.INVALID_ARGUMENT:
        error_type = InvalidRequestError
    elif error.code() == grpc.StatusCode.UNAUTHENTICATED:
        error_type = AuthenticationError
    elif error.code() == grpc.StatusCode.PERMISSION_DENIED:
        error_type = AuthorizationError
    elif error.code() == grpc.StatusCode.NOT_FOUND:
        error_type = NotFoundError
    elif error.code() in (grpc.StatusCode.ALREADY_EXISTS, grpc.StatusCode.ABORTED):
        error_type = ConflictError
    elif error.code() == grpc.StatusCode.FAILED_PRECONDITION:
        error_type = failed_precondition_error
    elif error.code() == grpc.StatusCode.DEADLINE_EXCEEDED:
        error_type = DeadlineExceededError
    elif error.code() == grpc.StatusCode.RESOURCE_EXHAUSTED:
        error_type = RateLimitError
    elif error.code() == grpc.StatusCode.UNAVAILABLE:
        error_type = ServiceUnavailableError
    else:
        error_type = TransportError
    return error_type(message)


def _binding_from_proto(binding: bot_access_pb2.PlatformAccountBinding) -> PlatformBinding:
    return PlatformBinding(
        binding_ref=binding.binding_ref,
        platform=binding.platform,
        platform_service_key=binding.platform_service_key,
        account_key=binding.account_key,
        display_name=binding.display_name,
        status=_platform_account_status(binding.status),
        generation=binding.generation,
        created_at=_datetime_from_timestamp(binding.created_at),
        updated_at=_datetime_from_timestamp(binding.updated_at),
    )


def _platform_account_status(value: int) -> PlatformAccountStatus:
    statuses: dict[int, PlatformAccountStatus] = {
        bot_access_pb2.PLATFORM_ACCOUNT_STATUS_ACTIVE: PlatformAccountStatus.ACTIVE,
        bot_access_pb2.PLATFORM_ACCOUNT_STATUS_INACTIVE: PlatformAccountStatus.INACTIVE,
        bot_access_pb2.PLATFORM_ACCOUNT_STATUS_REVOKED: PlatformAccountStatus.REVOKED,
    }
    return statuses.get(value, PlatformAccountStatus.UNSPECIFIED)


def _credential_status(value: int) -> CredentialStatus:
    statuses: dict[int, CredentialStatus] = {
        types_pb2.CREDENTIAL_STATUS_ACTIVE: CredentialStatus.ACTIVE,
        types_pb2.CREDENTIAL_STATUS_EXPIRED: CredentialStatus.EXPIRED,
        types_pb2.CREDENTIAL_STATUS_INVALID: CredentialStatus.INVALID,
        types_pb2.CREDENTIAL_STATUS_CHALLENGE_REQUIRED: CredentialStatus.CHALLENGE_REQUIRED,
    }
    return statuses.get(value, CredentialStatus.UNSPECIFIED)


def _binding_resource(binding: PlatformBinding) -> types_pb2.BindingResource:
    return types_pb2.BindingResource(binding_ref=binding.binding_ref, account_key=binding.account_key)


def _profile_summary_from_proto(profile: types_pb2.ProfileSummary) -> ProfileSummary:
    return ProfileSummary(
        profile_ref=profile.profile_ref,
        account_key=profile.account_key,
        game_biz=profile.game_biz,
        region=profile.region,
        player_id=profile.player_id,
        nickname=profile.nickname,
        level=profile.level,
        is_default=profile.is_default,
    )


def _device_summary_from_proto(device: types_pb2.DeviceSummary) -> DeviceSummary:
    return DeviceSummary(
        device_ref=device.device_ref,
        device_name=device.device_name,
        is_valid=device.is_valid,
        last_seen_at=_datetime_from_timestamp(device.last_seen_at),
    )


def _datetime_from_timestamp(value: Timestamp) -> datetime | None:
    if value.seconds == 0 and value.nanos == 0:
        return None
    converted = value.ToDatetime(tzinfo=timezone.utc)
    if not isinstance(converted, datetime):
        logger.error("Remote service returned an invalid protobuf timestamp")
        raise TransportError("remote service returned an invalid timestamp")
    return converted


def _correlation_metadata(request_id: str) -> tuple[tuple[str, str]]:
    return (("x-request-id", request_id),)


def _service_ticket_metadata(ticket: str, request_id: str) -> tuple[tuple[str, str], ...]:
    return (("authorization", f"Bearer {ticket}"), *_correlation_metadata(request_id))

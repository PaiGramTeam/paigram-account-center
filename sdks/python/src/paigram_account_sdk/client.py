from __future__ import annotations

import logging
from collections.abc import Awaitable, Callable, Mapping, Sequence
from datetime import datetime, timezone
from types import TracebackType
from typing import TypeVar, cast
from uuid import uuid4

import grpc
import httpx
from google.protobuf.json_format import MessageToDict
from google.protobuf.timestamp_pb2 import Timestamp

from paigram_account_sdk._generated.account.v1 import bot_access_pb2, bot_access_pb2_grpc
from paigram_account_sdk._generated.platform.v1 import platform_pb2, platform_pb2_grpc

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
    ServiceUnavailableError,
    TransportError,
)
from .models import (
    AuthKey,
    BotUser,
    CredentialStatus,
    CredentialStatusResult,
    CredentialSummary,
    DeviceInfo,
    DeviceSummary,
    PlatformAccountStatus,
    PlatformBinding,
    PlatformDescriptor,
    PlatformEndpoint,
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
        client_id: str,
        client_secret: str,
        platform_endpoints: Mapping[str, PlatformEndpoint],
        account_grpc_secure: bool = True,
        account_root_certificates: bytes | None = None,
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
        invalid_platform = next((key for key, endpoint in platform_endpoints.items() if not endpoint.target), None)
        if invalid_platform is not None:
            logger.warning("PaiGram Account SDK rejected an empty platform gRPC target for %s", invalid_platform)
            raise InvalidRequestError(f"platform endpoint target is required: {invalid_platform}")
        self._timeout = timeout
        self._platform_endpoints = dict(platform_endpoints)
        self._http_client = httpx.AsyncClient(
            base_url=account_http_url.rstrip("/"),
            timeout=timeout,
            transport=http_transport,
        )
        self._tokens = _ClientCredentialsTokenProvider(self._http_client, client_id, client_secret, timeout)
        self._account_channel = _create_channel(
            account_grpc_target,
            secure=account_grpc_secure,
            root_certificates=account_root_certificates,
        )
        self._account = bot_access_pb2_grpc.BotAccessServiceStub(self._account_channel)  # type: ignore[no-untyped-call]
        self._platform_channels: dict[str, grpc.aio.Channel] = {}
        self._platform_stubs: dict[str, platform_pb2_grpc.PlatformServiceStub] = {}
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
        if self._closed:
            return
        first_error: BaseException | None = None
        resources = (self._http_client, self._account_channel, *self._platform_channels.values())
        for resource in resources:
            try:
                await resource.aclose() if isinstance(resource, httpx.AsyncClient) else await resource.close()
            except BaseException as error:
                logger.error("PaiGram Account SDK failed to close a transport", exc_info=error)
                if first_error is None:
                    first_error = error
        self._closed = True
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
        binding_id: int,
        requested_scopes: Sequence[str],
        profile_id: int,
        request_id: str,
    ) -> ServiceTicket:
        response = cast(
            bot_access_pb2.IssueServiceTicketResponse,
            await self._account_call(
                self._account.IssueServiceTicket,
                bot_access_pb2.IssueServiceTicketRequest(
                    external_user_id=external_user_id,
                    binding_id=binding_id,
                    requested_scopes=requested_scopes,
                    profile_id=profile_id,
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
        stub = self._platform_stub(service_key)
        response = await _grpc_call(
            stub.DescribePlatform(
                platform_pb2.DescribePlatformRequest(),
                metadata=_correlation_metadata(request_id),
                timeout=self._timeout,
            ),
            request_id,
            failed_precondition_error=ServiceUnavailableError,
        )
        schema = MessageToDict(response.credential_schema, preserving_proto_field_name=True)
        return PlatformDescriptor(
            platform_key=response.platform_key,
            display_name=response.display_name,
            service_audience=response.service_audience,
            supported_actions=tuple(response.supported_actions),
            credential_schema=schema,
            version=response.version,
        )

    async def get_credential_summary(
        self,
        *,
        external_user_id: str,
        binding: PlatformBinding,
        request_id: str | None = None,
    ) -> CredentialSummary:
        resolved_request_id = self._resolve_request_id(request_id)
        stub, ticket = await self._authorize_platform_action(
            external_user_id=external_user_id,
            binding=binding,
            action_suffix="credential.read_meta",
            request_id=resolved_request_id,
        )
        response = await _grpc_call(
            stub.GetCredentialSummary(
                platform_pb2.GetCredentialSummaryRequest(
                    platform_account_id=binding.platform_account_id,
                ),
                metadata=_service_ticket_metadata(ticket.token, resolved_request_id),
                timeout=self._timeout,
            ),
            resolved_request_id,
            failed_precondition_error=CredentialError,
        )
        return _credential_summary_from_proto(response)

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
            action_suffix="status.read",
            request_id=resolved_request_id,
        )
        response = await _grpc_call(
            stub.GetCredentialStatus(
                platform_pb2.GetCredentialStatusRequest(
                    platform_account_id=binding.platform_account_id,
                ),
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
            action_suffix="credential.validate",
            request_id=resolved_request_id,
        )
        response = await _grpc_call(
            stub.ValidateCredential(
                platform_pb2.ValidateCredentialRequest(
                    platform_account_id=binding.platform_account_id,
                ),
                metadata=_service_ticket_metadata(ticket.token, resolved_request_id),
                timeout=self._timeout,
            ),
            resolved_request_id,
            failed_precondition_error=CredentialError,
        )
        return ValidationResult(status=_credential_status(response.status), error_code=response.error_code)

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
            action_suffix="profile.read",
            request_id=resolved_request_id,
        )
        response = await _grpc_call(
            stub.ListProfiles(
                platform_pb2.ListProfilesRequest(
                    platform_account_id=binding.platform_account_id,
                ),
                metadata=_service_ticket_metadata(ticket.token, resolved_request_id),
                timeout=self._timeout,
            ),
            resolved_request_id,
            failed_precondition_error=CredentialError,
        )
        return tuple(_profile_summary_from_proto(profile) for profile in response.profiles)

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
            action_suffix="profile.read",
            request_id=resolved_request_id,
        )
        response = await _grpc_call(
            stub.GetPrimaryProfile(
                platform_pb2.GetPrimaryProfileRequest(
                    platform_account_id=binding.platform_account_id,
                ),
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
        player_id: str,
        request_id: str | None = None,
    ) -> AuthKey:
        if not player_id:
            raise InvalidRequestError("player_id is required")
        resolved_request_id = self._resolve_request_id(request_id)
        stub, ticket = await self._authorize_platform_action(
            external_user_id=external_user_id,
            binding=binding,
            action_suffix="authkey.issue",
            request_id=resolved_request_id,
        )
        response = await _grpc_call(
            stub.GetAuthKey(
                platform_pb2.GetAuthKeyRequest(
                    platform_account_id=binding.platform_account_id,
                    player_id=player_id,
                ),
                metadata=_service_ticket_metadata(ticket.token, resolved_request_id),
                timeout=self._timeout,
            ),
            resolved_request_id,
            failed_precondition_error=CredentialError,
        )
        return AuthKey(value=response.authkey, expires_at=_datetime_from_timestamp(response.expires_at))

    async def upsert_device(
        self,
        *,
        external_user_id: str,
        binding: PlatformBinding,
        device: DeviceInfo,
        request_id: str | None = None,
    ) -> bool:
        if not device.device_id or not device.device_fp:
            raise InvalidRequestError("device_id and device_fp are required")
        resolved_request_id = self._resolve_request_id(request_id)
        stub, ticket = await self._authorize_platform_action(
            external_user_id=external_user_id,
            binding=binding,
            action_suffix="device.update",
            request_id=resolved_request_id,
        )
        response = await _grpc_call(
            stub.UpsertDevice(
                platform_pb2.UpsertDeviceRequest(
                    platform_account_id=binding.platform_account_id,
                    device=platform_pb2.DeviceInfo(
                        device_id=device.device_id,
                        device_fp=device.device_fp,
                        device_name=device.device_name,
                    ),
                ),
                metadata=_service_ticket_metadata(ticket.token, resolved_request_id),
                timeout=self._timeout,
            ),
            resolved_request_id,
            failed_precondition_error=CredentialError,
        )
        return bool(response.success)

    async def _account_call(
        self,
        method: Callable[..., Awaitable[object]],
        request: object,
        request_id: str,
    ) -> object:
        token = await self._tokens.get(request_id)
        call = method(
            request,
            metadata=(("authorization", f"Bearer {token}"), *_correlation_metadata(request_id)),
            timeout=self._timeout,
        )
        return await _grpc_call(
            call,
            request_id,
            failed_precondition_error=CredentialError,
        )

    async def _authorize_platform_action(
        self,
        *,
        external_user_id: str,
        binding: PlatformBinding,
        action_suffix: str,
        request_id: str,
    ) -> tuple[platform_pb2_grpc.PlatformServiceStub, ServiceTicket]:
        descriptor = await self._describe_platform(binding.platform_service_key, request_id)
        action = _find_action(descriptor.supported_actions, action_suffix)
        ticket = await self._issue_service_ticket(
            external_user_id=external_user_id,
            binding_id=binding.id,
            requested_scopes=(action,),
            profile_id=0,
            request_id=request_id,
        )
        return self._platform_stub(binding.platform_service_key), ticket

    def _platform_stub(self, service_key: str) -> platform_pb2_grpc.PlatformServiceStub:
        self._ensure_open()
        endpoint = self._platform_endpoints.get(service_key)
        if endpoint is None:
            logger.warning("PaiGram Account SDK has no endpoint for platform service %s", service_key)
            raise InvalidRequestError(f"platform endpoint is not configured: {service_key}")
        existing = self._platform_stubs.get(service_key)
        if existing is not None:
            return existing
        channel = _create_channel(
            endpoint.target,
            secure=endpoint.secure,
            root_certificates=endpoint.root_certificates,
        )
        stub = platform_pb2_grpc.PlatformServiceStub(channel)  # type: ignore[no-untyped-call]
        self._platform_channels[service_key] = channel
        self._platform_stubs[service_key] = stub
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


def _create_channel(
    target: str,
    *,
    secure: bool,
    root_certificates: bytes | None,
) -> grpc.aio.Channel:
    if not target:
        logger.warning("PaiGram Account SDK rejected an empty gRPC target")
        raise InvalidRequestError("gRPC target is required")
    if not secure:
        return grpc.aio.insecure_channel(target)
    credentials = grpc.ssl_channel_credentials(root_certificates=root_certificates)
    return grpc.aio.secure_channel(target, credentials)


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
    elif error.code() == grpc.StatusCode.UNAVAILABLE or error.code() == grpc.StatusCode.RESOURCE_EXHAUSTED:
        error_type = ServiceUnavailableError
    else:
        error_type = TransportError
    return error_type(message)


def _binding_from_proto(binding: bot_access_pb2.PlatformAccountBinding) -> PlatformBinding:
    return PlatformBinding(
        id=binding.id,
        user_id=binding.user_id,
        platform=binding.platform,
        platform_service_key=binding.platform_service_key,
        platform_account_id=binding.platform_account_id,
        display_name=binding.display_name,
        status=_platform_account_status(binding.status),
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
        platform_pb2.CREDENTIAL_STATUS_ACTIVE: CredentialStatus.ACTIVE,
        platform_pb2.CREDENTIAL_STATUS_EXPIRED: CredentialStatus.EXPIRED,
        platform_pb2.CREDENTIAL_STATUS_INVALID: CredentialStatus.INVALID,
        platform_pb2.CREDENTIAL_STATUS_CHALLENGE_REQUIRED: CredentialStatus.CHALLENGE_REQUIRED,
    }
    return statuses.get(value, CredentialStatus.UNSPECIFIED)


def _credential_summary_from_proto(response: platform_pb2.GetCredentialSummaryResponse) -> CredentialSummary:
    return CredentialSummary(
        platform_account_id=response.platform_account_id,
        status=_credential_status(response.status),
        last_validated_at=_datetime_from_timestamp(response.last_validated_at),
        last_refreshed_at=_datetime_from_timestamp(response.last_refreshed_at),
        devices=tuple(
            DeviceSummary(
                device_id=device.device_id,
                device_fp=device.device_fp,
                device_name=device.device_name,
                is_valid=device.is_valid,
                last_seen_at=_datetime_from_timestamp(device.last_seen_at),
            )
            for device in response.devices
        ),
        profiles=tuple(_profile_summary_from_proto(profile) for profile in response.profiles),
    )


def _profile_summary_from_proto(profile: platform_pb2.ProfileSummary) -> ProfileSummary:
    return ProfileSummary(
        id=profile.id,
        platform_account_id=profile.platform_account_id,
        game_biz=profile.game_biz,
        region=profile.region,
        player_id=profile.player_id,
        nickname=profile.nickname,
        level=profile.level,
        is_default=profile.is_default,
    )


def _datetime_from_timestamp(value: Timestamp) -> datetime | None:
    if value.seconds == 0 and value.nanos == 0:
        return None
    converted = value.ToDatetime(tzinfo=timezone.utc)
    if not isinstance(converted, datetime):
        logger.error("Remote service returned an invalid protobuf timestamp")
        raise TransportError("remote service returned an invalid timestamp")
    return converted


def _find_action(actions: Sequence[str], suffix: str) -> str:
    matches = [action for action in actions if action == suffix or action.endswith(f".{suffix}")]
    if len(matches) != 1:
        logger.warning("Platform does not advertise exactly one %s action", suffix)
        raise AuthorizationError(f"platform does not advertise exactly one {suffix} action")
    return matches[0]


def _correlation_metadata(request_id: str) -> tuple[tuple[str, str]]:
    return (("x-request-id", request_id),)


def _service_ticket_metadata(ticket: str, request_id: str) -> tuple[tuple[str, str], ...]:
    return (("authorization", f"Bearer {ticket}"), *_correlation_metadata(request_id))

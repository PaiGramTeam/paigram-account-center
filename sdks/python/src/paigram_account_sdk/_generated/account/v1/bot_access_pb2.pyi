import datetime

from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class PlatformAccountStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    PLATFORM_ACCOUNT_STATUS_UNSPECIFIED: _ClassVar[PlatformAccountStatus]
    PLATFORM_ACCOUNT_STATUS_ACTIVE: _ClassVar[PlatformAccountStatus]
    PLATFORM_ACCOUNT_STATUS_INACTIVE: _ClassVar[PlatformAccountStatus]
    PLATFORM_ACCOUNT_STATUS_REVOKED: _ClassVar[PlatformAccountStatus]
PLATFORM_ACCOUNT_STATUS_UNSPECIFIED: PlatformAccountStatus
PLATFORM_ACCOUNT_STATUS_ACTIVE: PlatformAccountStatus
PLATFORM_ACCOUNT_STATUS_INACTIVE: PlatformAccountStatus
PLATFORM_ACCOUNT_STATUS_REVOKED: PlatformAccountStatus

class StartEntryIdentityLinkRequest(_message.Message):
    __slots__ = ("external_subject", "external_username")
    EXTERNAL_SUBJECT_FIELD_NUMBER: _ClassVar[int]
    EXTERNAL_USERNAME_FIELD_NUMBER: _ClassVar[int]
    external_subject: str
    external_username: str
    def __init__(self, external_subject: _Optional[str] = ..., external_username: _Optional[str] = ...) -> None: ...

class StartEntryIdentityLinkResponse(_message.Message):
    __slots__ = ("approval_url", "issuer", "masked_subject", "bot_id", "bot_display_name", "expires_at")
    APPROVAL_URL_FIELD_NUMBER: _ClassVar[int]
    ISSUER_FIELD_NUMBER: _ClassVar[int]
    MASKED_SUBJECT_FIELD_NUMBER: _ClassVar[int]
    BOT_ID_FIELD_NUMBER: _ClassVar[int]
    BOT_DISPLAY_NAME_FIELD_NUMBER: _ClassVar[int]
    EXPIRES_AT_FIELD_NUMBER: _ClassVar[int]
    approval_url: str
    issuer: str
    masked_subject: str
    bot_id: str
    bot_display_name: str
    expires_at: _timestamp_pb2.Timestamp
    def __init__(self, approval_url: _Optional[str] = ..., issuer: _Optional[str] = ..., masked_subject: _Optional[str] = ..., bot_id: _Optional[str] = ..., bot_display_name: _Optional[str] = ..., expires_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class PlatformAccountBinding(_message.Message):
    __slots__ = ("binding_ref", "platform", "platform_service_key", "account_key", "display_name", "status", "generation", "created_at", "updated_at")
    BINDING_REF_FIELD_NUMBER: _ClassVar[int]
    PLATFORM_FIELD_NUMBER: _ClassVar[int]
    PLATFORM_SERVICE_KEY_FIELD_NUMBER: _ClassVar[int]
    ACCOUNT_KEY_FIELD_NUMBER: _ClassVar[int]
    DISPLAY_NAME_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    GENERATION_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    UPDATED_AT_FIELD_NUMBER: _ClassVar[int]
    binding_ref: str
    platform: str
    platform_service_key: str
    account_key: str
    display_name: str
    status: PlatformAccountStatus
    generation: int
    created_at: _timestamp_pb2.Timestamp
    updated_at: _timestamp_pb2.Timestamp
    def __init__(self, binding_ref: _Optional[str] = ..., platform: _Optional[str] = ..., platform_service_key: _Optional[str] = ..., account_key: _Optional[str] = ..., display_name: _Optional[str] = ..., status: _Optional[_Union[PlatformAccountStatus, str]] = ..., generation: _Optional[int] = ..., created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., updated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class ResolveBotUserRequest(_message.Message):
    __slots__ = ("external_user_id",)
    EXTERNAL_USER_ID_FIELD_NUMBER: _ClassVar[int]
    external_user_id: str
    def __init__(self, external_user_id: _Optional[str] = ...) -> None: ...

class ResolveBotUserResponse(_message.Message):
    __slots__ = ("user_id", "bot_id", "external_user_id", "external_username")
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    BOT_ID_FIELD_NUMBER: _ClassVar[int]
    EXTERNAL_USER_ID_FIELD_NUMBER: _ClassVar[int]
    EXTERNAL_USERNAME_FIELD_NUMBER: _ClassVar[int]
    user_id: int
    bot_id: str
    external_user_id: str
    external_username: str
    def __init__(self, user_id: _Optional[int] = ..., bot_id: _Optional[str] = ..., external_user_id: _Optional[str] = ..., external_username: _Optional[str] = ...) -> None: ...

class ListAccessibleBindingsRequest(_message.Message):
    __slots__ = ("external_user_id", "platform")
    EXTERNAL_USER_ID_FIELD_NUMBER: _ClassVar[int]
    PLATFORM_FIELD_NUMBER: _ClassVar[int]
    external_user_id: str
    platform: str
    def __init__(self, external_user_id: _Optional[str] = ..., platform: _Optional[str] = ...) -> None: ...

class ListAccessibleBindingsResponse(_message.Message):
    __slots__ = ("bindings",)
    BINDINGS_FIELD_NUMBER: _ClassVar[int]
    bindings: _containers.RepeatedCompositeFieldContainer[PlatformAccountBinding]
    def __init__(self, bindings: _Optional[_Iterable[_Union[PlatformAccountBinding, _Mapping]]] = ...) -> None: ...

class GetPlatformRuntimeRouteRequest(_message.Message):
    __slots__ = ("platform_service_key",)
    PLATFORM_SERVICE_KEY_FIELD_NUMBER: _ClassVar[int]
    platform_service_key: str
    def __init__(self, platform_service_key: _Optional[str] = ...) -> None: ...

class GetPlatformRuntimeRouteResponse(_message.Message):
    __slots__ = ("platform_key", "platform_service_key", "runtime_endpoint", "runtime_server_name", "service_audience", "supported_actions")
    PLATFORM_KEY_FIELD_NUMBER: _ClassVar[int]
    PLATFORM_SERVICE_KEY_FIELD_NUMBER: _ClassVar[int]
    RUNTIME_ENDPOINT_FIELD_NUMBER: _ClassVar[int]
    RUNTIME_SERVER_NAME_FIELD_NUMBER: _ClassVar[int]
    SERVICE_AUDIENCE_FIELD_NUMBER: _ClassVar[int]
    SUPPORTED_ACTIONS_FIELD_NUMBER: _ClassVar[int]
    platform_key: str
    platform_service_key: str
    runtime_endpoint: str
    runtime_server_name: str
    service_audience: str
    supported_actions: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, platform_key: _Optional[str] = ..., platform_service_key: _Optional[str] = ..., runtime_endpoint: _Optional[str] = ..., runtime_server_name: _Optional[str] = ..., service_audience: _Optional[str] = ..., supported_actions: _Optional[_Iterable[str]] = ...) -> None: ...

class IssueServiceTicketRequest(_message.Message):
    __slots__ = ("external_user_id", "binding_ref", "requested_action", "profile_ref")
    EXTERNAL_USER_ID_FIELD_NUMBER: _ClassVar[int]
    BINDING_REF_FIELD_NUMBER: _ClassVar[int]
    REQUESTED_ACTION_FIELD_NUMBER: _ClassVar[int]
    PROFILE_REF_FIELD_NUMBER: _ClassVar[int]
    external_user_id: str
    binding_ref: str
    requested_action: str
    profile_ref: str
    def __init__(self, external_user_id: _Optional[str] = ..., binding_ref: _Optional[str] = ..., requested_action: _Optional[str] = ..., profile_ref: _Optional[str] = ...) -> None: ...

class IssueServiceTicketResponse(_message.Message):
    __slots__ = ("ticket", "audience", "expires_at", "binding")
    TICKET_FIELD_NUMBER: _ClassVar[int]
    AUDIENCE_FIELD_NUMBER: _ClassVar[int]
    EXPIRES_AT_FIELD_NUMBER: _ClassVar[int]
    BINDING_FIELD_NUMBER: _ClassVar[int]
    ticket: str
    audience: str
    expires_at: _timestamp_pb2.Timestamp
    binding: PlatformAccountBinding
    def __init__(self, ticket: _Optional[str] = ..., audience: _Optional[str] = ..., expires_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., binding: _Optional[_Union[PlatformAccountBinding, _Mapping]] = ...) -> None: ...

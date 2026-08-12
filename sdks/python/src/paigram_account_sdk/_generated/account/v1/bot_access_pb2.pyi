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

class PlatformAccountBinding(_message.Message):
    __slots__ = ("id", "user_id", "platform", "platform_service_key", "platform_account_id", "display_name", "status", "meta_json", "created_at", "updated_at")
    ID_FIELD_NUMBER: _ClassVar[int]
    USER_ID_FIELD_NUMBER: _ClassVar[int]
    PLATFORM_FIELD_NUMBER: _ClassVar[int]
    PLATFORM_SERVICE_KEY_FIELD_NUMBER: _ClassVar[int]
    PLATFORM_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    DISPLAY_NAME_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    META_JSON_FIELD_NUMBER: _ClassVar[int]
    CREATED_AT_FIELD_NUMBER: _ClassVar[int]
    UPDATED_AT_FIELD_NUMBER: _ClassVar[int]
    id: int
    user_id: int
    platform: str
    platform_service_key: str
    platform_account_id: str
    display_name: str
    status: PlatformAccountStatus
    meta_json: str
    created_at: _timestamp_pb2.Timestamp
    updated_at: _timestamp_pb2.Timestamp
    def __init__(self, id: _Optional[int] = ..., user_id: _Optional[int] = ..., platform: _Optional[str] = ..., platform_service_key: _Optional[str] = ..., platform_account_id: _Optional[str] = ..., display_name: _Optional[str] = ..., status: _Optional[_Union[PlatformAccountStatus, str]] = ..., meta_json: _Optional[str] = ..., created_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., updated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

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

class UpsertPlatformBindingRequest(_message.Message):
    __slots__ = ("external_user_id", "platform", "platform_service_key", "platform_account_id", "display_name", "meta_json", "grant_scopes")
    EXTERNAL_USER_ID_FIELD_NUMBER: _ClassVar[int]
    PLATFORM_FIELD_NUMBER: _ClassVar[int]
    PLATFORM_SERVICE_KEY_FIELD_NUMBER: _ClassVar[int]
    PLATFORM_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    DISPLAY_NAME_FIELD_NUMBER: _ClassVar[int]
    META_JSON_FIELD_NUMBER: _ClassVar[int]
    GRANT_SCOPES_FIELD_NUMBER: _ClassVar[int]
    external_user_id: str
    platform: str
    platform_service_key: str
    platform_account_id: str
    display_name: str
    meta_json: str
    grant_scopes: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, external_user_id: _Optional[str] = ..., platform: _Optional[str] = ..., platform_service_key: _Optional[str] = ..., platform_account_id: _Optional[str] = ..., display_name: _Optional[str] = ..., meta_json: _Optional[str] = ..., grant_scopes: _Optional[_Iterable[str]] = ...) -> None: ...

class UpsertPlatformBindingResponse(_message.Message):
    __slots__ = ("binding", "created")
    BINDING_FIELD_NUMBER: _ClassVar[int]
    CREATED_FIELD_NUMBER: _ClassVar[int]
    binding: PlatformAccountBinding
    created: bool
    def __init__(self, binding: _Optional[_Union[PlatformAccountBinding, _Mapping]] = ..., created: _Optional[bool] = ...) -> None: ...

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

class IssueServiceTicketRequest(_message.Message):
    __slots__ = ("external_user_id", "binding_id", "requested_scopes", "profile_id")
    EXTERNAL_USER_ID_FIELD_NUMBER: _ClassVar[int]
    BINDING_ID_FIELD_NUMBER: _ClassVar[int]
    REQUESTED_SCOPES_FIELD_NUMBER: _ClassVar[int]
    PROFILE_ID_FIELD_NUMBER: _ClassVar[int]
    external_user_id: str
    binding_id: int
    requested_scopes: _containers.RepeatedScalarFieldContainer[str]
    profile_id: int
    def __init__(self, external_user_id: _Optional[str] = ..., binding_id: _Optional[int] = ..., requested_scopes: _Optional[_Iterable[str]] = ..., profile_id: _Optional[int] = ...) -> None: ...

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

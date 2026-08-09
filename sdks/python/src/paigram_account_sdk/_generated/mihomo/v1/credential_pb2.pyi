import datetime

from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class CredentialStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    CREDENTIAL_STATUS_UNSPECIFIED: _ClassVar[CredentialStatus]
    CREDENTIAL_STATUS_ACTIVE: _ClassVar[CredentialStatus]
    CREDENTIAL_STATUS_EXPIRED: _ClassVar[CredentialStatus]
    CREDENTIAL_STATUS_INVALID: _ClassVar[CredentialStatus]
    CREDENTIAL_STATUS_CHALLENGE_REQUIRED: _ClassVar[CredentialStatus]
CREDENTIAL_STATUS_UNSPECIFIED: CredentialStatus
CREDENTIAL_STATUS_ACTIVE: CredentialStatus
CREDENTIAL_STATUS_EXPIRED: CredentialStatus
CREDENTIAL_STATUS_INVALID: CredentialStatus
CREDENTIAL_STATUS_CHALLENGE_REQUIRED: CredentialStatus

class DeviceSummary(_message.Message):
    __slots__ = ("device_id", "device_fp", "device_name", "is_valid", "last_seen_at")
    DEVICE_ID_FIELD_NUMBER: _ClassVar[int]
    DEVICE_FP_FIELD_NUMBER: _ClassVar[int]
    DEVICE_NAME_FIELD_NUMBER: _ClassVar[int]
    IS_VALID_FIELD_NUMBER: _ClassVar[int]
    LAST_SEEN_AT_FIELD_NUMBER: _ClassVar[int]
    device_id: str
    device_fp: str
    device_name: str
    is_valid: bool
    last_seen_at: _timestamp_pb2.Timestamp
    def __init__(self, device_id: _Optional[str] = ..., device_fp: _Optional[str] = ..., device_name: _Optional[str] = ..., is_valid: _Optional[bool] = ..., last_seen_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class ProfileSummary(_message.Message):
    __slots__ = ("id", "platform_account_id", "game_biz", "region", "player_id", "nickname", "level", "is_default")
    ID_FIELD_NUMBER: _ClassVar[int]
    PLATFORM_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    GAME_BIZ_FIELD_NUMBER: _ClassVar[int]
    REGION_FIELD_NUMBER: _ClassVar[int]
    PLAYER_ID_FIELD_NUMBER: _ClassVar[int]
    NICKNAME_FIELD_NUMBER: _ClassVar[int]
    LEVEL_FIELD_NUMBER: _ClassVar[int]
    IS_DEFAULT_FIELD_NUMBER: _ClassVar[int]
    id: int
    platform_account_id: str
    game_biz: str
    region: str
    player_id: str
    nickname: str
    level: int
    is_default: bool
    def __init__(self, id: _Optional[int] = ..., platform_account_id: _Optional[str] = ..., game_biz: _Optional[str] = ..., region: _Optional[str] = ..., player_id: _Optional[str] = ..., nickname: _Optional[str] = ..., level: _Optional[int] = ..., is_default: _Optional[bool] = ...) -> None: ...

class GetCredentialSummaryRequest(_message.Message):
    __slots__ = ("service_ticket", "platform_account_id")
    SERVICE_TICKET_FIELD_NUMBER: _ClassVar[int]
    PLATFORM_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    service_ticket: str
    platform_account_id: str
    def __init__(self, service_ticket: _Optional[str] = ..., platform_account_id: _Optional[str] = ...) -> None: ...

class GetCredentialSummaryResponse(_message.Message):
    __slots__ = ("platform_account_id", "status", "last_validated_at", "last_refreshed_at", "devices", "profiles")
    PLATFORM_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    LAST_VALIDATED_AT_FIELD_NUMBER: _ClassVar[int]
    LAST_REFRESHED_AT_FIELD_NUMBER: _ClassVar[int]
    DEVICES_FIELD_NUMBER: _ClassVar[int]
    PROFILES_FIELD_NUMBER: _ClassVar[int]
    platform_account_id: str
    status: CredentialStatus
    last_validated_at: _timestamp_pb2.Timestamp
    last_refreshed_at: _timestamp_pb2.Timestamp
    devices: _containers.RepeatedCompositeFieldContainer[DeviceSummary]
    profiles: _containers.RepeatedCompositeFieldContainer[ProfileSummary]
    def __init__(self, platform_account_id: _Optional[str] = ..., status: _Optional[_Union[CredentialStatus, str]] = ..., last_validated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., last_refreshed_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., devices: _Optional[_Iterable[_Union[DeviceSummary, _Mapping]]] = ..., profiles: _Optional[_Iterable[_Union[ProfileSummary, _Mapping]]] = ...) -> None: ...

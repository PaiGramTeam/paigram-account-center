import datetime

from google.protobuf import struct_pb2 as _struct_pb2
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

class DescribePlatformRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class DescribePlatformResponse(_message.Message):
    __slots__ = ("platform_key", "display_name", "service_audience", "supported_actions", "credential_schema", "version")
    PLATFORM_KEY_FIELD_NUMBER: _ClassVar[int]
    DISPLAY_NAME_FIELD_NUMBER: _ClassVar[int]
    SERVICE_AUDIENCE_FIELD_NUMBER: _ClassVar[int]
    SUPPORTED_ACTIONS_FIELD_NUMBER: _ClassVar[int]
    CREDENTIAL_SCHEMA_FIELD_NUMBER: _ClassVar[int]
    VERSION_FIELD_NUMBER: _ClassVar[int]
    platform_key: str
    display_name: str
    service_audience: str
    supported_actions: _containers.RepeatedScalarFieldContainer[str]
    credential_schema: _struct_pb2.Struct
    version: str
    def __init__(self, platform_key: _Optional[str] = ..., display_name: _Optional[str] = ..., service_audience: _Optional[str] = ..., supported_actions: _Optional[_Iterable[str]] = ..., credential_schema: _Optional[_Union[_struct_pb2.Struct, _Mapping]] = ..., version: _Optional[str] = ...) -> None: ...

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
    __slots__ = ("platform_account_id",)
    PLATFORM_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    platform_account_id: str
    def __init__(self, platform_account_id: _Optional[str] = ...) -> None: ...

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

class GetCredentialStatusRequest(_message.Message):
    __slots__ = ("platform_account_id",)
    PLATFORM_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    platform_account_id: str
    def __init__(self, platform_account_id: _Optional[str] = ...) -> None: ...

class GetCredentialStatusResponse(_message.Message):
    __slots__ = ("status", "last_validated_at")
    STATUS_FIELD_NUMBER: _ClassVar[int]
    LAST_VALIDATED_AT_FIELD_NUMBER: _ClassVar[int]
    status: CredentialStatus
    last_validated_at: _timestamp_pb2.Timestamp
    def __init__(self, status: _Optional[_Union[CredentialStatus, str]] = ..., last_validated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class ValidateCredentialRequest(_message.Message):
    __slots__ = ("platform_account_id",)
    PLATFORM_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    platform_account_id: str
    def __init__(self, platform_account_id: _Optional[str] = ...) -> None: ...

class ValidateCredentialResponse(_message.Message):
    __slots__ = ("status", "error_code")
    STATUS_FIELD_NUMBER: _ClassVar[int]
    ERROR_CODE_FIELD_NUMBER: _ClassVar[int]
    status: CredentialStatus
    error_code: str
    def __init__(self, status: _Optional[_Union[CredentialStatus, str]] = ..., error_code: _Optional[str] = ...) -> None: ...

class ListProfilesRequest(_message.Message):
    __slots__ = ("platform_account_id",)
    PLATFORM_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    platform_account_id: str
    def __init__(self, platform_account_id: _Optional[str] = ...) -> None: ...

class ListProfilesResponse(_message.Message):
    __slots__ = ("profiles",)
    PROFILES_FIELD_NUMBER: _ClassVar[int]
    profiles: _containers.RepeatedCompositeFieldContainer[ProfileSummary]
    def __init__(self, profiles: _Optional[_Iterable[_Union[ProfileSummary, _Mapping]]] = ...) -> None: ...

class GetPrimaryProfileRequest(_message.Message):
    __slots__ = ("platform_account_id",)
    PLATFORM_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    platform_account_id: str
    def __init__(self, platform_account_id: _Optional[str] = ...) -> None: ...

class GetPrimaryProfileResponse(_message.Message):
    __slots__ = ("profile",)
    PROFILE_FIELD_NUMBER: _ClassVar[int]
    profile: ProfileSummary
    def __init__(self, profile: _Optional[_Union[ProfileSummary, _Mapping]] = ...) -> None: ...

class ConfirmPrimaryProfileRequest(_message.Message):
    __slots__ = ("platform_account_id", "player_id")
    PLATFORM_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    PLAYER_ID_FIELD_NUMBER: _ClassVar[int]
    platform_account_id: str
    player_id: str
    def __init__(self, platform_account_id: _Optional[str] = ..., player_id: _Optional[str] = ...) -> None: ...

class ConfirmPrimaryProfileResponse(_message.Message):
    __slots__ = ("profile",)
    PROFILE_FIELD_NUMBER: _ClassVar[int]
    profile: ProfileSummary
    def __init__(self, profile: _Optional[_Union[ProfileSummary, _Mapping]] = ...) -> None: ...

class GetAuthKeyRequest(_message.Message):
    __slots__ = ("platform_account_id", "player_id")
    PLATFORM_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    PLAYER_ID_FIELD_NUMBER: _ClassVar[int]
    platform_account_id: str
    player_id: str
    def __init__(self, platform_account_id: _Optional[str] = ..., player_id: _Optional[str] = ...) -> None: ...

class GetAuthKeyResponse(_message.Message):
    __slots__ = ("authkey", "expires_at")
    AUTHKEY_FIELD_NUMBER: _ClassVar[int]
    EXPIRES_AT_FIELD_NUMBER: _ClassVar[int]
    authkey: str
    expires_at: _timestamp_pb2.Timestamp
    def __init__(self, authkey: _Optional[str] = ..., expires_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class DeviceInfo(_message.Message):
    __slots__ = ("device_id", "device_fp", "device_name")
    DEVICE_ID_FIELD_NUMBER: _ClassVar[int]
    DEVICE_FP_FIELD_NUMBER: _ClassVar[int]
    DEVICE_NAME_FIELD_NUMBER: _ClassVar[int]
    device_id: str
    device_fp: str
    device_name: str
    def __init__(self, device_id: _Optional[str] = ..., device_fp: _Optional[str] = ..., device_name: _Optional[str] = ...) -> None: ...

class UpsertDeviceRequest(_message.Message):
    __slots__ = ("platform_account_id", "device")
    PLATFORM_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    DEVICE_FIELD_NUMBER: _ClassVar[int]
    platform_account_id: str
    device: DeviceInfo
    def __init__(self, platform_account_id: _Optional[str] = ..., device: _Optional[_Union[DeviceInfo, _Mapping]] = ...) -> None: ...

class UpsertDeviceResponse(_message.Message):
    __slots__ = ("success",)
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    success: bool
    def __init__(self, success: _Optional[bool] = ...) -> None: ...

class BindCredentialRequest(_message.Message):
    __slots__ = ("credential_payload_json",)
    CREDENTIAL_PAYLOAD_JSON_FIELD_NUMBER: _ClassVar[int]
    credential_payload_json: str
    def __init__(self, credential_payload_json: _Optional[str] = ...) -> None: ...

class BindCredentialResponse(_message.Message):
    __slots__ = ("summary",)
    SUMMARY_FIELD_NUMBER: _ClassVar[int]
    summary: GetCredentialSummaryResponse
    def __init__(self, summary: _Optional[_Union[GetCredentialSummaryResponse, _Mapping]] = ...) -> None: ...

class ReplaceCredentialRequest(_message.Message):
    __slots__ = ("platform_account_id", "credential_payload_json")
    PLATFORM_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    CREDENTIAL_PAYLOAD_JSON_FIELD_NUMBER: _ClassVar[int]
    platform_account_id: str
    credential_payload_json: str
    def __init__(self, platform_account_id: _Optional[str] = ..., credential_payload_json: _Optional[str] = ...) -> None: ...

class ReplaceCredentialResponse(_message.Message):
    __slots__ = ("summary",)
    SUMMARY_FIELD_NUMBER: _ClassVar[int]
    summary: GetCredentialSummaryResponse
    def __init__(self, summary: _Optional[_Union[GetCredentialSummaryResponse, _Mapping]] = ...) -> None: ...

class RefreshCredentialRequest(_message.Message):
    __slots__ = ("platform_account_id",)
    PLATFORM_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    platform_account_id: str
    def __init__(self, platform_account_id: _Optional[str] = ...) -> None: ...

class RefreshCredentialResponse(_message.Message):
    __slots__ = ("status", "refreshed_at")
    STATUS_FIELD_NUMBER: _ClassVar[int]
    REFRESHED_AT_FIELD_NUMBER: _ClassVar[int]
    status: CredentialStatus
    refreshed_at: _timestamp_pb2.Timestamp
    def __init__(self, status: _Optional[_Union[CredentialStatus, str]] = ..., refreshed_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class DeleteCredentialRequest(_message.Message):
    __slots__ = ("platform_account_id",)
    PLATFORM_ACCOUNT_ID_FIELD_NUMBER: _ClassVar[int]
    platform_account_id: str
    def __init__(self, platform_account_id: _Optional[str] = ...) -> None: ...

class DeleteCredentialResponse(_message.Message):
    __slots__ = ("success",)
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    success: bool
    def __init__(self, success: _Optional[bool] = ...) -> None: ...

class InvalidateConsumerGrantRequest(_message.Message):
    __slots__ = ("binding_id", "consumer", "minimum_grant_version")
    BINDING_ID_FIELD_NUMBER: _ClassVar[int]
    CONSUMER_FIELD_NUMBER: _ClassVar[int]
    MINIMUM_GRANT_VERSION_FIELD_NUMBER: _ClassVar[int]
    binding_id: int
    consumer: str
    minimum_grant_version: int
    def __init__(self, binding_id: _Optional[int] = ..., consumer: _Optional[str] = ..., minimum_grant_version: _Optional[int] = ...) -> None: ...

class InvalidateConsumerGrantResponse(_message.Message):
    __slots__ = ("success",)
    SUCCESS_FIELD_NUMBER: _ClassVar[int]
    success: bool
    def __init__(self, success: _Optional[bool] = ...) -> None: ...

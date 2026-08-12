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

class OperationKind(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    OPERATION_KIND_UNSPECIFIED: _ClassVar[OperationKind]
    OPERATION_KIND_BIND_CREDENTIAL: _ClassVar[OperationKind]
    OPERATION_KIND_REPLACE_CREDENTIAL: _ClassVar[OperationKind]
    OPERATION_KIND_REFRESH_CREDENTIAL: _ClassVar[OperationKind]
    OPERATION_KIND_DELETE_CREDENTIAL: _ClassVar[OperationKind]
    OPERATION_KIND_APPLY_AUTHORIZATION_FENCE: _ClassVar[OperationKind]

class OperationState(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    OPERATION_STATE_UNSPECIFIED: _ClassVar[OperationState]
    OPERATION_STATE_PENDING: _ClassVar[OperationState]
    OPERATION_STATE_SUCCEEDED: _ClassVar[OperationState]
    OPERATION_STATE_FAILED: _ClassVar[OperationState]
    OPERATION_STATE_NOT_RECEIVED: _ClassVar[OperationState]
    OPERATION_STATE_FAILED_INPUT_REQUIRED: _ClassVar[OperationState]
CREDENTIAL_STATUS_UNSPECIFIED: CredentialStatus
CREDENTIAL_STATUS_ACTIVE: CredentialStatus
CREDENTIAL_STATUS_EXPIRED: CredentialStatus
CREDENTIAL_STATUS_INVALID: CredentialStatus
CREDENTIAL_STATUS_CHALLENGE_REQUIRED: CredentialStatus
OPERATION_KIND_UNSPECIFIED: OperationKind
OPERATION_KIND_BIND_CREDENTIAL: OperationKind
OPERATION_KIND_REPLACE_CREDENTIAL: OperationKind
OPERATION_KIND_REFRESH_CREDENTIAL: OperationKind
OPERATION_KIND_DELETE_CREDENTIAL: OperationKind
OPERATION_KIND_APPLY_AUTHORIZATION_FENCE: OperationKind
OPERATION_STATE_UNSPECIFIED: OperationState
OPERATION_STATE_PENDING: OperationState
OPERATION_STATE_SUCCEEDED: OperationState
OPERATION_STATE_FAILED: OperationState
OPERATION_STATE_NOT_RECEIVED: OperationState
OPERATION_STATE_FAILED_INPUT_REQUIRED: OperationState

class BindingResource(_message.Message):
    __slots__ = ("binding_ref", "account_key")
    BINDING_REF_FIELD_NUMBER: _ClassVar[int]
    ACCOUNT_KEY_FIELD_NUMBER: _ClassVar[int]
    binding_ref: str
    account_key: str
    def __init__(self, binding_ref: _Optional[str] = ..., account_key: _Optional[str] = ...) -> None: ...

class DeviceSummary(_message.Message):
    __slots__ = ("device_ref", "device_name", "is_valid", "last_seen_at")
    DEVICE_REF_FIELD_NUMBER: _ClassVar[int]
    DEVICE_NAME_FIELD_NUMBER: _ClassVar[int]
    IS_VALID_FIELD_NUMBER: _ClassVar[int]
    LAST_SEEN_AT_FIELD_NUMBER: _ClassVar[int]
    device_ref: str
    device_name: str
    is_valid: bool
    last_seen_at: _timestamp_pb2.Timestamp
    def __init__(self, device_ref: _Optional[str] = ..., device_name: _Optional[str] = ..., is_valid: _Optional[bool] = ..., last_seen_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class ProfileSummary(_message.Message):
    __slots__ = ("profile_ref", "account_key", "game_biz", "region", "player_id", "nickname", "level", "is_default")
    PROFILE_REF_FIELD_NUMBER: _ClassVar[int]
    ACCOUNT_KEY_FIELD_NUMBER: _ClassVar[int]
    GAME_BIZ_FIELD_NUMBER: _ClassVar[int]
    REGION_FIELD_NUMBER: _ClassVar[int]
    PLAYER_ID_FIELD_NUMBER: _ClassVar[int]
    NICKNAME_FIELD_NUMBER: _ClassVar[int]
    LEVEL_FIELD_NUMBER: _ClassVar[int]
    IS_DEFAULT_FIELD_NUMBER: _ClassVar[int]
    profile_ref: str
    account_key: str
    game_biz: str
    region: str
    player_id: str
    nickname: str
    level: int
    is_default: bool
    def __init__(self, profile_ref: _Optional[str] = ..., account_key: _Optional[str] = ..., game_biz: _Optional[str] = ..., region: _Optional[str] = ..., player_id: _Optional[str] = ..., nickname: _Optional[str] = ..., level: _Optional[int] = ..., is_default: _Optional[bool] = ...) -> None: ...

class ProfileSnapshot(_message.Message):
    __slots__ = ("profiles", "complete", "revision", "observed_revision")
    PROFILES_FIELD_NUMBER: _ClassVar[int]
    COMPLETE_FIELD_NUMBER: _ClassVar[int]
    REVISION_FIELD_NUMBER: _ClassVar[int]
    OBSERVED_REVISION_FIELD_NUMBER: _ClassVar[int]
    profiles: _containers.RepeatedCompositeFieldContainer[ProfileSummary]
    complete: bool
    revision: int
    observed_revision: int
    def __init__(self, profiles: _Optional[_Iterable[_Union[ProfileSummary, _Mapping]]] = ..., complete: _Optional[bool] = ..., revision: _Optional[int] = ..., observed_revision: _Optional[int] = ...) -> None: ...

class OperationRef(_message.Message):
    __slots__ = ("operation_id", "kind", "binding_ref", "pre_generation", "target_generation", "request_fingerprint")
    OPERATION_ID_FIELD_NUMBER: _ClassVar[int]
    KIND_FIELD_NUMBER: _ClassVar[int]
    BINDING_REF_FIELD_NUMBER: _ClassVar[int]
    PRE_GENERATION_FIELD_NUMBER: _ClassVar[int]
    TARGET_GENERATION_FIELD_NUMBER: _ClassVar[int]
    REQUEST_FINGERPRINT_FIELD_NUMBER: _ClassVar[int]
    operation_id: str
    kind: OperationKind
    binding_ref: str
    pre_generation: int
    target_generation: int
    request_fingerprint: str
    def __init__(self, operation_id: _Optional[str] = ..., kind: _Optional[_Union[OperationKind, str]] = ..., binding_ref: _Optional[str] = ..., pre_generation: _Optional[int] = ..., target_generation: _Optional[int] = ..., request_fingerprint: _Optional[str] = ...) -> None: ...

class OperationResult(_message.Message):
    __slots__ = ("operation", "state", "reason_code", "account_key", "credential_status", "profile_snapshot", "updated_at", "last_validated_at", "last_refreshed_at")
    OPERATION_FIELD_NUMBER: _ClassVar[int]
    STATE_FIELD_NUMBER: _ClassVar[int]
    REASON_CODE_FIELD_NUMBER: _ClassVar[int]
    ACCOUNT_KEY_FIELD_NUMBER: _ClassVar[int]
    CREDENTIAL_STATUS_FIELD_NUMBER: _ClassVar[int]
    PROFILE_SNAPSHOT_FIELD_NUMBER: _ClassVar[int]
    UPDATED_AT_FIELD_NUMBER: _ClassVar[int]
    LAST_VALIDATED_AT_FIELD_NUMBER: _ClassVar[int]
    LAST_REFRESHED_AT_FIELD_NUMBER: _ClassVar[int]
    operation: OperationRef
    state: OperationState
    reason_code: str
    account_key: str
    credential_status: CredentialStatus
    profile_snapshot: ProfileSnapshot
    updated_at: _timestamp_pb2.Timestamp
    last_validated_at: _timestamp_pb2.Timestamp
    last_refreshed_at: _timestamp_pb2.Timestamp
    def __init__(self, operation: _Optional[_Union[OperationRef, _Mapping]] = ..., state: _Optional[_Union[OperationState, str]] = ..., reason_code: _Optional[str] = ..., account_key: _Optional[str] = ..., credential_status: _Optional[_Union[CredentialStatus, str]] = ..., profile_snapshot: _Optional[_Union[ProfileSnapshot, _Mapping]] = ..., updated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., last_validated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., last_refreshed_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class BindingState(_message.Message):
    __slots__ = ("exists", "binding_ref", "account_key", "credential_generation", "credential_status", "profile_snapshot", "last_validated_at", "last_refreshed_at")
    EXISTS_FIELD_NUMBER: _ClassVar[int]
    BINDING_REF_FIELD_NUMBER: _ClassVar[int]
    ACCOUNT_KEY_FIELD_NUMBER: _ClassVar[int]
    CREDENTIAL_GENERATION_FIELD_NUMBER: _ClassVar[int]
    CREDENTIAL_STATUS_FIELD_NUMBER: _ClassVar[int]
    PROFILE_SNAPSHOT_FIELD_NUMBER: _ClassVar[int]
    LAST_VALIDATED_AT_FIELD_NUMBER: _ClassVar[int]
    LAST_REFRESHED_AT_FIELD_NUMBER: _ClassVar[int]
    exists: bool
    binding_ref: str
    account_key: str
    credential_generation: int
    credential_status: CredentialStatus
    profile_snapshot: ProfileSnapshot
    last_validated_at: _timestamp_pb2.Timestamp
    last_refreshed_at: _timestamp_pb2.Timestamp
    def __init__(self, exists: _Optional[bool] = ..., binding_ref: _Optional[str] = ..., account_key: _Optional[str] = ..., credential_generation: _Optional[int] = ..., credential_status: _Optional[_Union[CredentialStatus, str]] = ..., profile_snapshot: _Optional[_Union[ProfileSnapshot, _Mapping]] = ..., last_validated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., last_refreshed_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

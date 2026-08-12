import datetime

from google.protobuf import struct_pb2 as _struct_pb2
from google.protobuf import timestamp_pb2 as _timestamp_pb2
from paigram_account_sdk._generated.platform.v2 import types_pb2 as _types_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class DescribePlatformRequest(_message.Message):
    __slots__ = ()
    def __init__(self) -> None: ...

class DescribePlatformResponse(_message.Message):
    __slots__ = ("platform_key", "display_name", "service_audience", "supported_actions", "credential_schema", "contract_version")
    PLATFORM_KEY_FIELD_NUMBER: _ClassVar[int]
    DISPLAY_NAME_FIELD_NUMBER: _ClassVar[int]
    SERVICE_AUDIENCE_FIELD_NUMBER: _ClassVar[int]
    SUPPORTED_ACTIONS_FIELD_NUMBER: _ClassVar[int]
    CREDENTIAL_SCHEMA_FIELD_NUMBER: _ClassVar[int]
    CONTRACT_VERSION_FIELD_NUMBER: _ClassVar[int]
    platform_key: str
    display_name: str
    service_audience: str
    supported_actions: _containers.RepeatedScalarFieldContainer[str]
    credential_schema: _struct_pb2.Struct
    contract_version: str
    def __init__(self, platform_key: _Optional[str] = ..., display_name: _Optional[str] = ..., service_audience: _Optional[str] = ..., supported_actions: _Optional[_Iterable[str]] = ..., credential_schema: _Optional[_Union[_struct_pb2.Struct, _Mapping]] = ..., contract_version: _Optional[str] = ...) -> None: ...

class GetStatusRequest(_message.Message):
    __slots__ = ("resource",)
    RESOURCE_FIELD_NUMBER: _ClassVar[int]
    resource: _types_pb2.BindingResource
    def __init__(self, resource: _Optional[_Union[_types_pb2.BindingResource, _Mapping]] = ...) -> None: ...

class GetStatusResponse(_message.Message):
    __slots__ = ("status", "last_validated_at")
    STATUS_FIELD_NUMBER: _ClassVar[int]
    LAST_VALIDATED_AT_FIELD_NUMBER: _ClassVar[int]
    status: _types_pb2.CredentialStatus
    last_validated_at: _timestamp_pb2.Timestamp
    def __init__(self, status: _Optional[_Union[_types_pb2.CredentialStatus, str]] = ..., last_validated_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class ValidateCredentialRequest(_message.Message):
    __slots__ = ("resource",)
    RESOURCE_FIELD_NUMBER: _ClassVar[int]
    resource: _types_pb2.BindingResource
    def __init__(self, resource: _Optional[_Union[_types_pb2.BindingResource, _Mapping]] = ...) -> None: ...

class ValidateCredentialResponse(_message.Message):
    __slots__ = ("status", "reason_code")
    STATUS_FIELD_NUMBER: _ClassVar[int]
    REASON_CODE_FIELD_NUMBER: _ClassVar[int]
    status: _types_pb2.CredentialStatus
    reason_code: str
    def __init__(self, status: _Optional[_Union[_types_pb2.CredentialStatus, str]] = ..., reason_code: _Optional[str] = ...) -> None: ...

class ListProfilesRequest(_message.Message):
    __slots__ = ("resource",)
    RESOURCE_FIELD_NUMBER: _ClassVar[int]
    resource: _types_pb2.BindingResource
    def __init__(self, resource: _Optional[_Union[_types_pb2.BindingResource, _Mapping]] = ...) -> None: ...

class ListProfilesResponse(_message.Message):
    __slots__ = ("snapshot",)
    SNAPSHOT_FIELD_NUMBER: _ClassVar[int]
    snapshot: _types_pb2.ProfileSnapshot
    def __init__(self, snapshot: _Optional[_Union[_types_pb2.ProfileSnapshot, _Mapping]] = ...) -> None: ...

class GetPrimaryProfileRequest(_message.Message):
    __slots__ = ("resource",)
    RESOURCE_FIELD_NUMBER: _ClassVar[int]
    resource: _types_pb2.BindingResource
    def __init__(self, resource: _Optional[_Union[_types_pb2.BindingResource, _Mapping]] = ...) -> None: ...

class GetPrimaryProfileResponse(_message.Message):
    __slots__ = ("profile",)
    PROFILE_FIELD_NUMBER: _ClassVar[int]
    profile: _types_pb2.ProfileSummary
    def __init__(self, profile: _Optional[_Union[_types_pb2.ProfileSummary, _Mapping]] = ...) -> None: ...

class GetAuthKeyRequest(_message.Message):
    __slots__ = ("resource", "profile_ref")
    RESOURCE_FIELD_NUMBER: _ClassVar[int]
    PROFILE_REF_FIELD_NUMBER: _ClassVar[int]
    resource: _types_pb2.BindingResource
    profile_ref: str
    def __init__(self, resource: _Optional[_Union[_types_pb2.BindingResource, _Mapping]] = ..., profile_ref: _Optional[str] = ...) -> None: ...

class GetAuthKeyResponse(_message.Message):
    __slots__ = ("authkey", "expires_at")
    AUTHKEY_FIELD_NUMBER: _ClassVar[int]
    EXPIRES_AT_FIELD_NUMBER: _ClassVar[int]
    authkey: str
    expires_at: _timestamp_pb2.Timestamp
    def __init__(self, authkey: _Optional[str] = ..., expires_at: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ...) -> None: ...

class GetDeviceRequest(_message.Message):
    __slots__ = ("resource", "device_ref")
    RESOURCE_FIELD_NUMBER: _ClassVar[int]
    DEVICE_REF_FIELD_NUMBER: _ClassVar[int]
    resource: _types_pb2.BindingResource
    device_ref: str
    def __init__(self, resource: _Optional[_Union[_types_pb2.BindingResource, _Mapping]] = ..., device_ref: _Optional[str] = ...) -> None: ...

class GetDeviceResponse(_message.Message):
    __slots__ = ("device",)
    DEVICE_FIELD_NUMBER: _ClassVar[int]
    device: _types_pb2.DeviceSummary
    def __init__(self, device: _Optional[_Union[_types_pb2.DeviceSummary, _Mapping]] = ...) -> None: ...

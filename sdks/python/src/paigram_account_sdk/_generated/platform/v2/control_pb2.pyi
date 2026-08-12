from paigram_account_sdk._generated.platform.v2 import types_pb2 as _types_pb2
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class BindCredentialRequest(_message.Message):
    __slots__ = ("operation", "credential_payload_json")
    OPERATION_FIELD_NUMBER: _ClassVar[int]
    CREDENTIAL_PAYLOAD_JSON_FIELD_NUMBER: _ClassVar[int]
    operation: _types_pb2.OperationRef
    credential_payload_json: str
    def __init__(self, operation: _Optional[_Union[_types_pb2.OperationRef, _Mapping]] = ..., credential_payload_json: _Optional[str] = ...) -> None: ...

class BindCredentialResponse(_message.Message):
    __slots__ = ("result",)
    RESULT_FIELD_NUMBER: _ClassVar[int]
    result: _types_pb2.OperationResult
    def __init__(self, result: _Optional[_Union[_types_pb2.OperationResult, _Mapping]] = ...) -> None: ...

class ReplaceCredentialRequest(_message.Message):
    __slots__ = ("operation", "account_key", "credential_payload_json")
    OPERATION_FIELD_NUMBER: _ClassVar[int]
    ACCOUNT_KEY_FIELD_NUMBER: _ClassVar[int]
    CREDENTIAL_PAYLOAD_JSON_FIELD_NUMBER: _ClassVar[int]
    operation: _types_pb2.OperationRef
    account_key: str
    credential_payload_json: str
    def __init__(self, operation: _Optional[_Union[_types_pb2.OperationRef, _Mapping]] = ..., account_key: _Optional[str] = ..., credential_payload_json: _Optional[str] = ...) -> None: ...

class ReplaceCredentialResponse(_message.Message):
    __slots__ = ("result",)
    RESULT_FIELD_NUMBER: _ClassVar[int]
    result: _types_pb2.OperationResult
    def __init__(self, result: _Optional[_Union[_types_pb2.OperationResult, _Mapping]] = ...) -> None: ...

class RefreshCredentialRequest(_message.Message):
    __slots__ = ("operation", "account_key")
    OPERATION_FIELD_NUMBER: _ClassVar[int]
    ACCOUNT_KEY_FIELD_NUMBER: _ClassVar[int]
    operation: _types_pb2.OperationRef
    account_key: str
    def __init__(self, operation: _Optional[_Union[_types_pb2.OperationRef, _Mapping]] = ..., account_key: _Optional[str] = ...) -> None: ...

class RefreshCredentialResponse(_message.Message):
    __slots__ = ("result",)
    RESULT_FIELD_NUMBER: _ClassVar[int]
    result: _types_pb2.OperationResult
    def __init__(self, result: _Optional[_Union[_types_pb2.OperationResult, _Mapping]] = ...) -> None: ...

class DeleteCredentialRequest(_message.Message):
    __slots__ = ("operation", "account_key")
    OPERATION_FIELD_NUMBER: _ClassVar[int]
    ACCOUNT_KEY_FIELD_NUMBER: _ClassVar[int]
    operation: _types_pb2.OperationRef
    account_key: str
    def __init__(self, operation: _Optional[_Union[_types_pb2.OperationRef, _Mapping]] = ..., account_key: _Optional[str] = ...) -> None: ...

class DeleteCredentialResponse(_message.Message):
    __slots__ = ("result",)
    RESULT_FIELD_NUMBER: _ClassVar[int]
    result: _types_pb2.OperationResult
    def __init__(self, result: _Optional[_Union[_types_pb2.OperationResult, _Mapping]] = ...) -> None: ...

class ApplyAuthorizationFenceRequest(_message.Message):
    __slots__ = ("operation", "consumer_principal", "minimum_grant_version", "minimum_owner_epoch", "minimum_consumer_epoch", "minimum_entry_epoch")
    OPERATION_FIELD_NUMBER: _ClassVar[int]
    CONSUMER_PRINCIPAL_FIELD_NUMBER: _ClassVar[int]
    MINIMUM_GRANT_VERSION_FIELD_NUMBER: _ClassVar[int]
    MINIMUM_OWNER_EPOCH_FIELD_NUMBER: _ClassVar[int]
    MINIMUM_CONSUMER_EPOCH_FIELD_NUMBER: _ClassVar[int]
    MINIMUM_ENTRY_EPOCH_FIELD_NUMBER: _ClassVar[int]
    operation: _types_pb2.OperationRef
    consumer_principal: str
    minimum_grant_version: int
    minimum_owner_epoch: int
    minimum_consumer_epoch: int
    minimum_entry_epoch: int
    def __init__(self, operation: _Optional[_Union[_types_pb2.OperationRef, _Mapping]] = ..., consumer_principal: _Optional[str] = ..., minimum_grant_version: _Optional[int] = ..., minimum_owner_epoch: _Optional[int] = ..., minimum_consumer_epoch: _Optional[int] = ..., minimum_entry_epoch: _Optional[int] = ...) -> None: ...

class ApplyAuthorizationFenceResponse(_message.Message):
    __slots__ = ("result",)
    RESULT_FIELD_NUMBER: _ClassVar[int]
    result: _types_pb2.OperationResult
    def __init__(self, result: _Optional[_Union[_types_pb2.OperationResult, _Mapping]] = ...) -> None: ...

class SetPrimaryProfileRequest(_message.Message):
    __slots__ = ("operation", "account_key", "profile_ref", "expected_profile_revision")
    OPERATION_FIELD_NUMBER: _ClassVar[int]
    ACCOUNT_KEY_FIELD_NUMBER: _ClassVar[int]
    PROFILE_REF_FIELD_NUMBER: _ClassVar[int]
    EXPECTED_PROFILE_REVISION_FIELD_NUMBER: _ClassVar[int]
    operation: _types_pb2.OperationRef
    account_key: str
    profile_ref: str
    expected_profile_revision: int
    def __init__(self, operation: _Optional[_Union[_types_pb2.OperationRef, _Mapping]] = ..., account_key: _Optional[str] = ..., profile_ref: _Optional[str] = ..., expected_profile_revision: _Optional[int] = ...) -> None: ...

class SetPrimaryProfileResponse(_message.Message):
    __slots__ = ("result",)
    RESULT_FIELD_NUMBER: _ClassVar[int]
    result: _types_pb2.OperationResult
    def __init__(self, result: _Optional[_Union[_types_pb2.OperationResult, _Mapping]] = ...) -> None: ...

class GetOperationRequest(_message.Message):
    __slots__ = ("operation_id",)
    OPERATION_ID_FIELD_NUMBER: _ClassVar[int]
    operation_id: str
    def __init__(self, operation_id: _Optional[str] = ...) -> None: ...

class GetOperationResponse(_message.Message):
    __slots__ = ("result",)
    RESULT_FIELD_NUMBER: _ClassVar[int]
    result: _types_pb2.OperationResult
    def __init__(self, result: _Optional[_Union[_types_pb2.OperationResult, _Mapping]] = ...) -> None: ...

class ResolveOperationRequest(_message.Message):
    __slots__ = ("operation",)
    OPERATION_FIELD_NUMBER: _ClassVar[int]
    operation: _types_pb2.OperationRef
    def __init__(self, operation: _Optional[_Union[_types_pb2.OperationRef, _Mapping]] = ...) -> None: ...

class ResolveOperationResponse(_message.Message):
    __slots__ = ("result",)
    RESULT_FIELD_NUMBER: _ClassVar[int]
    result: _types_pb2.OperationResult
    def __init__(self, result: _Optional[_Union[_types_pb2.OperationResult, _Mapping]] = ...) -> None: ...

class GetBindingStateRequest(_message.Message):
    __slots__ = ("binding_ref",)
    BINDING_REF_FIELD_NUMBER: _ClassVar[int]
    binding_ref: str
    def __init__(self, binding_ref: _Optional[str] = ...) -> None: ...

class GetBindingStateResponse(_message.Message):
    __slots__ = ("state",)
    STATE_FIELD_NUMBER: _ClassVar[int]
    state: _types_pb2.BindingState
    def __init__(self, state: _Optional[_Union[_types_pb2.BindingState, _Mapping]] = ...) -> None: ...

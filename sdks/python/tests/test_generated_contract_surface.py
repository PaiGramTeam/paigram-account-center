from dataclasses import fields
from importlib.util import find_spec

from paigram_account_sdk import PlatformBinding
from paigram_account_sdk._generated.account.v1 import bot_access_pb2


def test_deprecated_mihomo_credential_transport_is_not_published() -> None:
    assert find_spec("paigram_account_sdk._generated.mihomo") is None


def test_legacy_bot_binding_write_contract_is_not_published() -> None:
    assert not hasattr(bot_access_pb2, "UpsertPlatformBindingRequest")
    assert not hasattr(bot_access_pb2, "UpsertPlatformBindingResponse")
    assert bot_access_pb2.PlatformAccountBinding.DESCRIPTOR.fields_by_name.get("meta_json") is None
    assert "meta_json" not in {field.name for field in fields(PlatformBinding)}

from dataclasses import fields
from importlib.util import find_spec

from paigram_account_sdk import PlatformBinding
from paigram_account_sdk._generated.account.v1 import bot_access_pb2


def test_only_v2_platform_runtime_transport_is_published() -> None:
    assert find_spec("paigram_account_sdk._generated.mihomo.v2") is not None
    assert find_spec("paigram_account_sdk._generated.platform.v2") is not None
    assert find_spec("paigram_account_sdk._generated.platform.v1") is None


def test_legacy_bot_binding_write_contract_is_not_published() -> None:
    assert not hasattr(bot_access_pb2, "UpsertPlatformBindingRequest")
    assert not hasattr(bot_access_pb2, "UpsertPlatformBindingResponse")
    assert bot_access_pb2.PlatformAccountBinding.DESCRIPTOR.fields_by_name.get("meta_json") is None
    public_fields = {field.name for field in fields(PlatformBinding)}
    assert "meta_json" not in public_fields
    assert "id" not in public_fields
    assert "user_id" not in public_fields
    assert {"binding_ref", "account_key", "generation"} <= public_fields

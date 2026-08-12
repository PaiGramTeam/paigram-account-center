from importlib.util import find_spec


def test_deprecated_mihomo_credential_transport_is_not_published() -> None:
    assert find_spec("paigram_account_sdk._generated.mihomo") is None

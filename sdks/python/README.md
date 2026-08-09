# PaiGram Account SDK

PaiGram 使用的异步 Python SDK。它负责：

- 使用 OAuth 2.0 client credentials 向 Account Center 获取机器令牌；
- 查询 PaiGram 用户及其可访问的平台绑定；
- 为指定绑定签发短期 service ticket；
- 使用 service ticket 调用已配置的平台服务。

SDK 不提供写入原始平台凭据的接口。凭据录入与所有权管理属于 Account Center 的第一方前端和服务端流程。

```python
from paigram_account_sdk import PaiGramAccountClient, PlatformEndpoint

async with PaiGramAccountClient(
    account_http_url="https://account.example.com",
    account_grpc_target="account.example.com:443",
    client_id="paigram",
    client_secret="...",
    platform_endpoints={
        "mihomo": PlatformEndpoint(target="mihomo.example.com:443"),
    },
) as client:
    bindings = await client.list_bindings("telegram:10001")
    summary = await client.get_credential_summary(
        external_user_id="telegram:10001",
        binding=bindings[0],
    )
```

本地验证：

```powershell
uv sync --all-groups
uv run pytest
uv run ruff check .
uv run mypy
uv build
```

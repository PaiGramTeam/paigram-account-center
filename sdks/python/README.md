# PaiGram Account SDK

PaiGram 使用的异步 Python SDK。它负责：

- 使用 OAuth 2.0 client credentials 向 Account Center 获取机器令牌；
- 查询 PaiGram 用户及其可访问的平台绑定；
- 为当前外部用户创建短期、单次的 Web 身份关联批准链接；
- 为指定绑定签发短期 service ticket；
- 使用 service ticket 查询凭据状态、校验凭据、读取角色与主角色；
- 获取短期 AuthKey，并按稳定设备引用读取设备状态。

SDK 不提供写入原始平台凭据的接口。凭据录入与所有权管理属于 Account Center 的第一方前端和服务端流程。
身份关联 URL 的 fragment 包含一次性 bearer challenge；只能私发给对应外部用户，不得写入日志、分析事件或群聊消息。

```python
from paigram_account_sdk import PaiGramAccountClient

async with PaiGramAccountClient(
    account_http_url="https://account.example.com",
    account_grpc_target="account.example.com:443",
    account_grpc_server_name="account.example.com",
    client_id="telegram-service",
    client_secret="...",
) as client:
    link = await client.start_entry_identity_link(
        "telegram:10001",
        external_username="traveler",
    )
    # Send link.approval_url only to this external user in a private message.
    bindings = await client.list_bindings("telegram:10001")
    status = await client.get_credential_status(
        external_user_id="telegram:10001",
        binding=bindings[0],
    )
    profiles = await client.list_profiles(
        external_user_id="telegram:10001",
        binding=bindings[0],
    )
    authkey = await client.get_auth_key(
        external_user_id="telegram:10001",
        binding=bindings[0],
        profile_ref=profiles[0].profile_ref,
    )
```

`client_id` 表示一个 OAuth 消费者凭据；账号中心会把它映射到独立的逻辑 `bot_id`。同一个 Bot 可以使用多个服务凭据，但平台授权按消费者 `client_id` 独立授予和撤销。

SDK 不公开刷新、删除或写入平台原始凭据的方法。消费者只能请求注册平台声明的读取/运行时动作，所有权操作必须走账号中心的用户端或管理端接口。

平台 runtime endpoint、可选 TLS server name、audience 与 action catalog 由 SDK 使用机器令牌从 Account Center registry 获取，并与 Platform descriptor 交叉校验。Account 与 Platform gRPC 默认使用明文传输；传入 `account_root_certificates`（或 `account_grpc_server_name`）可为 Account 启用 TLS，`platform_root_certificates={"platform-mihomo-service": ca_pem}` 则只为映射中的平台启用 TLS。映射中的值可为 `None` 以使用系统信任库。显式 TLS 连接失败时不会回退明文，且内部服务连接不继承 HTTP 代理环境变量。

本地验证：

```powershell
uv sync --all-groups
uv run pytest
uv run ruff check .
uv run mypy
uv build
```

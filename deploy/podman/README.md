# Podman 单机部署

该部署配置包含用户端、管理端、Account Center、Platform Mihomo，以及两套独立的 PostgreSQL/Redis。默认发布 Nginx 的统一 HTTP 入口和仅绑定回环地址的 Platform gRPC 端口；数据库、Redis 与 Account Center 后端端口只在 Podman 内部网络可达。

## 准备环境

需要 Podman 以及 `podman compose` 可用的 Compose provider。在 PowerShell 7 中生成本地配置：

```powershell
cd deploy/podman
./init-env.ps1
```

脚本生成被 Git 忽略的 `.env`，其中包含两套数据库/Redis 凭据、匹配的 Ed25519 票据密钥对、两套数据加密密钥和初始管理员凭据。部署前必须把 `PAI_MIHOMO_UPSTREAM_BASE_URL` 改成真实 HTTPS 上游，并检查 `PAI_HTTP_BIND`、`PAI_HTTP_PORT`、`PAI_FRONTEND_BASE_URL` 和管理员信息。默认 HTTP 与 Platform gRPC 分别只监听 `127.0.0.1:18080` 和 `127.0.0.1:19000`。

Platform 启动后，管理员需在“服务注册”中幂等创建 `mihomo` registry 项，内部端点填写 `platform-mihomo:9000`，service audience 填写 `platform-mihomo-service`，支持动作以 Platform `DescribePlatform` 返回值为准。当前单机 Compose 仅用于预发布联调；在向不可信网络开放 gRPC 前，仍必须完成计划中的 control/runtime 双入口 TLS/mTLS 切分。

预发布模板默认 `PAI_REQUIRE_EMAIL_VERIFICATION=false`，避免在未配置 SMTP 时创建无法登录的待验证账号。要启用强制验证，必须同时把它改为 `true`、将 `PAI_FRONTEND_BASE_URL` 设为外部 HTTPS 地址，并在 Account Center 配置中启用有效的邮件服务；release 模式会对这组不变量 fail closed。

默认 Podman 网段为 `10.90.0.0/24`。运行前使用 `podman network ls` 和本机路由表确认它未与其他容器网络、VPN 或局域网重叠；多个实例必须使用不同的 `PAI_INSTANCE`、端口和网段。

## 构建与启动

```powershell
./deploy.ps1
```

部署脚本会构建全部镜像、启动服务、等待 Account Center/Platform 健康检查，并确认数据库、Redis 与 Account Center 没有发布宿主机端口。用户端位于 `/`，管理端位于 `/admin/`，API 与 OpenAPI 文档由同一入口反向代理。服务就绪后可检查：

已有同名、同标签镜像时，可使用 `./deploy.ps1 -NoBuild` 跳过构建，仅更新并验证运行容器。

```powershell
$settings = Get-Content .env -Raw | ConvertFrom-StringData
$instance = $settings.PAI_INSTANCE
$healthHost = if ($settings.PAI_HTTP_BIND -eq "0.0.0.0") { "127.0.0.1" } else { $settings.PAI_HTTP_BIND }

Invoke-RestMethod "http://${healthHost}:$($settings.PAI_HTTP_PORT)/healthz"
podman ps --filter "label=com.docker.compose.project=$instance"
Invoke-WebRequest "http://${healthHost}:$($settings.PAI_HTTP_PORT)/"
Invoke-WebRequest "http://${healthHost}:$($settings.PAI_HTTP_PORT)/admin/"
podman port "$instance-frontend"
podman port $instance
podman port "$instance-postgres"
podman port "$instance-redis"
podman port "$instance-platform-mihomo"
podman port "$instance-platform-postgres"
podman port "$instance-platform-redis"
```

Account Center、两套数据库与两套 Redis 的 `podman port` 应无输出。前端 Nginx 和 Platform Mihomo 只应显示各自显式配置的回环地址映射。

停止服务但保留数据卷：

```powershell
$settings = Get-Content .env -Raw | ConvertFrom-StringData
podman compose --env-file .env -p $settings.PAI_INSTANCE -f compose.yaml down
```

数据保存在以 `PAI_INSTANCE` 为前缀的命名卷中。删除这些卷会永久删除数据库和 Redis 数据，因此常规停止或升级不应附加 `--volumes`。

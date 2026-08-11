# Podman 单机部署

该部署配置只发布 Account Center HTTP 端口。PostgreSQL、Redis 和 Account Center gRPC 端口仅在 Podman 内部网络中可达，不映射到宿主机。前端与 Platform Mihomo 保持仓库既定的独立部署边界，不包含在此 Compose 项目中。

## 准备环境

需要 Podman 以及 `podman compose` 可用的 Compose provider。在 PowerShell 7 中生成本地配置：

```powershell
cd deploy/podman
./init-env.ps1
```

脚本生成被 Git 忽略的 `.env`，其中包含数据库、Redis、服务票据、2FA 加密和初始管理员的随机凭据。部署前应检查 `PAI_HTTP_BIND`、`PAI_HTTP_PORT`、`PAI_FRONTEND_BASE_URL` 和管理员信息。默认只监听宿主机回环地址 `127.0.0.1:18080`；需要由反向代理或局域网访问时，再显式调整绑定地址和防火墙。

默认 Podman 网段为 `10.90.0.0/24`。运行前使用 `podman network ls` 和本机路由表确认它未与其他容器网络、VPN 或局域网重叠；多个实例必须使用不同的 `PAI_INSTANCE`、端口和网段。

## 构建与启动

```powershell
./deploy.ps1
```

部署脚本会构建 Account Center 镜像、启动服务、等待健康检查，并确认 PostgreSQL 与 Redis 没有发布宿主机端口。服务就绪后可检查：

已有同名、同标签镜像时，可使用 `./deploy.ps1 -NoBuild` 跳过构建，仅更新并验证运行容器。

```powershell
$settings = Get-Content .env -Raw | ConvertFrom-StringData
$instance = $settings.PAI_INSTANCE
$healthHost = if ($settings.PAI_HTTP_BIND -eq "0.0.0.0") { "127.0.0.1" } else { $settings.PAI_HTTP_BIND }

Invoke-RestMethod "http://${healthHost}:$($settings.PAI_HTTP_PORT)/healthz"
podman ps --filter "label=com.docker.compose.project=$instance"
podman port $instance
podman port "$instance-postgres"
podman port "$instance-redis"
```

最后两个 `podman port` 命令应无输出。只有 Account Center 容器应显示 `8080/tcp` 的宿主机映射。

停止服务但保留数据卷：

```powershell
$settings = Get-Content .env -Raw | ConvertFrom-StringData
podman compose --env-file .env -p $settings.PAI_INSTANCE -f compose.yaml down
```

数据保存在以 `PAI_INSTANCE` 为前缀的命名卷中。删除这些卷会永久删除数据库和 Redis 数据，因此常规停止或升级不应附加 `--volumes`。

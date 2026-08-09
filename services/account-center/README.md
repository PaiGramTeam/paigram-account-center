# Paigram Account Center

用户中心系统，为 PaiGram 系列机器人提供集中的用户数据管理，同时为终端用户提供账号管理界面。

HTTP 服务由 Gin 承载中间件和已有处理器，并通过 Huma 注册业务路由、生成 OpenAPI 文档。数据持久化使用 PostgreSQL。

本地配置可从 `config/config.yaml.example` 开始。默认启用 `/openapi` 文档页面和 `/openapi.json` 规范端点。

无需数据库或 Redis 即可把同一套路由契约导出到仓库：

```powershell
go run ./cmd/paigram openapi --out ../../contracts/openapi.json
```

HTTP 路由或请求/响应模型变化后，应同时重新生成 `contracts/openapi.json` 和前端的 OpenAPI TypeScript 类型。

集成测试使用 Testcontainers 启动共享的 PostgreSQL 与 Redis 容器，并为每个测试从已迁移的基线库克隆隔离数据库：

```powershell
go test -tags=integration ./integration
```

本机默认使用 Podman；Docker 环境设置 `PAI_TESTCONTAINERS_PROVIDER=docker`。

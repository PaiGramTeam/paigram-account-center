# PaiGram Account Center Frontend

Account Center 的第一方管理界面，包含用户端、管理员端和共享模块。

## 本地开发

安装锁定依赖：

```powershell
bun install --frozen-lockfile
```

启动应用：

```powershell
bun run dev:user
bun run dev:admin
```

不依赖后端启动浏览器内 Mock：

```powershell
bun run dev:user:mock
bun run dev:admin:mock
```

Mock 模式使用 MSW 拦截 `/api/v1` 请求。可通过 `?mockScenario=error` 或 `?mockScenario=slow` 切换错误、延迟场景。

执行完整的前端本地验证：

```powershell
bun run format:check
bun run lint
bun run type-check
bun run test
bun run build:all
```

执行 Playwright 路由拦截测试和浏览器 Mock 测试：

```powershell
bun run e2e
bun run e2e:mock
```

测试默认由 Playwright 管理独立的 Vite 服务。仅在确认对应端口已有开发服务器时，可设置
`PLAYWRIGHT_REUSE_SERVER=1` 复用现有进程。

首次运行前需要安装 Chromium：

```powershell
bunx playwright install chromium
```

## OpenAPI 类型

先从配套的 `services/account-center` 后端导出仓库内的权威规范：

```powershell
Push-Location ../services/account-center
go run ./cmd/paigram openapi --out ../../contracts/openapi.json
Pop-Location
```

再从 `contracts/openapi.json` 生成共享 TypeScript 类型：

```powershell
bun run openapi:gen
```

`contracts/openapi.json` 和生成的 `packages/shared-components/src/api/generated/schema.ts` 都应随 HTTP 契约变更一并更新。

用户端默认将 `/api` 代理到 `http://localhost:8080`。生产环境的 Account Center 地址通过 `VITE_API_BASE_URL` 提供。

## Workspace

- `packages/user-app`：用户端 Vue 应用。
- `packages/admin-app`：管理员端 Vue 应用。
- `packages/shared-components`：共享布局、状态、类型和 Account Center HTTP 客户端。

本目录只维护本地构建所需的 Bun、TypeScript、Vite、ESLint 和 Prettier 配置，不包含远端 CI/CD 或部署配置。

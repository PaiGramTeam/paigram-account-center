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

执行完整的前端本地验证：

```powershell
bun run format:check
bun run lint
bun run type-check
bun run test
bun run build:all
```

用户端默认将 `/api` 代理到 `http://localhost:8080`。生产环境的 Account Center 地址通过 `VITE_API_BASE_URL` 提供。

## Workspace

- `packages/user-app`：用户端 Vue 应用。
- `packages/admin-app`：管理员端 Vue 应用。
- `packages/shared-components`：共享布局、状态、类型和 Account Center HTTP 客户端。

本目录只维护本地构建所需的 Bun、TypeScript、Vite、ESLint 和 Prettier 配置，不包含远端 CI/CD 或部署配置。

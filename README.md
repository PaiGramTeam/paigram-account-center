# PaiGram Account Center

PaiGram Account Center is the product repository for unified PaiGram identity, authorization, platform account ownership, Mihomo credential operations, first-party management interfaces, and bot-consumer access.

The repository is a source-control boundary, not a runtime monolith. Account Center and Platform Mihomo remain independent processes with separate databases, configuration, credentials, and release lifecycles.

## Repository layout

```text
services/account-center/       Account Center service
services/platform-mihomo/      Mihomo platform service
contracts/proto/               Cross-service Protocol Buffer sources
contracts/gen/go/              Generated Go contract module
sdks/python/                   Async Python consumer SDK
frontend/                      User and administrator web applications
scripts/                       Local verification entrypoints
```

## Dependency direction

- The frontend calls Account Center's public interface.
- Account Center owns users, roles, bindings, grants, and short-lived service tickets.
- Platform Mihomo owns raw credentials, devices, refresh state, and platform lifecycle data.
- The Python SDK hides transport and generated contracts from PaiGram consumers.
- PaiGram remains an independent repository and pins an immutable SDK Git revision with uv.

Raw platform credentials must never be stored by Account Center, the SDK, or bot consumers.

## Local verification

Required local tools are Go, Buf, Bun, uv, `protoc-gen-go`, and `protoc-gen-go-grpc`. Docker is not required by the repository verification flow.

Run the complete local verification suite from the repository root:

```powershell
pwsh ./scripts/verify.ps1
```

The default verification checks the pinned current PaiGram `main` baseline and requires a clean worktree. During development, `-AllowDirty` permits intentional local edits and `-SkipPaiGramCompatibility` permits an offline run.

The repository intentionally does not copy build or deployment systems from the source repositories.

Regenerate Go and Python contract bindings directly when a `.proto` file changes:

```powershell
pwsh ./contracts/generate.ps1
```

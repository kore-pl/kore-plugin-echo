# Changelog

本仓库遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/) 与语义化版本。

## [0.1.1] - 2026-11-22

### 修复

- 依赖 `github.com/pengtang1024/kore/pkg/pluginapi` 升至 `v0.1.1`。`v0.1.0` 的 `go.mod` 声明旧 module 路径 `github.com/kore-project/kore/pkg/pluginapi`，已被 `retract`，消费者无法解析。
- 脱离本地 `go.work` 后可构建：`GOWORK=off go build ./...` / `GOWORK=off make build` / `GOWORK=off make dist` 全部通过，干净 clone 可复现。

### 说明

- 无代码与协议变更；`info.json` 仅版本号递增（0.1.0 → 0.1.1），`protocol` / `capabilities` / `triggers` / `requires_kore` 不变。

## [0.1.0] - 2026-11-22

### 新增

- 首个 M1 验证插件：`install → hello → invoke → 崩溃重启` 链路验证件（`depth_level = basic`，非生产级）。
- `kore.hello` 三步握手：发 `kore.hello`（§5 全字段 + `id`/`token` 别名）→ 读响应帧 → 校验 `ok`/`id`/协议主版本。
- 会话循环：读帧 → 按 `method` 分发 → 写 `response`，回显 `id` 与 `req_id`；未知方法回 `E_NOTFOUND`。
- 验证方法：`ping`、`echo`、`slow`（1s）、`timeout`（3s）、`crash`（退出码 7）；协议必选方法 `kore.ping` / `kore.reload` / `kore.shutdown`。
- `Makefile`：`build` / `dist`（回填 `size_bytes`、生成包内 `SHA256SUMS` 与包旁 `.sha256`）/ `check` / `clean`。
- 单测：`net.Pipe` 模拟主壳侧，覆盖握手、`ping`、`echo` 原样回显与 `req_id` 回显、未知方法 `E_NOTFOUND`。

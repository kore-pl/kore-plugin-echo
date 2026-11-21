# Changelog

本仓库遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/) 与语义化版本。

## [0.1.0] - 2026-11-22

### 新增

- 首个 M1 验证插件：`install → hello → invoke → 崩溃重启` 链路验证件（`depth_level = basic`，非生产级）。
- `kore.hello` 三步握手：发 `kore.hello`（§5 全字段 + `id`/`token` 别名）→ 读响应帧 → 校验 `ok`/`id`/协议主版本。
- 会话循环：读帧 → 按 `method` 分发 → 写 `response`，回显 `id` 与 `req_id`；未知方法回 `E_NOTFOUND`。
- 验证方法：`ping`、`echo`、`slow`（1s）、`timeout`（3s）、`crash`（退出码 7）；协议必选方法 `kore.ping` / `kore.reload` / `kore.shutdown`。
- `Makefile`：`build` / `dist`（回填 `size_bytes`、生成包内 `SHA256SUMS` 与包旁 `.sha256`）/ `check` / `clean`。
- 单测：`net.Pipe` 模拟主壳侧，覆盖握手、`ping`、`echo` 原样回显与 `req_id` 回显、未知方法 `E_NOTFOUND`。

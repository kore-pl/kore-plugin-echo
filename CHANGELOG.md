# Changelog

本仓库遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/) 与语义化版本。

## 0.1.6 — 2026-11-25

### 新增

- 补齐七语言使用说明（`docs/DETAIL*.md`）与插件元数据（`locale/*.json`），README 顶部新增语言切换行；新增 SECURITY 英文译文。
- 补齐 dependabot、CODEOWNERS 与本地化分支模型、SSH 签名约定说明。

### 变更

- 发起带 SSH 签名的 `v0.1.6` 版本 tag；仅文档与工程标准化，不改协议、能力与方法契约，历史 tag 不动、不推送。

## [0.1.5] - 2026-11-24

### 变更

- **`depth_level`：`production` → `basic`**。依据 `kore/docs/PLUGIN-CATALOG.md` §6.1 升级判据①（边界用例：异常输入 / 并发 / 权限不足 / 资源耗尽各 ≥1 例并进 CI）**不满足**——本仓无 `tests/boundary/`，`grep -rn "t.Parallel" tests/` 命中 0。故障注入（判据②）已通过，但两条须同时满足；此前声明 `production` 属**过度声明**，索引的 `basic` 才是正确值。**非降级缺陷**，不涉及代码、能力与方法契约变化；补齐边界用例后再升 production。

## [0.1.4] - 2026-11-23

### 新增

- 补 `tests/fault/`：对 echo 做**进程级故障注入**——用假主壳（UDS + 真帧协议）起真二进制，覆盖握手超时 / 主壳拒握手 / 对端断开 / 非法参数 / 未知方法 / `crash` 约定退出码 / `kore.shutdown` 优雅退出等 10 个用例。保护"主壳侧异常时插件不能 hang、不能谎报成功"。
- `depth_level`：`basic` → `production`（G-20：production 插件必须有 `tests/fault/`，本仓此前缺该目录）。

### 说明

- 无方法 / 协议 / 能力变更：`capabilities` / `triggers` / `requires_kore` / `lifecycle` 均不变；`id@version` 换 sha 故 patch bump。

## [0.1.3] - 2026-11-23

### 修复

- `info.json.lifecycle`：`resident` → `lazy`。echo 是验证插件，只在验收调用时被唤起，常驻会白占一个 resident 槽位（默认上限 2）。实测 `site` + `terminal` 两个 resident 占满后，再启用 echo 直接 `E_RATELIMIT`（HTTP 429），迫使验收前先停掉业务插件腾槽。**验证工具不该消耗被验证的资源。**
- 改为 `lazy` 后：enable 只登记 `desired=enabled` 不起进程；`echo.ping` 首次调用按需冷启，空闲 `idle_timeout` 后自动退出并归还槽位（槽位口径见内核 `internal/core/supervisor/lazy.go`）。

### 说明

- 无方法 / 协议 / 能力变更：`protocol` / `capabilities` / `triggers` / `requires_kore` 不变，`id@version` 换 sha 故 patch bump。

## [0.1.2] - 2026-11-23

- build: dist 去除 ./ 根条目以通过安装器校验
- build: dist 改为确定性打包（sha256 可复现）
- build: 位级可复现打包（stage chmod + tar --mode 归一、size_bytes 收敛），二进制改嵌 manifest.json，消除同版本 sha 漂移

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

# kore-plugin-echo

kore 的 **M1 验证插件**：用于验证主壳的 `install → hello → invoke → 崩溃重启` 全链路，
不提供任何运维业务能力，**不是生产级插件**（`info.json.depth_level = "basic"`）。

## 定位与边界

- 协议：`kore.plugin.v1`，`lifecycle = lazy`（按需冷启、空闲退出），唯一依赖 `github.com/pengtang1024/kore/pkg/pluginapi` + 标准库。
- 生命周期取 `lazy` 而非 `resident`：本插件是**验证工具**，只在验收时被 `echo.ping` 唤起；常驻会白占一个 resident 槽位（默认上限 2），导致 `site` + `terminal` 已占满时无法再跑验收，真机上表现为 `E_RATELIMIT`（HTTP 429）。**验证工具不该消耗被验证的资源。**
- 只做四件事：完成 `kore.hello` 握手、响应调用、按约定方法模拟慢调用 / 超时 / 崩溃、收到 `kore.shutdown` 优雅退出。
- 不读写内核 `internal/`，不联网、不写状态库、不落任何业务数据。

## 方法表

| 方法（裸名 / 全名） | 行为 |
| --- | --- |
| `ping` / `echo.ping` | 返回 `{"pong":true}` |
| `echo` / `echo.echo` | 原样返回 `params`（非 JSON 对象回 `E_INVALID`） |
| `slow` / `echo.slow` | 睡眠 1s 后返回 `{"slept_ms":1000}` |
| `timeout` / `echo.timeout` | 睡眠 3s 后返回 `{"slept_ms":3000}`（验证调用超时） |
| `crash` / `echo.crash` | 立即 `os.Exit(7)`（验证重启计数） |
| `kore.ping` / `kore.reload` / `kore.shutdown` | 协议必选方法；`kore.reload` 无外部配置，`config_digest` 为空 |

响应帧一律回显请求的 `id`（连接内匹配）与 `req_id`（跨进程追踪）；未知方法回 `E_NOTFOUND`。

## 环境变量

| 变量 | 必填 | 说明 |
| --- | --- | --- |
| `KORE_PLUGIN_SOCK` | 是 | 主壳创建的 UDS 路径 |
| `KORE_PLUGIN_ID` | 否 | 内核登记 id，缺省取内嵌 `info.json.id` |
| `KORE_PLUGIN_TOKEN` | 否 | 内核注入令牌；M1 只随 `kore.hello` 上报，内核尚未校验 |
| `KORE_LOG_LEVEL` | 否 | `debug`/`info`/`warn`/`error`，缺省 `warn`；日志只写 stderr |

`stdout` 只留给协议帧，任何日志都不得写 stdout。

## 构建与打包

```bash
make build   # CGO_ENABLED=0 GOAMD64=v1 go build -trimpath -buildvcs=false -ldflags="-s -w" -o bin/echo .
make dist    # 产出 dist/echo-0.1.5.tar.gz（info.json + bin/echo + web/ + SHA256SUMS）与同名 .sha256
make check   # gofmt → go vet → go test -timeout 200s
```

`dist` 会按包内实际字节数回填 `info.json.size_bytes`（仓库里的清单保持 `0`，由打包脚本写入真实值）。

本地联调依赖**不提交**的 `go.work`（`use .` + `use ../kore/pkg/pluginapi`）；`pkg/pluginapi`
发布前删除 `go.work` 会让 `go mod download` 无法解析该 module。跨仓 `replace` 禁止写入 `go.mod`（G-17）。

## 已知限制

- kore M1 未实现心跳循环与重启计数：`kore.ping` 已按 §7.1 实现但暂无调用方；`crash` 的退出码 `7` 供 supervisor 观测进程异常退出。
- `scenario_tags` 使用 `ops`：kore `registry` 白名单为 `web|data|ops|sec|app`，不包含任务书初稿里的 `dev`。
- `web/` 为空目录：M1 验证插件没有前端。
- 仅 Linux 为 Tier A；macOS / Windows 未做插件侧交叉编译验证，`rss_kb` 在非 Linux 平台返回 `0`。

<!-- oss:begin -->
## 开源与贡献

[![CI](https://github.com/kore-pl/kore-plugin-echo/actions/workflows/ci.yml/badge.svg)](https://github.com/kore-pl/kore-plugin-echo/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go 1.26](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](go.mod)

本仓库是 **kore** 的 `Echo 验证插件`（id `echo`），独立仓库、独立版本线、独立 CI：GitHub Actions 只手动触发（`workflow_dispatch`），按版本 tag 自动触发在 Gitea Actions（`.gitea/workflows/ci.yml`，push `v*` + 手动）。内核与协议见 [kore](https://github.com/kore-pl/kore)。

- **许可**：MIT（inbound = outbound，不要求 CLA）；提交用 `git commit -s` 签 DCO。
- **贡献**：[CONTRIBUTING.md](CONTRIBUTING.md)（构建、门禁、发版、边界）。
- **安全**：[SECURITY.md](SECURITY.md)——漏洞不要开公开 issue。
- **行为准则**：[CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)。
- **兼容性**：`protocol: kore.plugin.v1` · `requires_kore: >=0.1.0 <0.2.0` · `lifecycle: lazy` · 深度等级 `basic`。
- **验证方式**：本地 `make check` 优先（GitHub CI 只手动触发，Gitea CI 按版本 tag 触发；发布前 `make dist` 连跑两次核对 sha 一致）。
- **发布产物**：`make dist` → `dist/echo-0.1.5.tar.gz` + `.sha256`（位级可复现，连跑两次 sha 相同）。
- **相关仓库**：[索引 kore-plugins](https://github.com/kore-pl/kore-plugins) · [协议文档](https://github.com/kore-pl/kore/blob/main/docs/PLUGIN-PROTOCOL.md) · [SDK 指南](https://github.com/kore-pl/kore/blob/main/docs/PLUGIN-SDK.md)

<!-- oss:end -->

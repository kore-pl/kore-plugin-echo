# kore-plugin-echo 使用说明

kore 的 M1 验证插件（id `echo`）：验证主壳 `install → hello → invoke → 崩溃重启` 全链路。不提供任何运维业务能力，**不是生产级插件**（`depth_level=basic`）。

## 安装与兼容性

由 kore 内核安装发布的 tar.gz，也可独立构建（`make build` / `make dist`）。协议 `kore.plugin.v1`，`requires_kore: >=0.1.0 <0.2.0`，`lifecycle=lazy`——验证工具只在验收时被唤起，不占常驻槽位。只做四件事：握手、响应调用、模拟慢调用/超时/崩溃、收到 `kore.shutdown` 优雅退出。不读写内核 `internal/`、不联网、不落业务数据。

## 方法

| 方法 | 行为 |
| --- | --- |
| `ping` / `echo.ping` | 返回 `{"pong":true}` |
| `echo` / `echo.echo` | 原样返回 `params`（非 JSON 对象回 `E_INVALID`） |
| `slow` / `echo.slow` | 睡 1s 后返回 `{"slept_ms":1000}` |
| `timeout` / `echo.timeout` | 睡 3s 后返回 `{"slept_ms":3000}`，验证调用超时 |
| `crash` / `echo.crash` | 立即 `os.Exit(7)`，验证重启计数 |
| `kore.ping/reload/shutdown` | 协议必选方法；`kore.reload` 无外部配置，`config_digest` 为空 |

响应帧回显请求的 `id` 与 `req_id`；未知方法回 `E_NOTFOUND`。`stdout` 只留给协议帧，日志一律写 stderr。

## 环境变量

| 变量 | 必填 | 说明 |
| --- | --- | --- |
| `KORE_PLUGIN_SOCK` | 是 | 主壳创建的 UDS 路径 |
| `KORE_PLUGIN_ID` | 否 | 缺省取内嵌 `info.json.id` |
| `KORE_PLUGIN_TOKEN` | 否 | M1 只随 `kore.hello` 上报，内核尚未校验 |
| `KORE_LOG_LEVEL` | 否 | debug/info/warn/error，缺省 warn |

## 已知限制

- kore M1 未实现心跳循环与重启计数；`crash` 的退出码 `7` 供 supervisor 观测。
- `web/` 为空：验证插件没有前端。
- Linux 为 Tier A；其它平台未做交叉编译验证，`rss_kb` 返回 0。

## 构建与验证

```bash
make check   # gofmt + go vet + go test -timeout 200s（含 race）
make build && make dist
```

## 安全与贡献

漏洞不要开公开 issue，按 [SECURITY.md](SECURITY.md) 私密披露；贡献见 [CONTRIBUTING.md](CONTRIBUTING.md)。MIT 许可。

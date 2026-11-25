# Echo 验证插件 Wiki

语言：简体中文（本 Wiki 以简体中文为主，完整七语言说明见仓库 `docs/DETAIL*.md`）。

页面：[Home](https://github.com/kore-pl/kore-plugin-echo/wiki/Home) · [Installation](https://github.com/kore-pl/kore-plugin-echo/wiki/Installation) · [Configuration](https://github.com/kore-pl/kore-plugin-echo/wiki/Configuration) · [Usage](https://github.com/kore-pl/kore-plugin-echo/wiki/Usage) · [FAQ](https://github.com/kore-pl/kore-plugin-echo/wiki/FAQ) · [Changelog](https://github.com/kore-pl/kore-plugin-echo/wiki/Changelog)

## 方法一览

- `ping / echo.ping → {"pong":true}`
- `echo / echo.echo → 原样返回 params`
- `slow / echo.slow → 睡 1s`
- `timeout / echo.timeout → 睡 3s`
- `crash / echo.crash → os.Exit(7)`
- `kore.ping / kore.reload / kore.shutdown`

完整入参/返回字段见仓库 README 的方法表与 [DETAIL 中文说明](https://github.com/kore-pl/kore-plugin-echo/blob/main/docs/DETAIL.md)。

## 典型流程

1. 主壳按 `lifecycle=lazy` 冷启插件并完成 `kore.hello` 握手。
2. 调用方经 UDS 发方法调用，请求带 `id` 与 `req_id`；响应帧回显两者。
3. 未知方法返回 `E_NOTFOUND`；参数非法返回 `E_INVALID`；能力不足返回相应错误码。
4. 空闲后 `kore.shutdown` 优雅退出，或由主壳回收。

## 验证

仓库 `make check` 跑 gofmt、go vet、测试与 race 门禁；E2E 与故障注入用例见仓库 `tests/`。`make dist` 连跑两次 sha 应一致。

## 多语言文档

[简体中文](https://github.com/kore-pl/kore-plugin-echo/blob/main/docs/DETAIL.md) · [English](https://github.com/kore-pl/kore-plugin-echo/blob/main/docs/DETAIL.en.md) · [Español](https://github.com/kore-pl/kore-plugin-echo/blob/main/docs/DETAIL.es.md) · [Français](https://github.com/kore-pl/kore-plugin-echo/blob/main/docs/DETAIL.fr.md) · [Deutsch](https://github.com/kore-pl/kore-plugin-echo/blob/main/docs/DETAIL.de.md) · [Русский](https://github.com/kore-pl/kore-plugin-echo/blob/main/docs/DETAIL.ru.md) · [Português](https://github.com/kore-pl/kore-plugin-echo/blob/main/docs/DETAIL.pt.md)

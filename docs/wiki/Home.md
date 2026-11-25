# Echo 验证插件 Wiki

语言：简体中文（本 Wiki 以简体中文为主，完整七语言说明见仓库 `docs/DETAIL*.md`）。

页面：[Home](https://github.com/kore-pl/kore-plugin-echo/wiki/Home) · [Installation](https://github.com/kore-pl/kore-plugin-echo/wiki/Installation) · [Configuration](https://github.com/kore-pl/kore-plugin-echo/wiki/Configuration) · [Usage](https://github.com/kore-pl/kore-plugin-echo/wiki/Usage) · [FAQ](https://github.com/kore-pl/kore-plugin-echo/wiki/FAQ) · [Changelog](https://github.com/kore-pl/kore-plugin-echo/wiki/Changelog)

## 这是什么

Echo 验证插件（id `echo`）是 kore 的M1 全链路验证工具（非生产级）：验证主壳 install → hello → invoke → 崩溃重启全链路；不提供运维业务能力。

## 核心能力

- 完成 kore.hello 握手
- 回显请求 id 与 req_id
- sleep 1s / 3s 模拟慢调用与超时
- crash 立即 os.Exit(7) 验证重启计数
- 收到 kore.shutdown 优雅退出
- lazy 生命周期：不占 resident 槽位

协议 `kore.plugin.v1`，`lifecycle=lazy`，深度等级见仓库 `info.json`。内核、协议与 SDK 文档见 [kore](https://github.com/kore-pl/kore)。

## 快速开始

1. 安装：见 [Installation](https://github.com/kore-pl/kore-plugin-echo/wiki/Installation)。
2. 配置：见 [Configuration](https://github.com/kore-pl/kore-plugin-echo/wiki/Configuration)。
3. 调用：见 [Usage](https://github.com/kore-pl/kore-plugin-echo/wiki/Usage) 与仓库 README 的方法表。
4. 遇到问题：见 [FAQ](https://github.com/kore-pl/kore-plugin-echo/wiki/FAQ)。

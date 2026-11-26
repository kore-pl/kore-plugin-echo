# Echo 验证插件 Wiki

语言：简体中文（本 Wiki 以简体中文为主，完整七语言说明见仓库 `docs/DETAIL*.md`）。

页面：[Home](https://github.com/kore-pl/kore-plugin-echo/wiki/Home) · [Installation](https://github.com/kore-pl/kore-plugin-echo/wiki/Installation) · [Configuration](https://github.com/kore-pl/kore-plugin-echo/wiki/Configuration) · [Usage](https://github.com/kore-pl/kore-plugin-echo/wiki/Usage) · [FAQ](https://github.com/kore-pl/kore-plugin-echo/wiki/FAQ) · [Changelog](https://github.com/kore-pl/kore-plugin-echo/wiki/Changelog)

## 常见问题

### 这是生产插件吗？

不是。它是 M1 验证工具，depth_level=production（故障注入与边界用例均通过），不承担运维业务。

### 为什么用 lazy？

验证工具只在验收时被唤起，常驻会白占 resident 槽位导致真实验收 E_RATELIMIT。

### crash 之后谁来重启？

由主壳 supervisor 观察退出码 7 并重启；M1 尚未实现心跳与重启计数。

### 去哪里报 bug / 提需求？

在仓库开 issue；安全漏洞不要开公开 issue，按 [SECURITY.md](https://github.com/kore-pl/kore-plugin-echo/blob/main/SECURITY.md) 私密披露。

//go:build !faultinject

package main

// maybeInjectPanic 默认是空操作；只有 -tags faultinject 的进程级故障测试二进制
// 会让保留方法 echo.__panic_probe 触发 panic（见 faultinject.go）。

func maybeInjectPanic(string) {}

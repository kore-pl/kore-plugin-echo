//go:build faultinject

package main

// maybeInjectPanic 是故障注入测试专用的 panic 钩子：仅 -tags faultinject 构建生效，
// 生产二进制（-tags 缺省）里这个方法名仍走 E_NOTFOUND。
func maybeInjectPanic(method string) {
	if method == "echo.__panic_probe" || method == "__panic_probe" {
		panic("fault injection: method panic probe")
	}
}

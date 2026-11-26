package fault

import (
	"encoding/binary"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// sendRequest 发一帧请求并读回对应响应。
func sendRequest(t *testing.T, conn net.Conn, method string, params json.RawMessage) *pluginapi.Response {
	t.Helper()
	request := &pluginapi.Request{Type: pluginapi.TypeRequest, ID: "req-1", ReqID: "r-1", Method: method, Params: params}
	if err := pluginapi.WriteJSON(conn, request); err != nil {
		t.Fatalf("写请求失败: %v", err)
	}
	payload, err := pluginapi.ReadFrame(conn, pluginapi.MaxFrameSize)
	if err != nil {
		t.Fatalf("读响应失败: %v", err)
	}
	env, err := pluginapi.DecodeFrame(payload)
	if err != nil {
		t.Fatalf("解码响应失败: %v", err)
	}
	response, ok := env.(*pluginapi.Response)
	if !ok {
		t.Fatalf("期望 response 帧，实得 %T", env)
	}
	if response.ID != request.ID {
		t.Fatalf("响应 id=%q，期望 %q", response.ID, request.ID)
	}
	return response
}

// invoke 执行一次成功调用，失败即测试失败。
func invoke(t *testing.T, conn net.Conn, method string, params json.RawMessage) *pluginapi.Response {
	t.Helper()
	response := sendRequest(t, conn, method, params)
	if response.Error != nil {
		t.Fatalf("%s 意外失败: %s %s", method, response.Error.Code, response.Error.Message)
	}
	return response
}

// startAndHandshake 启动插件进程并完成一次成功握手（顺序不能反：accept 会阻塞到插件连上来）。
func startAndHandshake(t *testing.T, kernel *fakeKernel) (net.Conn, *exec.Cmd, *safeBuffer) {
	t.Helper()
	cmd, stderr := startPlugin(t, kernel.socket)
	conn := kernel.accept(t)
	hello := readRequest(t, conn)
	if hello.Method != "kore.hello" {
		t.Fatalf("首帧方法 = %q，期望 kore.hello", hello.Method)
	}
	replyHello(t, conn, hello, true, pluginapi.Protocol)
	return conn, cmd, stderr
}

// TestStartupMissingSocketEnv：缺少 KORE_PLUGIN_SOCK 时按配置错误退出 1，而不是 hang 或空跑。
func TestStartupMissingSocketEnv(t *testing.T) {
	cmd := exec.Command(binaryPath)
	cmd.Env = append(os.Environ(), "GOWORK=off", "KORE_PLUGIN_ID=echo")
	stderr := &safeBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动失败: %v", err)
	}
	if code := waitExit(t, cmd, 5*time.Second); code != 1 {
		t.Fatalf("退出码 = %d，期望 1；stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "KORE_PLUGIN_SOCK") {
		t.Fatalf("stderr 未提示缺少 socket 环境变量: %s", stderr.String())
	}
}

// TestStartupSocketAbsent：主壳 socket 不存在时立即失败退出 1（不重试、不假装就绪）。
func TestStartupSocketAbsent(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.sock")
	cmd, stderr := startPlugin(t, missing)
	if code := waitExit(t, cmd, 5*time.Second); code != 1 {
		t.Fatalf("退出码 = %d，期望 1；stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "连接主壳 socket") {
		t.Fatalf("stderr 未提示连接失败: %s", stderr.String())
	}
}

// TestHandshakeTimeout：主壳收下 hello 却不回响应时，插件必须在 hello 预算内退出 1（不能无限等）。
func TestHandshakeTimeout(t *testing.T) {
	kernel := newFakeKernel(t)
	cmd, stderr := startPlugin(t, kernel.socket)
	conn := kernel.accept(t)
	_ = readRequest(t, conn) // 读掉 hello，故意不回
	started := time.Now()
	code := waitExit(t, cmd, 6*time.Second)
	elapsed := time.Since(started)
	if code != 1 {
		t.Fatalf("退出码 = %d，期望 1；stderr=%s", code, stderr.String())
	}
	if elapsed < time.Second {
		t.Fatalf("插件 %.2fs 就退出，未等满握手预算（可能在读前就失败）", elapsed.Seconds())
	}
	if !strings.Contains(stderr.String(), "kore.hello") {
		t.Fatalf("stderr 未体现 hello 失败: %s", stderr.String())
	}
}

// TestHandshakeRejected：主壳回 ok=false 时插件必须退出 1，不得继续会话循环。
func TestHandshakeRejected(t *testing.T) {
	kernel := newFakeKernel(t)
	cmd, stderr := startPlugin(t, kernel.socket)
	conn := kernel.accept(t)
	hello := readRequest(t, conn)
	replyHello(t, conn, hello, false, pluginapi.Protocol)
	if code := waitExit(t, cmd, 5*time.Second); code != 1 {
		t.Fatalf("退出码 = %d，期望 1；stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "ok=false") {
		t.Fatalf("stderr 未体现握手被拒: %s", stderr.String())
	}
}

// TestPeerDisconnectBeforeFrame：握手后主壳直接断开 → 插件按优雅退出 0（不是崩溃 1）。
func TestPeerDisconnectBeforeFrame(t *testing.T) {
	kernel := newFakeKernel(t)
	conn, cmd, stderr := startAndHandshake(t, kernel)
	_ = conn.Close()
	if code := waitExit(t, cmd, 5*time.Second); code != 0 {
		t.Fatalf("对端断开后退出码 = %d，期望 0；stderr=%s", code, stderr.String())
	}
}

// TestInvokeInvalidParams：echo 只接受 JSON 对象，数组参数回 E_INVALID 且不静默成功。
func TestInvokeInvalidParams(t *testing.T) {
	kernel := newFakeKernel(t)
	conn, cmd, stderr := startAndHandshake(t, kernel)
	response := sendRequest(t, conn, "echo.echo", json.RawMessage(`[1,2,3]`))
	if response.Error == nil || response.Error.Code != "E_INVALID" {
		t.Fatalf("非法参数错误 = %+v，期望 E_INVALID", response.Error)
	}
	invoke(t, conn, "kore.shutdown", nil) // 收尾让进程干净退出
	if code := waitExit(t, cmd, 5*time.Second); code != 0 {
		t.Fatalf("shutdown 后退出码 = %d，期望 0；stderr=%s", code, stderr.String())
	}
}

// TestInvokeUnknownMethod：未声明方法回 E_NOTFOUND，不能落到默认成功分支。
func TestInvokeUnknownMethod(t *testing.T) {
	kernel := newFakeKernel(t)
	conn, cmd, stderr := startAndHandshake(t, kernel)
	response := sendRequest(t, conn, "echo.nonexistent", nil)
	if response.Error == nil || response.Error.Code != "E_NOTFOUND" {
		t.Fatalf("未知方法错误 = %+v，期望 E_NOTFOUND", response.Error)
	}
	invoke(t, conn, "kore.shutdown", nil)
	if code := waitExit(t, cmd, 5*time.Second); code != 0 {
		t.Fatalf("shutdown 后退出码 = %d，期望 0；stderr=%s", code, stderr.String())
	}
}

// TestEchoNullParams：params 缺省/null 按协议补 {}，而不是报错。
func TestEchoNullParams(t *testing.T) {
	kernel := newFakeKernel(t)
	conn, cmd, _ := startAndHandshake(t, kernel)
	response := invoke(t, conn, "echo.echo", nil)
	if string(response.Result) != "{}" {
		t.Fatalf("空 params 结果 = %s，期望 {}", response.Result)
	}
	invoke(t, conn, "kore.shutdown", nil)
	_ = waitExit(t, cmd, 5*time.Second)
}

// TestCrashExitCode：crash 方法按约定退出 7，供内核验证重启计数与退避。
func TestCrashExitCode(t *testing.T) {
	kernel := newFakeKernel(t)
	conn, cmd, stderr := startAndHandshake(t, kernel)
	request := &pluginapi.Request{Type: pluginapi.TypeRequest, ID: "req-crash", Method: "echo.crash"}
	if err := pluginapi.WriteJSON(conn, request); err != nil {
		t.Fatalf("写 crash 请求失败: %v", err)
	}
	if code := waitExit(t, cmd, 5*time.Second); code != 7 {
		t.Fatalf("crash 退出码 = %d，期望 7；stderr=%s", code, stderr.String())
	}
}

// TestShutdownCleanExit：kore.shutdown 应答成功后退出 0（lazy 插件归还槽位的正常路径）。
func TestShutdownCleanExit(t *testing.T) {
	kernel := newFakeKernel(t)
	conn, cmd, stderr := startAndHandshake(t, kernel)
	invoke(t, conn, "kore.shutdown", nil)
	if code := waitExit(t, cmd, 5*time.Second); code != 0 {
		t.Fatalf("shutdown 后退出码 = %d，期望 0；stderr=%s", code, stderr.String())
	}
}

// ── issue #1 指定的 5 类故障注入（进程级，走 UDS + 真帧协议）──

// TestFaultUDSReconnect：UDS 断连重连——握手后主壳断开，插件优雅退出；
// 主壳重建 socket 后新进程重新握手（协议要求重连必须重新握手）并正常服务。
func TestFaultUDSReconnect(t *testing.T) {
	kernel := newFakeKernel(t)
	conn, cmd, stderr := startAndHandshake(t, kernel)
	_ = conn.Close()
	if code := waitExit(t, cmd, 5*time.Second); code != 0 {
		t.Fatalf("断开后退出码 = %d，期望 0；stderr=%s", code, stderr.String())
	}
	reconnected := newFakeKernel(t)
	conn2, cmd2, stderr2 := startAndHandshake(t, reconnected)
	response := invoke(t, conn2, "echo.ping", nil)
	if string(response.Result) != `{"pong":true}` {
		t.Fatalf("重连后 echo.ping 结果 = %s，期望 pong", response.Result)
	}
	invoke(t, conn2, "kore.shutdown", nil)
	if code := waitExit(t, cmd2, 5*time.Second); code != 0 {
		t.Fatalf("重连后 shutdown 退出码 = %d，期望 0；stderr=%s", code, stderr2.String())
	}
}

// TestFaultFrameOversize：帧超限——长度前缀超过 MaxFrameSize+1，插件在长度前缀处
// 即拒绝，不读载荷、不分配大缓冲、不 hang；连接关闭后进程退出 1。
func TestFaultFrameOversize(t *testing.T) {
	kernel := newFakeKernel(t)
	cmd, stderr := startPlugin(t, kernel.socket)
	conn := kernel.accept(t)
	hello := readRequest(t, conn)
	replyHello(t, conn, hello, true, pluginapi.Protocol)
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(pluginapi.MaxFrameSize)+1)
	if _, err := conn.Write(header[:]); err != nil {
		t.Fatalf("写超限长度前缀失败: %v", err)
	}
	if code := waitExit(t, cmd, 5*time.Second); code != 1 {
		t.Fatalf("超限帧后退出码 = %d，期望 1；stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "读帧失败") {
		t.Fatalf("stderr 未提示读帧失败: %s", stderr.String())
	}
}

// TestFaultHandshakeTokenWrong：握手 token 错——错 token 必须原样出现在 hello 参数；
// M1 协议 §17：Linux 上 token 仅审计关联、不做 HMAC 校验，握手仍成功；
// 但 token 绝不能进 stderr 日志（凭据不落日志）。
func TestFaultHandshakeTokenWrong(t *testing.T) {
	const correctToken = "token-expected-by-kernel"
	const wrongToken = "token-wrong-injected"
	kernel := newFakeKernel(t)
	cmd, stderr := startPluginIn(t, kernel.socket, "KORE_PLUGIN_TOKEN="+wrongToken)
	conn := kernel.accept(t)
	hello := readRequest(t, conn)
	if hello.Method != "kore.hello" {
		t.Fatalf("首帧方法 = %q，期望 kore.hello", hello.Method)
	}
	var params struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(hello.Params, &params); err != nil {
		t.Fatalf("解析 hello 参数失败: %v", err)
	}
	if params.Token != wrongToken {
		t.Fatalf("hello token = %q，期望原样上报 %q", params.Token, wrongToken)
	}
	if params.Token == correctToken {
		t.Fatal("错 token 被改写：注入失效")
	}
	replyHello(t, conn, hello, true, pluginapi.Protocol)
	response := invoke(t, conn, "echo.ping", nil)
	if string(response.Result) != `{"pong":true}` {
		t.Fatalf("错 token 下 echo.ping 结果 = %s", response.Result)
	}
	if strings.Contains(stderr.String(), wrongToken) || strings.Contains(stderr.String(), correctToken) {
		t.Fatalf("stderr 泄露 token: %s", stderr.String())
	}
	invoke(t, conn, "kore.shutdown", nil)
	if code := waitExit(t, cmd, 5*time.Second); code != 0 {
		t.Fatalf("shutdown 后退出码 = %d，期望 0；stderr=%s", code, stderr.String())
	}
}

// TestFaultMethodPanicProcessSurvives：方法 panic 后进程存活——用 -tags faultinject
// 构建的探针二进制让 echo.__panic_probe 真 panic，断言：① 会话收到 E_INTERNAL
// 而不是连接断开；② 插件进程没有退出；③ 后续调用与 shutdown 正常完成。
func TestFaultMethodPanicProcessSurvives(t *testing.T) {
	kernel := newFakeKernel(t)
	cmd, stderr := startPluginProbe(t, kernel.socket)
	conn := kernel.accept(t)
	hello := readRequest(t, conn)
	if hello.Method != "kore.hello" {
		t.Fatalf("首帧方法 = %q，期望 kore.hello", hello.Method)
	}
	replyHello(t, conn, hello, true, pluginapi.Protocol)

	response := sendRequest(t, conn, "echo.__panic_probe", nil)
	if response.Error == nil || response.Error.Code != pluginapi.E_INTERNAL {
		t.Fatalf("panic 响应 = %+v，期望 E_INTERNAL", response.Error)
	}

	status := queryPing(t, conn, "after-panic")
	if status.Status != "ready" {
		t.Fatalf("panic 后 status = %q，期望 ready", status.Status)
	}
	biz := invoke(t, conn, "echo.ping", nil)
	if string(biz.Result) != `{"pong":true}` {
		t.Fatalf("panic 后 echo.ping 结果 = %s", biz.Result)
	}
	invoke(t, conn, "kore.shutdown", nil)
	if code := waitExit(t, cmd, 5*time.Second); code != 0 {
		t.Fatalf("panic 后 shutdown 退出码 = %d，期望 0（进程存活）；stderr=%s", code, stderr.String())
	}
}

// TestFaultSlowCallTimeout：慢调用超时——echo.slow 固定 1s，调用方预算 250ms；
// 预算内放弃、迟到响应 id/req_id 不错乱、插件进程继续存活。
func TestFaultSlowCallTimeout(t *testing.T) {
	const budget = 250 * time.Millisecond
	kernel := newFakeKernel(t)
	conn, cmd, stderr := startAndHandshake(t, kernel)
	out := callWithBudget(conn, "req-fault-slow", "r-fault-slow", "echo.slow", nil,
		pluginapi.Timeout(int(budget/time.Millisecond)), budget)
	assertCallerAbandoned(t, out, budget, time.Second)
	late := readLateResponse(t, conn, "req-fault-slow", "r-fault-slow", 3*time.Second)
	if string(late.Result) != `{"slept_ms":1000}` {
		t.Fatalf("迟到 slow 结果 = %s", late.Result)
	}
	alive := queryPing(t, conn, "slow-timeout")
	if alive.Status != "ready" {
		t.Fatalf("超时后 status = %q，期望 ready", alive.Status)
	}
	invoke(t, conn, "kore.shutdown", nil)
	if code := waitExit(t, cmd, 5*time.Second); code != 0 {
		t.Fatalf("shutdown 后退出码 = %d，期望 0；stderr=%s", code, stderr.String())
	}
}

// startPluginProbe 启动带 panic 注入钩子的二进制（仅测试构建），其余同 startPlugin。
func startPluginProbe(t *testing.T, socket string) (*exec.Cmd, *safeBuffer) {
	cmd := exec.Command(probeBinaryPath)
	cmd.Env = append(os.Environ(), "GOWORK=off", "KORE_PLUGIN_SOCK="+socket, "KORE_PLUGIN_ID=echo", "KORE_LOG_LEVEL=debug")
	stderr := &safeBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动 faultinject 探针失败: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	return cmd, stderr
}

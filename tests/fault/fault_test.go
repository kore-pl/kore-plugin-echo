package fault

import (
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

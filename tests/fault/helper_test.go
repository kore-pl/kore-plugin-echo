// Package fault 对 echo 验证插件做进程级故障注入：用假主壳（UDS + 真帧协议）起真二进制，
// 覆盖握手超时 / 拒握手 / 对端断开 / 非法参数 / 未知方法 / crash 退出码。
//
// 这些用例保护的是"主壳侧异常时插件不能 hang、不能谎报成功"——
// 与内核侧 supervisor 的重启 / 重连策略配套，是 depth_level=production 的故障注入证据。
package fault

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// binaryPath 是 TestMain 预构建的 echo 二进制（一次构建，所有用例复用）。
var binaryPath string

// TestMain 在仓库根构建插件二进制，避免每个用例 go run 的秒级开销。
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "echo-fault-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建临时目录失败: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)
	binaryPath = filepath.Join(dir, "echo")
	build := exec.Command("go", "build", "-o", binaryPath, ".")
	build.Dir = filepath.Join("..", "..")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "构建 echo 失败: %v\n%s", err, out)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// fakeKernel 是主壳侧的最小替身：监听 UDS、收 hello、按用例注入异常。
type fakeKernel struct {
	listener net.Listener
	socket   string
}

// newFakeKernel 在临时目录建 UDS 并开始监听。
func newFakeKernel(t *testing.T) *fakeKernel {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "kernel.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("监听 UDS 失败: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return &fakeKernel{listener: listener, socket: socket}
}

// accept 接受插件进程的连接并设读超时，避免用例挂死。
func (k *fakeKernel) accept(t *testing.T) net.Conn {
	t.Helper()
	_ = k.listener.(*net.UnixListener).SetDeadline(time.Now().Add(5 * time.Second))
	conn, err := k.listener.Accept()
	if err != nil {
		t.Fatalf("接受插件连接失败: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	return conn
}

// readRequest 读一帧并断言是 request；返回请求以便回填 id/req_id。
func readRequest(t *testing.T, conn net.Conn) *pluginapi.Request {
	t.Helper()
	payload, err := pluginapi.ReadFrame(conn, pluginapi.MaxFrameSize)
	if err != nil {
		t.Fatalf("读帧失败: %v", err)
	}
	env, err := pluginapi.DecodeFrame(payload)
	if err != nil {
		t.Fatalf("解码帧失败: %v", err)
	}
	request, ok := env.(*pluginapi.Request)
	if !ok {
		t.Fatalf("期望 request 帧，实得 %T", env)
	}
	return request
}

// replyHello 按实际请求 id 回握手响应，ok/protocol 由用例指定。
func replyHello(t *testing.T, conn net.Conn, request *pluginapi.Request, ok bool, protocol string) {
	t.Helper()
	result, err := json.Marshal(struct {
		OK       bool   `json:"ok"`
		Protocol string `json:"protocol"`
	}{OK: ok, Protocol: protocol})
	if err != nil {
		t.Fatalf("序列化 hello 结果失败: %v", err)
	}
	response := &pluginapi.Response{Type: pluginapi.TypeResponse, ID: request.ID, ReqID: request.ReqID, Result: result}
	if err := pluginapi.WriteJSON(conn, response); err != nil {
		t.Fatalf("写 hello 响应失败: %v", err)
	}
}

// handshakeOK 完成一次成功握手（协议取 SDK 常量）。
func handshakeOK(t *testing.T, kernel *fakeKernel) net.Conn {
	t.Helper()
	conn := kernel.accept(t)
	hello := readRequest(t, conn)
	if hello.Method != "kore.hello" {
		t.Fatalf("首帧方法 = %q，期望 kore.hello", hello.Method)
	}
	replyHello(t, conn, hello, true, pluginapi.Protocol)
	return conn
}

// startPlugin 以 KORE_PLUGIN_SOCK 指向假主壳启动插件，stderr 收集到缓冲区。
func startPlugin(t *testing.T, socket string) (*exec.Cmd, *safeBuffer) {
	t.Helper()
	return startPluginIn(t, socket)
}

// startPluginIn 是 startPlugin 的扩展版：追加 extraEnv（如 TMPDIR=...），
// 供「放弃后无临时文件残留」的用例把插件 TMPDIR 指到可观测的临时目录。
func startPluginIn(t *testing.T, socket string, extraEnv ...string) (*exec.Cmd, *safeBuffer) {
	t.Helper()
	cmd := exec.Command(binaryPath)
	cmd.Env = append(append(os.Environ(), "GOWORK=off", "KORE_PLUGIN_SOCK="+socket, "KORE_PLUGIN_ID=echo", "KORE_LOG_LEVEL=debug"), extraEnv...)
	stderr := &safeBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动插件失败: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	return cmd, stderr
}

// waitExit 等待进程退出并返回退出码；超时即失败。
func waitExit(t *testing.T, cmd *exec.Cmd, timeout time.Duration) int {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			return 0
		}
		var exitErr *exec.ExitError
		if ok := asExitError(err, &exitErr); ok {
			return exitErr.ExitCode()
		}
		t.Fatalf("等待进程退出失败: %v", err)
	case <-time.After(timeout):
		t.Fatalf("插件 %s 内未退出（疑似 hang）", timeout)
	}
	return -1
}

// asExitError 是 errors.As 的薄封装，避免 helper 顶端再引入 errors 只为一处。
func asExitError(err error, target **exec.ExitError) bool {
	exitErr, ok := err.(*exec.ExitError)
	if ok {
		*target = exitErr
	}
	return ok
}

// safeBuffer 是并发安全的 stderr 缓冲（插件写、测试读）。
type safeBuffer struct {
	data []byte
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *safeBuffer) String() string { return string(b.data) }

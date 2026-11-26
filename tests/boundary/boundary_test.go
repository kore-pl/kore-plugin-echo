// Package boundary 是 echo 升 production 的边界用例（PLUGIN-CATALOG §6.1 判据①）：
// 异常输入 / 并发 / 权限不足 / 资源耗尽各 ≥1 例。全部进程级，走 UDS + 真帧协议。
package boundary

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

var probeBinary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "echo-boundary-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建临时目录失败: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)
	probeBinary = filepath.Join(dir, "echo")
	build := exec.Command("go", "build", "-o", probeBinary, ".")
	build.Dir = filepath.Join("..", "..")
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "构建 echo 失败: %v\n%s", err, out)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

type fakeKernel struct {
	listener net.Listener
	socket   string
}

func newKernel(t *testing.T) *fakeKernel {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "kernel.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("监听 UDS 失败: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return &fakeKernel{listener: listener, socket: socket}
}

func (k *fakeKernel) accept(t *testing.T) net.Conn {
	t.Helper()
	_ = k.listener.(*net.UnixListener).SetDeadline(time.Now().Add(5 * time.Second))
	conn, err := k.listener.Accept()
	if err != nil {
		t.Fatalf("accept 失败: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	return conn
}

func startEcho(t *testing.T, socket string, extraEnv ...string) (*exec.Cmd, *safeBuffer) {
	t.Helper()
	cmd := exec.Command(probeBinary)
	cmd.Env = append(append(os.Environ(), "GOWORK=off", "KORE_PLUGIN_SOCK="+socket, "KORE_PLUGIN_ID=echo"), extraEnv...)
	stderr := &safeBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动插件失败: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	return cmd, stderr
}

type safeBuffer struct{ data []byte }

func (b *safeBuffer) Write(p []byte) (int, error) { b.data = append(b.data, p...); return len(p), nil }
func (b *safeBuffer) String() string              { return string(b.data) }

func readFrame(t *testing.T, conn net.Conn) any {
	t.Helper()
	payload, err := pluginapi.ReadFrame(conn, pluginapi.MaxFrameSize)
	if err != nil {
		t.Fatalf("读帧失败: %v", err)
	}
	env, err := pluginapi.DecodeFrame(payload)
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}
	return env
}

func readRequest(t *testing.T, conn net.Conn) *pluginapi.Request {
	t.Helper()
	env := readFrame(t, conn)
	req, ok := env.(*pluginapi.Request)
	if !ok {
		t.Fatalf("期望 request，实得 %T", env)
	}
	return req
}

func writeJSON(t *testing.T, conn net.Conn, v any) {
	t.Helper()
	if err := pluginapi.WriteJSON(conn, v); err != nil {
		t.Fatalf("写帧失败: %v", err)
	}
}

func handshake(t *testing.T, conn net.Conn) {
	t.Helper()
	hello := readRequest(t, conn)
	if hello.Method != "kore.hello" {
		t.Fatalf("首帧方法 = %q", hello.Method)
	}
	result, _ := json.Marshal(map[string]any{"ok": true, "protocol": pluginapi.Protocol})
	writeJSON(t, conn, &pluginapi.Response{Type: pluginapi.TypeResponse, ID: hello.ID, ReqID: hello.ReqID, Result: result})
}

func call(t *testing.T, conn net.Conn, method string, params json.RawMessage) *pluginapi.Response {
	t.Helper()
	req := &pluginapi.Request{Type: pluginapi.TypeRequest, ID: "b-" + method, ReqID: "r-" + method, Method: method, Params: params}
	writeJSON(t, conn, req)
	env := readFrame(t, conn)
	resp, ok := env.(*pluginapi.Response)
	if !ok {
		t.Fatalf("期望 response，实得 %T", env)
	}
	return resp
}

// 异常输入：畸形参数不得静默成功，也不得让进程退出。
func TestBoundaryInvalidInput(t *testing.T) {
	kernel := newKernel(t)
	cmd, _ := startEcho(t, kernel.socket)
	conn := kernel.accept(t)
	handshake(t, conn)
	for _, params := range []json.RawMessage{json.RawMessage(`[1,2,3]`), json.RawMessage(`"not-object"`)} {
		resp := call(t, conn, "echo.echo", params)
		if resp.Error == nil || resp.Error.Code != pluginapi.E_INVALID {
			t.Fatalf("非法参数 %s 的响应 = %+v，期望 E_INVALID", params, resp.Error)
		}
	}
	ping := call(t, conn, "echo.ping", nil)
	if string(ping.Result) != `{"pong":true}` {
		t.Fatalf("异常输入后插件不存活: %s", ping.Result)
	}
	call(t, conn, "kore.shutdown", nil)
	if err := cmd.Wait(); err != nil {
		t.Fatalf("shutdown 后退出失败: %v", err)
	}
}

// 并发：8 个插件实例同时握手并调用 ping，互不串扰（t.Parallel 子测试，可进 -race）。
func TestBoundaryConcurrentInstances(t *testing.T) {
	for i := 0; i < 8; i++ {
		i := i
		t.Run(fmt.Sprintf("instance-%d", i), func(t *testing.T) {
			t.Parallel()
			kernel := newKernel(t)
			cmd, _ := startEcho(t, kernel.socket)
			conn := kernel.accept(t)
			handshake(t, conn)
			resp := call(t, conn, "echo.ping", nil)
			if string(resp.Result) != `{"pong":true}` {
				t.Fatalf("并发实例 %d 结果 = %s", i, resp.Result)
			}
			call(t, conn, "kore.shutdown", nil)
			if err := cmd.Wait(); err != nil {
				t.Fatalf("并发实例 %d 退出失败: %v", i, err)
			}
		})
	}
}

// 权限不足：错 token 不得越权；未声明方法回 E_NOTFOUND，且 token 不落日志。
func TestBoundaryPermissionDenied(t *testing.T) {
	kernel := newKernel(t)
	cmd, stderr := startEcho(t, kernel.socket, "KORE_PLUGIN_TOKEN=wrong-token")
	conn := kernel.accept(t)
	hello := readRequest(t, conn)
	var params struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(hello.Params, &params); err != nil {
		t.Fatalf("解析 hello 参数失败: %v", err)
	}
	if params.Token != "wrong-token" {
		t.Fatalf("hello token = %q，期望 wrong-token", params.Token)
	}
	result, _ := json.Marshal(map[string]any{"ok": true, "protocol": pluginapi.Protocol})
	writeJSON(t, conn, &pluginapi.Response{Type: pluginapi.TypeResponse, ID: hello.ID, ReqID: hello.ReqID, Result: result})
	resp := call(t, conn, "system.admin.reboot", nil)
	if resp.Error == nil || resp.Error.Code != pluginapi.E_NOTFOUND {
		t.Fatalf("未声明方法响应 = %+v，期望 E_NOTFOUND", resp.Error)
	}
	if strings.Contains(stderr.String(), "wrong-token") {
		t.Fatalf("stderr 泄露 token: %s", stderr.String())
	}
	call(t, conn, "kore.shutdown", nil)
	if err := cmd.Wait(); err != nil {
		t.Fatalf("shutdown 失败: %v", err)
	}
}

// 资源耗尽：单帧超过 MaxFrameSize 上限时插件不得分配大缓冲或 hang；连接立即关闭。
func TestBoundaryResourceExhaustion(t *testing.T) {
	kernel := newKernel(t)
	cmd, _ := startEcho(t, kernel.socket)
	conn := kernel.accept(t)
	handshake(t, conn)
	var hdr [4]byte
	hdr[0], hdr[1], hdr[2], hdr[3] = 0x80, 0x01, 0x00, 0x01
	if _, err := conn.Write(hdr[:]); err != nil {
		t.Fatalf("写超限帧头失败: %v", err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("超限帧后插件退出码 = 0，期望 1（协议失败）")
	}
	if err := conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
		t.Fatalf("设读超时失败: %v", err)
	}
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("超限帧后连接仍可读：未按协议关闭")
	}
}

// 并发写：同一会话顺序回帧，req_id 一一对应，无串包（供 -race 覆盖）。
func TestBoundaryConcurrentCallsSameSession(t *testing.T) {
	kernel := newKernel(t)
	cmd, _ := startEcho(t, kernel.socket)
	conn := kernel.accept(t)
	handshake(t, conn)
	var wg sync.WaitGroup
	errCh := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("r-conc-%d", i)
			params := json.RawMessage(fmt.Sprintf(`{"n":%d}`, i))
			req := &pluginapi.Request{Type: pluginapi.TypeRequest, ID: "id-" + id, ReqID: id, Method: "echo.echo", Params: params}
			if err := pluginapi.WriteJSON(conn, req); err != nil {
				errCh <- err.Error()
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for msg := range errCh {
		t.Fatalf("并发写失败: %s", msg)
	}
	seen := map[string]bool{}
	for i := 0; i < 8; i++ {
		env := readFrame(t, conn)
		resp, ok := env.(*pluginapi.Response)
		if !ok {
			t.Fatalf("实得帧类型 %T", env)
		}
		if seen[resp.ReqID] {
			t.Fatalf("重复 req_id: %s", resp.ReqID)
		}
		seen[resp.ReqID] = true
	}
	call(t, conn, "kore.shutdown", nil)
	if err := cmd.Wait(); err != nil {
		t.Fatalf("shutdown 失败: %v", err)
	}
}

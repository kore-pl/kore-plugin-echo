package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"github.com/kore-project/kore/pkg/pluginapi"
)

// testConfig 构造握手与会话循环共用的一致配置（不依赖环境变量）。
func testConfig() config {
	return config{
		id:           "echo",
		version:      "0.1.0",
		requiresKore: ">=0.1.0 <0.2.0",
		capabilities: []string{"system:read"},
		startedAt:    time.Now(),
	}
}

// testLogger 返回丢弃输出的日志器，避免单测噪声。
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// startServe 在插件侧跑会话循环，返回结束通道。
func startServe(conn net.Conn) <-chan error {
	done := make(chan error, 1)
	go func() {
		err := serve(conn, testConfig(), testLogger())
		_ = conn.Close()
		done <- err
	}()
	return done
}

// readFrame 从主壳侧读一帧并返回解码后的信封。
func readFrame(t *testing.T, conn net.Conn) any {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	payload, err := pluginapi.ReadFrame(conn, pluginapi.MaxFrameSize)
	if err != nil {
		t.Fatalf("读帧失败: %v", err)
	}
	env, err := pluginapi.DecodeFrame(payload)
	if err != nil {
		t.Fatalf("解码帧失败: %v", err)
	}
	return env
}

// readRequest 读一帧并断言是 request。
func readRequest(t *testing.T, conn net.Conn) *pluginapi.Request {
	t.Helper()
	env := readFrame(t, conn)
	request, ok := env.(*pluginapi.Request)
	if !ok {
		t.Fatalf("帧类型是 %T，不是 request", env)
	}
	return request
}

// readResponse 读一帧并断言是 response。
func readResponse(t *testing.T, conn net.Conn) *pluginapi.Response {
	t.Helper()
	env := readFrame(t, conn)
	response, ok := env.(*pluginapi.Response)
	if !ok {
		t.Fatalf("帧类型是 %T，不是 response", env)
	}
	return response
}

// writeRequest 在主壳侧写一帧请求。
func writeRequest(t *testing.T, conn net.Conn, method, reqID string, params json.RawMessage) {
	t.Helper()
	request := &pluginapi.Request{
		Type: pluginapi.TypeRequest, ID: "id-" + reqID, ReqID: reqID, Method: method, Params: params,
	}
	if err := pluginapi.WriteJSON(conn, request); err != nil {
		t.Fatalf("写请求失败: %v", err)
	}
}

// TestHandshake 用 net.Pipe 模拟主壳侧，校验 hello 首帧字段与三步握手。
func TestHandshake(t *testing.T) {
	pluginConn, kernelConn := net.Pipe()
	defer func() { _ = kernelConn.Close() }()
	done := make(chan error, 1)
	go func() { done <- handshake(pluginConn, testConfig()); _ = pluginConn.Close() }()

	request := readRequest(t, kernelConn)
	if request.Method != helloMethod {
		t.Fatalf("首帧方法=%q，期望 %q", request.Method, helloMethod)
	}
	var params helloParams
	if err := json.Unmarshal(request.Params, &params); err != nil {
		t.Fatalf("解析 hello 参数失败: %v", err)
	}
	if params.PluginID != "echo" || params.ID != "echo" {
		t.Fatalf("plugin_id/id=%q/%q，期望都是 echo", params.PluginID, params.ID)
	}
	if params.Protocol != pluginapi.Protocol || params.Version != "0.1.0" {
		t.Fatalf("protocol/version=%q/%q 不符合 §5", params.Protocol, params.Version)
	}
	if params.PID != os.Getpid() {
		t.Fatalf("pid=%d，期望 %d", params.PID, os.Getpid())
	}
	result, err := json.Marshal(helloResult{OK: true, Protocol: pluginapi.Protocol})
	if err != nil {
		t.Fatalf("序列化握手响应失败: %v", err)
	}
	writeResponse(t, kernelConn, request.ID, request.ReqID, result)
	if err := <-done; err != nil {
		t.Fatalf("握手返回错误: %v", err)
	}
}

// writeResponse 在主壳侧写一帧响应（id/req_id 原样回显）。
func writeResponse(t *testing.T, conn net.Conn, id, reqID string, result json.RawMessage) {
	t.Helper()
	response := &pluginapi.Response{Type: pluginapi.TypeResponse, ID: id, ReqID: reqID, Result: result}
	if err := pluginapi.WriteJSON(conn, response); err != nil {
		t.Fatalf("写响应失败: %v", err)
	}
}

// TestServeMethods 覆盖 ping / echo（含全名）/ 未知方法与 req_id 回显。
func TestServeMethods(t *testing.T) {
	cases := []struct {
		name       string
		method     string
		params     json.RawMessage
		wantResult string
		wantCode   string
	}{
		{name: "ping", method: "ping", wantResult: `{"pong":true}`},
		{name: "echo", method: "echo", params: json.RawMessage(`{"k":"v"}`), wantResult: `{"k":"v"}`},
		{name: "echo 全名", method: "echo.echo", params: json.RawMessage(`{"n":1}`), wantResult: `{"n":1}`},
		{name: "未知方法", method: "echo.nope", wantCode: pluginapi.E_NOTFOUND},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pluginConn, kernelConn := net.Pipe()
			defer func() { _ = kernelConn.Close() }()
			done := startServe(pluginConn)
			writeRequest(t, kernelConn, tc.method, "r-1", tc.params)
			response := readResponse(t, kernelConn)
			if response.ReqID != "r-1" {
				t.Fatalf("req_id=%q，期望回显 r-1", response.ReqID)
			}
			if tc.wantCode != "" {
				if response.Error == nil || response.Error.Code != tc.wantCode {
					t.Fatalf("error=%v，期望 code %s", response.Error, tc.wantCode)
				}
			} else if string(response.Result) != tc.wantResult {
				t.Fatalf("result=%s，期望 %s", response.Result, tc.wantResult)
			}
			writeRequest(t, kernelConn, "kore.shutdown", "r-2", nil)
			if shutdown := readResponse(t, kernelConn); string(shutdown.Result) != `{"ok":true}` {
				t.Fatalf("shutdown 结果=%s", shutdown.Result)
			}
			if err := <-done; err != nil {
				t.Fatalf("serve 返回错误: %v", err)
			}
		})
	}
}

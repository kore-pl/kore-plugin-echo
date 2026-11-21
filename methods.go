package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

const (
	// exitCrash 是 crash 方法的约定退出码，用于验证主壳重启计数。
	exitCrash = 7
	// slowDelay 与 timeoutDelay 是 slow / timeout 的固定睡眠时长。
	slowDelay    = time.Second
	timeoutDelay = 3 * time.Second
)

// statusResult 是 kore.ping 的结果（PLUGIN-PROTOCOL §7.1）。
type statusResult struct {
	Status     string `json:"status"`
	UptimeS    int64  `json:"uptime_s"`
	Goroutines int    `json:"goroutines"`
	RSSKB      int64  `json:"rss_kb"`
}

// dispatch 把请求分发为响应帧；第二个返回值表示收到 kore.shutdown。
// crash 方法直接 os.Exit(7)，不返回。
func dispatch(request *pluginapi.Request, cfg config, logger *slog.Logger) (*pluginapi.Response, bool) {
	response := &pluginapi.Response{Type: pluginapi.TypeResponse, ID: request.ID, ReqID: request.ReqID}
	switch request.Method {
	case "kore.shutdown":
		response.Result = json.RawMessage(`{"ok":true}`)
		logger.Info("收到 kore.shutdown，优雅退出")
		return response, true
	case "kore.ping":
		response.Result = pingResult(cfg)
		return response, false
	case "kore.reload":
		response.Result = json.RawMessage(`{"ok":true,"config_digest":""}`)
		return response, false
	}
	verb, ok := businessVerb(request.Method, cfg.id)
	if !ok {
		response.Error = notFound(request.Method)
		return response, false
	}
	if err := handleVerb(verb, request, response, logger); err != nil {
		response.Error = err
		response.Result = nil
	}
	return response, false
}

// handleVerb 执行业务动词，返回协议错误体；crash 以退出码 7 结束进程。
func handleVerb(verb string, request *pluginapi.Request, response *pluginapi.Response, logger *slog.Logger) *pluginapi.Error {
	switch verb {
	case "ping":
		response.Result = json.RawMessage(`{"pong":true}`)
	case "echo":
		params, err := echoParams(request.Params)
		if err != nil {
			return err
		}
		response.Result = params
	case "slow":
		time.Sleep(slowDelay)
		response.Result = json.RawMessage(`{"slept_ms":1000}`)
	case "timeout":
		time.Sleep(timeoutDelay)
		response.Result = json.RawMessage(`{"slept_ms":3000}`)
	case "crash":
		logger.Warn("crash 方法被调用，按验证约定退出", "exit_code", exitCrash)
		os.Exit(exitCrash)
	default:
		return notFound(request.Method)
	}
	return nil
}

// businessVerb 把 <id>.<verb> 或裸 <verb> 归一为动词名（M1 两种写法都接受）。
func businessVerb(method, id string) (string, bool) {
	verb := strings.TrimPrefix(method, id+".")
	switch verb {
	case "ping", "echo", "slow", "timeout", "crash":
		return verb, true
	default:
		return "", false
	}
}

// echoParams 校验 params 是 JSON 对象后原样返回；缺省按协议补 {}。
func echoParams(raw json.RawMessage) (json.RawMessage, *pluginapi.Error) {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage(`{}`), nil
	}
	if raw[0] != '{' {
		return nil, &pluginapi.Error{Code: pluginapi.E_INVALID, Message: "params 必须是 JSON 对象"}
	}
	return raw, nil
}

// pingResult 组装 kore.ping 结果；序列化失败只可能来自结构体定义错误。
func pingResult(cfg config) json.RawMessage {
	result, err := json.Marshal(statusResult{
		Status:     "ready",
		UptimeS:    int64(time.Since(cfg.startedAt).Seconds()),
		Goroutines: runtime.NumGoroutine(),
		RSSKB:      rssKB(),
	})
	if err != nil {
		return json.RawMessage(`{"status":"ready"}`)
	}
	return result
}

// notFound 构造 E_NOTFOUND 错误体（§9）。
func notFound(method string) *pluginapi.Error {
	return &pluginapi.Error{Code: pluginapi.E_NOTFOUND, Message: "未知方法：" + method}
}

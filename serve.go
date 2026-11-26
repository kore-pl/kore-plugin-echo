package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// serve 执行会话循环：读帧 → 分发 → 写 response；收到 kore.shutdown 后返回 nil。
func serve(conn net.Conn, cfg config, logger *slog.Logger) error {
	for {
		payload, err := pluginapi.ReadFrame(conn, pluginapi.MaxFrameSize)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil // 帧边界上的干净关闭
			}
			return fmt.Errorf("读帧失败: %w", err)
		}
		request, err := decodeRequest(payload)
		if err != nil {
			return err
		}
		if request == nil {
			logger.Debug("忽略非 request 帧")
			continue
		}
		// 方法级 panic 不得带走进程：隔离为 E_INTERNAL 响应，会话循环继续服务。
		// crash 约定的 os.Exit(7) 不经 recover，仍按约定退出。
		response, shutdown := dispatchSafely(request, cfg, logger)
		if err := pluginapi.WriteJSON(conn, response); err != nil {
			if shutdown {
				return nil // 内核已关闭连接，退出码仍按优雅退出
			}
			return fmt.Errorf("写响应失败: %w", err)
		}
		if shutdown {
			return nil
		}
	}
}

// decodeRequest 解码一帧并返回 request；非 request 帧返回 (nil, nil) 继续循环。
func decodeRequest(payload []byte) (*pluginapi.Request, error) {
	env, err := pluginapi.DecodeFrame(payload)
	if err != nil {
		return nil, fmt.Errorf("解码帧失败: %w", err)
	}
	request, ok := env.(*pluginapi.Request)
	if !ok {
		return nil, nil
	}
	return request, nil
}

// panicValue 把 recover 值安全转成字符串，避免任意值实现 Stringer 时二次 panic。
type panicValue struct{ value any }

func (p panicValue) String() string { return fmt.Sprint(p.value) }

// dispatchSafely 把方法级 panic 转成 E_INTERNAL 响应帧，保住进程与会话循环。
// 日志只记方法与请求 id，不写 token（凭据不得进日志）。
func dispatchSafely(request *pluginapi.Request, cfg config, logger *slog.Logger) (response *pluginapi.Response, shutdown bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Error("方法 panic 已隔离", "method", request.Method, "id", request.ID, "panic", panicValue{value: recovered})
			response = &pluginapi.Response{
				Type:  pluginapi.TypeResponse,
				ID:    request.ID,
				ReqID: request.ReqID,
				Error: &pluginapi.Error{Code: pluginapi.E_INTERNAL, Message: "方法执行异常，已隔离"},
			}
			shutdown = false
		}
	}()
	return dispatch(request, cfg, logger)
}

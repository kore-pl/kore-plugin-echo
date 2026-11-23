// 调用方预算（context deadline）耗尽时的放弃用例：slow / timeout 是内核超时策略的探针，
// 本文件断言「调用方在预算内放弃、放弃后插件侧无残留、放弃错误是超时语义」。
//
// 调用方错误码口径：本仓只含插件侧代码，真实映射在内核 bus——net 超时 → E_UPSTREAM
// 「插件 <id> 调用超时」（kore/internal/core/bus/call.go:119-122）。本仓没有 E_TIMEOUT，
// 故断言以内核真实语义 E_UPSTREAM 为准，并用 net.Error.Timeout() 校验底层原因。
package fault

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/pengtang1024/kore/pkg/pluginapi"
)

// callerOutcome 是一次「带预算调用」的结果：要么拿到响应，要么在预算内放弃。
type callerOutcome struct {
	// response 非 nil 表示调用在预算内完成（本文件的用例都期望它为 nil）。
	response *pluginapi.Response
	// err 非 nil 表示调用方放弃；应为 net 超时。
	err error
	// elapsed 是从写请求到放弃/收响应的实际耗时。
	elapsed time.Duration
}

// callWithBudget 模拟内核 bus.Call 的调用方预算：设置连接 deadline → 写请求 → 读响应。
//
// timeoutMS 取内核实际会写入帧的值（bus.Call 用 pluginapi.Timeout 规范化：小于 1000ms 的
// ctx 剩余预算会被抬到 1000ms 下限），因此「帧上声明 1000ms、调用方实际只等 250ms」正是
// 内核 ctx 截止时间早于协议下限时的真实形态。
func callWithBudget(conn net.Conn, id, reqID, method string, params json.RawMessage, timeoutMS int, budget time.Duration) callerOutcome {
	request := &pluginapi.Request{
		Type:      pluginapi.TypeRequest,
		ID:        id,
		ReqID:     reqID,
		Method:    method,
		Params:    params,
		TimeoutMS: timeoutMS,
	}
	started := time.Now()
	_ = conn.SetDeadline(time.Now().Add(budget))
	defer func() { _ = conn.SetDeadline(time.Time{}) }()
	if err := pluginapi.WriteJSON(conn, request); err != nil {
		return callerOutcome{err: err, elapsed: time.Since(started)}
	}
	payload, err := pluginapi.ReadFrame(conn, pluginapi.MaxFrameSize)
	elapsed := time.Since(started)
	if err != nil {
		return callerOutcome{err: err, elapsed: elapsed}
	}
	envelope, err := pluginapi.DecodeFrame(payload)
	if err != nil {
		return callerOutcome{err: err, elapsed: elapsed}
	}
	response, ok := envelope.(*pluginapi.Response)
	if !ok {
		return callerOutcome{err: fmt.Errorf("期望 response 帧，实得 %T", envelope), elapsed: elapsed}
	}
	return callerOutcome{response: response, elapsed: elapsed}
}

// callerErrorCode 镜像内核 bus.frameError 的超时归类：net 超时 → E_UPSTREAM。
// 本函数刻意只用于断言「放弃原因属于哪一类」，不构造插件侧行为。
func callerErrorCode(err error) string {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return pluginapi.E_UPSTREAM
	}
	return ""
}

// assertCallerAbandoned 断言调用方在预算内放弃：没收到响应、原因是超时、耗时对得上预算且
// 明显早于方法本身的完成时刻（不是等到方法返回才失败）。
func assertCallerAbandoned(t *testing.T, out callerOutcome, budget, methodDelay time.Duration) {
	t.Helper()
	if out.response != nil {
		t.Fatalf("调用方在预算内收到响应（%s），未放弃：result=%s", out.elapsed, out.response.Result)
	}
	if out.err == nil {
		t.Fatalf("调用方既没收到响应也没有错误（budget=%s）", budget)
	}
	var netErr net.Error
	if !errors.As(out.err, &netErr) || !netErr.Timeout() {
		t.Fatalf("放弃原因不是超时（net.Error.Timeout()=false）：%v", out.err)
	}
	if !errors.Is(out.err, os.ErrDeadlineExceeded) {
		t.Fatalf("放弃原因不是 deadline exceeded：%v", out.err)
	}
	if code := callerErrorCode(out.err); code != pluginapi.E_UPSTREAM {
		t.Fatalf("放弃的错误码=%q，期望 %s（内核 bus 对 net 超时的真实归类）", code, pluginapi.E_UPSTREAM)
	}
	if out.elapsed < budget*8/10 {
		t.Fatalf("调用方 %.2fs 就失败，未等满预算 %s（可能在读前就出错）", out.elapsed.Seconds(), budget)
	}
	if out.elapsed > 3*budget {
		t.Fatalf("调用方 %.2fs 才放弃，超出预算 %s 三倍（deadline 未生效）", out.elapsed.Seconds(), budget)
	}
	if out.elapsed >= methodDelay {
		t.Fatalf("调用方 %.2fs 才放弃，已不早于方法完成时刻 %s（未真正在预算内放弃）", out.elapsed.Seconds(), methodDelay)
	}
}

// readLateResponse 读取被放弃调用随后才写回的响应：证明插件没有被放弃动作卡死，
// 仍按原请求 id/req_id 产出合法帧（不是半帧或串包）。
func readLateResponse(t *testing.T, conn net.Conn, id, reqID string, within time.Duration) *pluginapi.Response {
	t.Helper()
	_ = conn.SetDeadline(time.Now().Add(within))
	defer func() { _ = conn.SetDeadline(time.Time{}) }()
	payload, err := pluginapi.ReadFrame(conn, pluginapi.MaxFrameSize)
	if err != nil {
		t.Fatalf("读取被放弃调用的迟到响应失败（插件可能卡死）：%v", err)
	}
	envelope, err := pluginapi.DecodeFrame(payload)
	if err != nil {
		t.Fatalf("解码迟到响应失败：%v", err)
	}
	response, ok := envelope.(*pluginapi.Response)
	if !ok {
		t.Fatalf("迟到帧类型是 %T，不是 response", envelope)
	}
	if response.ID != id || response.ReqID != reqID {
		t.Fatalf("迟到响应 id/req_id=%q/%q，期望 %q/%q（帧流已串包）", response.ID, response.ReqID, id, reqID)
	}
	if response.Error != nil {
		t.Fatalf("迟到的 slow 响应带错误：%s %s", response.Error.Code, response.Error.Message)
	}
	return response
}

// pingStatus 是 kore.ping 结果中本文件关心的字段（PLUGIN-PROTOCOL §7.1）。
type pingStatus struct {
	Status     string `json:"status"`
	UptimeS    int64  `json:"uptime_s"`
	Goroutines int    `json:"goroutines"`
	RSSKB      int64  `json:"rss_kb"`
}

// queryPing 调 kore.ping 并解析状态，作为插件存活与 goroutine 基线的观测口。
func queryPing(t *testing.T, conn net.Conn, tag string) pingStatus {
	t.Helper()
	out := callWithBudget(conn, "req-ping-"+tag, "r-ping-"+tag, "kore.ping", nil, 0, 2*time.Second)
	if out.err != nil {
		t.Fatalf("kore.ping(%s) 失败：%v", tag, out.err)
	}
	if out.response.Error != nil {
		t.Fatalf("kore.ping(%s) 返回错误：%s %s", tag, out.response.Error.Code, out.response.Error.Message)
	}
	var status pingStatus
	if err := json.Unmarshal(out.response.Result, &status); err != nil {
		t.Fatalf("解析 kore.ping(%s) 结果失败：%v，原文=%s", tag, err, out.response.Result)
	}
	return status
}

// assertNoTempFiles 断言插件进程的 TMPDIR 里没有留下任何文件（临时文件残留）。
func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读取插件 TMPDIR 失败：%v", err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("插件在 TMPDIR 留下 %d 个文件/目录：%v", len(entries), names)
	}
}

// TestSlowCallerBudgetAbandon：slow 需 1s 才返回，调用方预算 250ms → 必须在预算内放弃，
// 放弃后插件无 goroutine / 进程 / 临时文件残留，且仍能继续服务（迟到响应 + ping 正常）。
func TestSlowCallerBudgetAbandon(t *testing.T) {
	const budget = 250 * time.Millisecond
	pluginTmp := t.TempDir()
	kernel := newFakeKernel(t)
	cmd, stderr := startPluginIn(t, kernel.socket, "TMPDIR="+pluginTmp)
	conn := kernel.accept(t)
	hello := readRequest(t, conn)
	if hello.Method != "kore.hello" {
		t.Fatalf("首帧方法 = %q，期望 kore.hello", hello.Method)
	}
	replyHello(t, conn, hello, true, pluginapi.Protocol)

	// 基线：调用前插件 goroutine 数（插件自己的 kore.ping 读数，不依赖外部 pprof）。
	before := queryPing(t, conn, "before")

	// 内核 ctx 只剩 250ms 时，帧上 timeout_ms 会被规范化成 1000ms 下限（pluginapi.Timeout）。
	out := callWithBudget(conn, "req-slow-abandon", "r-slow-abandon", "echo.slow", nil,
		pluginapi.Timeout(int(budget/time.Millisecond)), budget)
	assertCallerAbandoned(t, out, budget, time.Second)

	// 迟到响应：插件在放弃后仍完成 slow 并写出合法帧 → 未卡死、未串包。
	late := readLateResponse(t, conn, "req-slow-abandon", "r-slow-abandon", 3*time.Second)
	if string(late.Result) != `{"slept_ms":1000}` {
		t.Fatalf("迟到 slow 结果 = %s，期望 {\"slept_ms\":1000}", late.Result)
	}

	// 残留：放弃一次调用后 goroutine 不得增长（NumGoroutine 由插件自己上报）。
	after := queryPing(t, conn, "after")
	if after.Goroutines > before.Goroutines {
		t.Fatalf("放弃 slow 后插件 goroutine 从 %d 涨到 %d，疑似泄漏", before.Goroutines, after.Goroutines)
	}

	// 业务 ping（缺口方法）必须是纯内存即时返回，同时作为放弃后的存活探针。
	bizPing := callWithBudget(conn, "req-biz-ping", "r-biz-ping", "echo.ping", nil, 0, 2*time.Second)
	if bizPing.err != nil || bizPing.response == nil {
		t.Fatalf("放弃后 echo.ping 失败：err=%v", bizPing.err)
	}
	if string(bizPing.response.Result) != `{"pong":true}` {
		t.Fatalf("echo.ping 结果 = %s，期望 {\"pong\":true}", bizPing.response.Result)
	}

	// 进程残留：正常 shutdown 后必须退出 0，不留后台进程。
	shutdown := callWithBudget(conn, "req-shutdown", "r-shutdown", "kore.shutdown", nil, 0, 2*time.Second)
	if shutdown.err != nil || shutdown.response == nil || shutdown.response.Error != nil {
		t.Fatalf("shutdown 失败：err=%v resp=%+v", shutdown.err, shutdown.response)
	}
	if code := waitExit(t, cmd, 5*time.Second); code != 0 {
		t.Fatalf("shutdown 后退出码 = %d，期望 0；stderr=%s", code, stderr.String())
	}
	assertNoTempFiles(t, pluginTmp)
}

// TestTimeoutCallerBudgetAbandon：timeout 需 3s 才返回，调用方预算 300ms → 必须在预算内
// 放弃；随后调用方关闭连接，插件必须自行退出（不因在途 sleep 而残留进程）。
func TestTimeoutCallerBudgetAbandon(t *testing.T) {
	const budget = 300 * time.Millisecond
	pluginTmp := t.TempDir()
	kernel := newFakeKernel(t)
	cmd, stderr := startPluginIn(t, kernel.socket, "TMPDIR="+pluginTmp)
	conn := kernel.accept(t)
	hello := readRequest(t, conn)
	if hello.Method != "kore.hello" {
		t.Fatalf("首帧方法 = %q，期望 kore.hello", hello.Method)
	}
	replyHello(t, conn, hello, true, pluginapi.Protocol)

	out := callWithBudget(conn, "req-timeout-abandon", "r-timeout-abandon", "echo.timeout", nil,
		pluginapi.Timeout(int(budget/time.Millisecond)), budget)
	// 方法本身 3s；调用方必须在远早于它时放弃。
	assertCallerAbandoned(t, out, budget, 3*time.Second)

	// 调用方放弃后直接断开：插件完成在途 sleep 后写响应失败 / 读到 EOF，必须退出。
	_ = conn.Close()
	code := waitExit(t, cmd, 6*time.Second)
	// 退出码取 0（读到 EOF 的优雅路径）或 1（写响应失败）都属「无残留进程」；
	// 这里只断言进程确实被回收，并记录实际码，避免把既有协议分支的差异写成通过。
	if code != 0 && code != 1 {
		t.Fatalf("放弃并断开后退出码 = %d，期望 0 或 1；stderr=%s", code, stderr.String())
	}
	t.Logf("timeout 被放弃后插件退出码 = %d（0=EOF 优雅退出 / 1=写响应失败）", code)
	assertNoTempFiles(t, pluginTmp)
}

// TestPingNoFaultSurface：ping 没有可注入的故障面——无参数、无外部依赖、无定时器/并发路径；
// 唯一的外部读取是 /proc/self/statm 的 RSS（失败按 Tier B 降级返回 0，不阻塞、不改变 status），
// 注入「读 /proc 失败」只会得到 rss_kb=0 的降级值，不构成故障场景。判定「无需注入」。
// 本用例只锁正向契约，并作为 slow/timeout 放弃用例的存活 / goroutine 观测口。
func TestPingNoFaultSurface(t *testing.T) {
	kernel := newFakeKernel(t)
	conn, cmd, stderr := startAndHandshake(t, kernel)

	status := queryPing(t, conn, "contract")
	if status.Status != "ready" {
		t.Fatalf("kore.ping status = %q，期望 ready", status.Status)
	}
	if status.UptimeS < 0 {
		t.Fatalf("kore.ping uptime_s = %d，不应为负", status.UptimeS)
	}
	if status.Goroutines < 1 {
		t.Fatalf("kore.ping goroutines = %d，至少应有主 goroutine", status.Goroutines)
	}
	// rss_kb 在 Linux 读 /proc/self/statm；Tier B 平台按设计返回 0，只在 Linux 断言 >0。
	if runtime.GOOS == "linux" && status.RSSKB <= 0 {
		t.Fatalf("kore.ping rss_kb = %d，期望 >0（Linux /proc/self/statm）", status.RSSKB)
	}

	bizPing := callWithBudget(conn, "req-ping-biz", "r-ping-biz", "echo.ping", nil, 0, 2*time.Second)
	if bizPing.err != nil || bizPing.response == nil || bizPing.response.Error != nil {
		t.Fatalf("echo.ping 失败：err=%v resp=%+v", bizPing.err, bizPing.response)
	}
	if string(bizPing.response.Result) != `{"pong":true}` {
		t.Fatalf("echo.ping 结果 = %s，期望 {\"pong\":true}", bizPing.response.Result)
	}

	invoke(t, conn, "kore.shutdown", nil)
	if code := waitExit(t, cmd, 5*time.Second); code != 0 {
		t.Fatalf("shutdown 后退出码 = %d，期望 0；stderr=%s", code, stderr.String())
	}
}

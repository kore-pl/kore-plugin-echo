# kore-plugin-echo Usage

kore's M1 verification plugin (id `echo`): verifies the host's full `install → hello → invoke → crash-restart` chain. It provides no operational capability and is **not a production plugin** (`depth_level=basic`).

## Installation and compatibility

Install the released tar.gz with the kore plugin installer, or build standalone (`make build` / `make dist`). Protocol `kore.plugin.v1`, `requires_kore: >=0.1.0 <0.2.0`, `lifecycle=lazy` — the verification tool is invoked only during acceptance and never consumes a resident slot. It does four things: handshake, answer calls, simulate slow calls/timeouts/crashes, and exit gracefully on `kore.shutdown`. It does not read kernel `internal/`, use the network, or store business data.

## Methods

| Method | Behavior |
| --- | --- |
| `ping` / `echo.ping` | Returns `{"pong":true}` |
| `echo` / `echo.echo` | Echoes `params` (non-object input returns `E_INVALID`) |
| `slow` / `echo.slow` | Sleeps 1s, returns `{"slept_ms":1000}` |
| `timeout` / `echo.timeout` | Sleeps 3s, returns `{"slept_ms":3000}`, verifies call timeout |
| `crash` / `echo.crash` | Immediately `os.Exit(7)`, verifies restart counting |
| `kore.ping/reload/shutdown` | Mandatory protocol methods; `kore.reload` has no config and an empty `config_digest` |

Response frames echo the request's `id` and `req_id`; unknown methods return `E_NOTFOUND`. `stdout` carries protocol frames only; logs go to stderr.

## Environment variables

| Variable | Required | Description |
| --- | --- | --- |
| `KORE_PLUGIN_SOCK` | Yes | UDS path created by the host |
| `KORE_PLUGIN_ID` | No | Defaults to embedded `info.json.id` |
| `KORE_PLUGIN_TOKEN` | No | Reported only with `kore.hello` in M1; not yet validated |
| `KORE_LOG_LEVEL` | No | debug/info/warn/error, default warn |

## Known limitations

- kore M1 has no heartbeat loop or restart counting; `crash` exit code `7` is for supervisor observation.
- `web/` is empty: the verification plugin has no frontend.
- Linux is Tier A; cross-platform builds are unverified and `rss_kb` returns 0 elsewhere.

## Build and verification

```bash
make check   # gofmt + go vet + go test -timeout 200s (including race)
make build && make dist
```

## Security and contributing

Do not open public issues for vulnerabilities; disclose privately per [SECURITY.md](SECURITY.md). See [CONTRIBUTING.md](CONTRIBUTING.md). MIT licensed.

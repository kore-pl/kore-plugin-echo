# kore-plugin-echo: guía de uso

Plugin de verificación M1 de kore (id `echo`): comprueba la cadena completa `install → hello → invoke → reinicio tras fallo` del host. No ofrece capacidad operativa y **no es un plugin de producción** (`depth_level=basic`).

## Instalación y compatibilidad

Instale el tar.gz publicado con el instalador de plugins de kore, o compílelo por separado (`make build` / `make dist`). Protocolo `kore.plugin.v1`, `requires_kore: >=0.1.0 <0.2.0`, `lifecycle=lazy`: la herramienta se invoca solo durante la aceptación y nunca ocupa un espacio residente. Hace cuatro cosas: handshake, responder llamadas, simular lentitud/timeout/fallo y salir limpiamente con `kore.shutdown`. No lee `internal/` del kernel, no usa red y no guarda datos de negocio.

## Métodos

| Método | Comportamiento |
| --- | --- |
| `ping` / `echo.ping` | Devuelve `{"pong":true}` |
| `echo` / `echo.echo` | Devuelve `params` tal cual (entrada no objeto → `E_INVALID`) |
| `slow` / `echo.slow` | Duerme 1 s y devuelve `{"slept_ms":1000}` |
| `timeout` / `echo.timeout` | Duerme 3 s y devuelve `{"slept_ms":3000}`; verifica el timeout |
| `crash` / `echo.crash` | Ejecuta `os.Exit(7)`; verifica el conteo de reinicios |
| `kore.ping/reload/shutdown` | Métodos obligatorios; `kore.reload` sin configuración y `config_digest` vacío |

Las tramas de respuesta repiten `id` y `req_id` de la petición; método desconocido → `E_NOTFOUND`. `stdout` solo para tramas del protocolo; los registros van a stderr.

## Variables de entorno

| Variable | Obligatoria | Descripción |
| --- | --- | --- |
| `KORE_PLUGIN_SOCK` | Sí | Ruta UDS creada por el host |
| `KORE_PLUGIN_ID` | No | Por defecto, `info.json.id` integrado |
| `KORE_PLUGIN_TOKEN` | No | En M1 solo se envía con `kore.hello`; aún no se valida |
| `KORE_LOG_LEVEL` | No | debug/info/warn/error, por defecto warn |

## Limitaciones conocidas

- kore M1 no tiene bucle de latido ni conteo de reinicios; el código `7` de `crash` sirve al supervisor.
- `web/` está vacío: no hay frontend.
- Linux es Tier A; otras plataformas no están verificadas y `rss_kb` devuelve 0.

## Compilación y verificación

```bash
make check   # gofmt + go vet + go test -timeout 200s (incluye race)
make build && make dist
```

## Seguridad y contribución

No abra issues públicos para vulnerabilidades; infórmelas según [SECURITY.md](SECURITY.md). Vea [CONTRIBUTING.md](CONTRIBUTING.md). Licencia MIT.

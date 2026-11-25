# kore-plugin-echo — guia de uso

Plugin de verificação M1 do kore (id `echo`): verifica a cadeia completa do host `install → hello → invoke → reinício após falha`. Não oferece capacidade de operações e **não é um plugin de produção** (`depth_level=basic`).

## Instalação e compatibilidade

Instale o tar.gz publicado com o instalador de plugins do kore ou compile isoladamente (`make build` / `make dist`). Protocolo `kore.plugin.v1`, `requires_kore: >=0.1.0 <0.2.0`, `lifecycle=lazy` — a ferramenta é chamada apenas na aceitação e nunca ocupa um slot residente. Ela faz quatro coisas: handshake, responder chamadas, simular chamadas lentas/timeouts/falhas e sair graciosamente com `kore.shutdown`. Não lê o `internal/` do kernel, não usa rede e não armazena dados de negócio.

## Métodos

| Método | Comportamento |
| --- | --- |
| `ping` / `echo.ping` | Retorna `{"pong":true}` |
| `echo` / `echo.echo` | Retorna `params` sem alterações (entrada não objeto → `E_INVALID`) |
| `slow` / `echo.slow` | Dorme 1 s e retorna `{"slept_ms":1000}` |
| `timeout` / `echo.timeout` | Dorme 3 s e retorna `{"slept_ms":3000}`; verifica o timeout |
| `crash` / `echo.crash` | Executa `os.Exit(7)`; verifica a contagem de reinícios |
| `kore.ping/reload/shutdown` | Métodos obrigatórios do protocolo; `kore.reload` sem configuração e `config_digest` vazio |

Os quadros de resposta repete `id` e `req_id` da requisição; método desconhecido → `E_NOTFOUND`. `stdout` é reservado aos quadros do protocolo; os logs vão para stderr.

## Variáveis de ambiente

| Variável | Obrigatória | Descrição |
| --- | --- | --- |
| `KORE_PLUGIN_SOCK` | Sim | Caminho UDS criado pelo host |
| `KORE_PLUGIN_ID` | Não | Por padrão, `info.json.id` incorporado |
| `KORE_PLUGIN_TOKEN` | Não | Em M1 enviado apenas com `kore.hello`; ainda não validado |
| `KORE_LOG_LEVEL` | Não | debug/info/warn/error, padrão warn |

## Limitações conhecidas

- O kore M1 não tem loop de heartbeat nem contagem de reinícios; o código `7` de `crash` serve ao supervisor.
- `web/` está vazio: não há frontend.
- Linux é Tier A; outras plataformas não verificadas, `rss_kb` retorna 0.

## Compilação e verificação

```bash
make check   # gofmt + go vet + go test -timeout 200s (inclui race)
make build && make dist
```

## Segurança e contribuição

Não abra issues públicas para vulnerabilidades; divulgue-as conforme [SECURITY.md](SECURITY.md). Veja [CONTRIBUTING.md](CONTRIBUTING.md). Licença MIT.

# kore-plugin-echo — Anwendungshandbuch

kores M1-Verifikationsplugin (id `echo`): prüft die gesamte Kette `install → hello → invoke → Neustart nach Absturz` des Hosts. Es bietet keine Betriebsfunktion und ist **kein Produktions-Plugin** (`depth_level=basic`).

## Installation und Kompatibilität

Installieren Sie das veröffentlichte tar.gz mit dem kore-Plugin-Installer oder bauen Sie es eigenständig (`make build` / `make dist`). Protokoll `kore.plugin.v1`, `requires_kore: >=0.1.0 <0.2.0`, `lifecycle=lazy` — das Werkzeug wird nur bei der Abnahme gerufen und belegt nie einen Resident-Slot. Es macht vier Dinge: Handshake, Antworten auf Aufrufe, Simulieren von langen Aufrufen/Timeouts/Abstürzen und sauberer Exit bei `kore.shutdown`. Es liest nicht den Kernel-`internal/`, nutzt kein Netzwerk und speichert keine Fachdaten.

## Methoden

| Methode | Verhalten |
| --- | --- |
| `ping` / `echo.ping` | Liefert `{"pong":true}` |
| `echo` / `echo.echo` | Gibt `params` unverändert zurück (kein Objekt → `E_INVALID`) |
| `slow` / `echo.slow` | Schläft 1 s und liefert `{"slept_ms":1000}` |
| `timeout` / `echo.timeout` | Schläft 3 s und liefert `{"slept_ms":3000}`; prüft das Timeout |
| `crash` / `echo.crash` | Sofort `os.Exit(7)`; prüft die Neustart-Zählung |
| `kore.ping/reload/shutdown` | Obligatorische Protokollmethoden; `kore.reload` ohne Konfiguration, leeres `config_digest` |

Antwort-Frames spiegeln `id` und `req_id` der Anfrage; unbekannte Methode → `E_NOTFOUND`. `stdout` enthält nur Protokoll-Frames; Protokolle gehen nach stderr.

## Umgebungsvariablen

| Variable | Pflicht | Beschreibung |
| --- | --- | --- |
| `KORE_PLUGIN_SOCK` | Ja | Vom Host erzeugter UDS-Pfad |
| `KORE_PLUGIN_ID` | Nein | Standard: eingebettetes `info.json.id` |
| `KORE_PLUGIN_TOKEN` | Nein | In M1 nur mit `kore.hello` gemeldet, noch nicht geprüft |
| `KORE_LOG_LEVEL` | Nein | debug/info/warn/error, Standard warn |

## Bekannte Einschränkungen

- kore M1 hat keine Heartbeat-Schleife und keine Neustart-Zählung; Exit-Code `7` dient dem Supervisor.
- `web/` ist leer: kein Frontend.
- Linux ist Tier A; andere Plattformen ungeprüft, `rss_kb` liefert 0.

## Bauen und prüfen

```bash
make check   # gofmt + go vet + go test -timeout 200s (inkl. Race-Gate)
make build && make dist
```

## Sicherheit und Mitwirkung

Melden Sie Schwachstellen nicht in öffentlichen Issues, sondern gemäß [SECURITY.md](SECURITY.md). Beiträge: [CONTRIBUTING.md](CONTRIBUTING.md). MIT-Lizenz.

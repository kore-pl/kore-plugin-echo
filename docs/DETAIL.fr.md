# kore-plugin-echo — guide d'utilisation

Plugin de vérification M1 de kore (id `echo`) : il vérifie la chaîne complète `install → hello → invoke → redémarrage après plantage` de l'hôte. Il n'offre aucune fonction d'exploitation et **n'est pas un plugin de production** (`depth_level=basic`).

## Installation et compatibilité

Installez le tar.gz publié via l'installateur de plugins kore, ou compilez-le seul (`make build` / `make dist`). Protocole `kore.plugin.v1`, `requires_kore: >=0.1.0 <0.2.0`, `lifecycle=lazy` : l'outil n'est appelé que pendant la recette et n'occupe jamais un emplacement résident. Il fait quatre choses : handshake, répondre aux appels, simuler lenteur/timeout/plantage et quitter proprement sur `kore.shutdown`. Il ne lit pas le `internal/` du noyau, n'utilise pas le réseau et ne stocke aucune donnée métier.

## Méthodes

| Méthode | Comportement |
| --- | --- |
| `ping` / `echo.ping` | Renvoie `{"pong":true}` |
| `echo` / `echo.echo` | Renvoie `params` tel quel (entrée non-objet → `E_INVALID`) |
| `slow` / `echo.slow` | Dort 1 s et renvoie `{"slept_ms":1000}` |
| `timeout` / `echo.timeout` | Dort 3 s et renvoie `{"slept_ms":3000}` ; vérifie le timeout |
| `crash` / `echo.crash` | Exécute `os.Exit(7)` ; vérifie le comptage des redémarrages |
| `kore.ping/reload/shutdown` | Méthodes obligatoires ; `kore.reload` sans configuration et `config_digest` vide |

Les trames de réponse reprennent `id` et `req_id` de la requête ; méthode inconnue → `E_NOTFOUND`. `stdout` ne contient que les trames du protocole ; les journaux vont dans stderr.

## Variables d'environnement

| Variable | Obligatoire | Description |
| --- | --- | --- |
| `KORE_PLUGIN_SOCK` | Oui | Chemin UDS créé par l'hôte |
| `KORE_PLUGIN_ID` | Non | Par défaut, `info.json.id` intégré |
| `KORE_PLUGIN_TOKEN` | Non | En M1, envoyé seulement avec `kore.hello` ; pas encore vérifié |
| `KORE_LOG_LEVEL` | Non | debug/info/warn/error, par défaut warn |

## Limitations connues

- kore M1 n'a pas de boucle de battement ni de comptage des redémarrages ; le code `7` de `crash` sert au superviseur.
- `web/` est vide : aucun frontend.
- Linux est Tier A ; les autres plateformes ne sont pas vérifiées et `rss_kb` renvoie 0.

## Compilation et vérification

```bash
make check   # gofmt + go vet + go test -timeout 200s (y compris race)
make build && make dist
```

## Sécurité et contribution

N'ouvrez pas d'issue publique pour une vulnérabilité ; signalez-la selon [SECURITY.md](SECURITY.md). Voir [CONTRIBUTING.md](CONTRIBUTING.md). Licence MIT.

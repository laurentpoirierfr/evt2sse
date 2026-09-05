# evt2sse


Relais qui expose les notifications **PostgreSQL `NOTIFY`/`LISTEN`** derrière une façade **Server-Sent Events (SSE)**, avec une petite IHM de suivi.


![Aperçu animé de la présentation](assets/evt2sse.gif)



## Architecture

```
PostgreSQL ──(LISTEN)──> serveur evt2sse ──(SSE)──> clients navigateur
    ▲                                                    ▲
    └────────(pg_notify)─────────  POST /api/send ────────┘
```

- Le serveur garde **une connexion dédiée** en `LISTEN` sur un canal (défaut `evt2sse`).
- `POST /api/send` émet un `pg_notify` via un pool séparé (jamais de contention avec le LISTEN).
- `GET /api/listen` → flux SSE (`text/event-stream`) avec horodatage et id incrémental.
- L'IHM (page `/`) se connecte au flux et permet d'émettre des messages de test.

## Structure

```
cmd/evt2sse/main.go     point d'entrée (flags, connexion, arrêt propre)
cmd/simulator/          simulateur de test : émet et affiche les événements
internal/relay/         relais PostgreSQL LISTEN/NOTIFY (connexion dédiée + pool + reconnexion)
internal/server/        routes HTTP (/api/send, /api/listen, /api/status) et diffusion SSE
internal/web/           IHM embarquée via go:embed (index.html)
pkg/client/             client Go réutilisable (écoute SSE, envoi, statut)
```

## Lancer en local

```bash
docker compose up -d --build
# puis pour un test rapide :
curl -N http://localhost:8080/api/listen &
curl -X POST http://localhost:8080/api/send \
     -H 'Content-Type: application/json' \
     -d '{"channel":"evt2sse","payload":"{\"bonjour\":\"monde\"}"}'
```

Ouvrir http://localhost:8080 pour l'IHM.

## Options

```
./evt2sse -addr :8080 -channel evt2sse [-db postgres://...]
```

- `-db` : chaîne de connexion (sinon `PGURL`, `DATABASE_URL`, ou défaut local).
- `-channel` : canal NOTIFY/LISTEN par défaut.
- `-addr` : adresse d'écoute HTTP.

## API

| Route | Méthode | Description |
|-------|---------|-------------|
| `/api/send` | `POST` | Publie un message. Body `{"channel":"canal","payload":"..."}`. Émet un `pg_notify`. |
| `/api/listen` | `GET` | Flux SSE. Événements `ready` (connexion) et `message`. |
| `/api/channels` | `GET` | Liste des canaux écoutés : `{"default":"evt2sse","channels":["evt2sse"]}`. |
| `/api/channels` | `POST` | S'abonner à un canal : body `{"channel":"notif_jobs"}` (LISTEN). |
| `/api/channels/{name}` | `DELETE` | Se désabonner d'un canal (ferme son LISTEN). |
| `/api/status` | `GET` | JSON : canal, nb clients, dernier id, état de la base. |
| `/` | `GET` | IHM de suivi. |

**Multi-canaux** : le serveur écoute dynamiquement plusieurs canaux (une
connexion LISTEN par canal, avec reconnexion automatique individuelle). Tous
les événements reçus sont relayés à **tous** les clients SSE, tagués par leur
canal d'origine. L'IHM permet de s'abonner/se désabonner et de filtrer
l'affichage par canal.

Exemple de payload SSE reçu :

```
event: message
data: {"id":12,"channel":"evt2sse","payload":"{\"bonjour\":\"monde\"}","time":"2026-09-05T09:45:13.279361472Z"}
```

## Client Go (pkg/client)

`pkg/client` s'importe depuis vos projets via le module
`github.com/laurentpoirierfr/evt2sse/pkg/client`.

```go
import "github.com/laurentpoirierfr/evt2sse/pkg/client"

ctx := context.Background()
cli := client.New("http://localhost:8080")

// Écouter les événements (reconnexion automatique activée par défaut).
stream := cli.Listen(ctx)
defer stream.Close()
for evt := range stream.Events() {
    fmt.Printf("%s sur %s: %s\n", evt.Channel, evt.Time, evt.Payload)
}

// Émettre une notification.
err := cli.Send(ctx, "evt2sse", `{"event":"deploy","status":"ok"}`)
err = cli.SendJSON(ctx, "evt2sse", map[string]any{"n": 1}) // payload sérialisé en JSON

// Interroger l'état du serveur.
st, _ := cli.Status(ctx)
fmt.Println(st.Connected, st.Clients)

// Gérer les canaux écoutés par le serveur.
err = cli.Subscribe(ctx, "notif_jobs")        // LISTEN sur notif_jobs
err = cli.Unsubscribe(ctx, "notif_jobs")      // ferme l'écoute
chans, _ := cli.Channels(ctx)                 // ["evt2sse", ...]
```

Options : `WithHTTPClient`, `WithDefaultChannel`, et pour `Listen` :
`WithAutoReconnect(false)`.

## Simulateur (cmd/simulator)

Outil de test qui écoute le flux SSE, affiche les événements reçus sur la
console, et peut émettre des événements simulés via `pkg/client`.

```bash
go run ./cmd/simulator                          # émettre + afficher (1 envoi / 2 s)
go run ./cmd/simulator -send=false              # simple écoute
go run ./cmd/simulator -count 5 -interval 500ms # 5 envois rapides puis écoute
go run ./cmd/simulator -duration 30s            # s'arrête tout seul après 30 s
go run ./cmd/simulator -no-color                # sans couleurs ANSI
```

Flags : `-url` (défaut `http://localhost:8080`), `-channel`, `-payload` (modèle
avec `{{n}}` = compteur, `{{ts}}` = horodatage), `-interval`, `-count`,
`-duration`, `-send`, `-no-color`.

## Notes

- Utilisable aussi depuis n'importe quel client Postgres : tout `NOTIFY canal, '...'` fait sur la base est relayé aux clients SSE connectés.
- Le serveur se reconnecte automatiquement à Postgres (3 s de pause) en cas de coupure.
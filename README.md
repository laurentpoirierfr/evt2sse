<p align="center">
  <img src="assets/logo.svg" alt="evt2sse — Notifications PostgreSQL NOTIFY/LISTEN vers SSE" width="480">
</p>

# evt2sse

Relais qui expose les notifications **PostgreSQL `NOTIFY`/`LISTEN`** derrière une façade **Server-Sent Events (SSE)**, avec une petite IHM de suivi.

![Aperçu animé de la présentation](assets/evt2sse.gif)

## Fonctionnalités

- **`NOTIFY`/`LISTEN` → SSE** : une connexion dédiée en `LISTEN`, un flux `text/event-stream` multi-client avec horodatage et `id` incrémental.
- **Multi-canaux dynamiques** : abonnement/désabonnement à chaud (`POST`/`DELETE /api/channels`), une connexion `LISTEN` par canal avec reconnexion automatique individuelle ; tous les clients reçoivent tous les événements, taggués par leur canal d'origine.
- **Résilience** :
  - reconnexion PostgreSQL auto : backoff exponentiel ± 20 %, plafonné à 30 s, démarrage tolérant à une base indisponible ;
  - reprise **Last-Event-ID** après coupure (tampon des 256 derniers événements) ;
  - envoi **idempotent** : `id` dédupliqué 5 min côté relais ;
  - **heartbeat SSE** (15 s) : évite les timeouts de proxy et détecte les clientes coupées.
- **Prêt pour Kubernetes** : endpoints d'ops `/ops/liveness`, `/ops/readiness` (lié à l'état de la base) et `/ops/info` (version, commit, build), image **distroless non-root** multi-arch (amd64/arm64).
- **IHM embarquée** (`/`) via `go:embed` : connexion au flux, envoi de messages de test, filtrage par canal.
- **Client Go réutilisable** (`pkg/client`) et **simulateur** (`cmd/simulator`).
- **Déploiement** : `Dockerfile` multi-stage, `docker-compose.yaml` (db + app), **chart Helm** `k8s/helm/evt2sse`, intégration CI-friendly via `Makefile`.

## Architecture

```
PostgreSQL ──(LISTEN)──> serveur evt2sse ──(SSE)──> clients navigateur
    ▲                                                     ▲
    └────────(pg_notify)─────────  POST /api/send ────────┘
```

- Le serveur garde **une connexion dédiée** en `LISTEN` sur un canal (défaut `evt2sse`).
- `POST /api/send` émet un `pg_notify` via un pool séparé (jamais de contention avec le LISTEN).
- `GET /api/listen` → flux SSE (`text/event-stream`) avec horodatage et id incrémental.
- L'IHM (page `/`) se connecte au flux et permet d'émettre des messages de test.

## Structure

```
.
├── assets/       média du README (gif de présentation)
├── cmd/          binaires Go : l'application et son simulateur
├── internal/     code métier non réutilisable à l'extérieur du module
├── k8s/          déploiement Kubernetes : chart Helm de l'application
├── motion/       présentation Motion Canvas (rendu mp4/gif)
└── pkg/          bibliothèque Go réutilisable (client evt2sse)
```

| Répertoire | Contenu |
|------------|---------|
| `cmd/evt2sse/` | Point d'entrée : flags (`-addr`, `-db`, `-channel`), démarrage avec retry, arrêt propre sur signal |
| `cmd/simulator/` | Simulateur de test : écoute le flux SSE, affiche les événements, en émet d'éventuels simulés |
| `internal/relay/` | Relais `LISTEN`/`NOTIFY` : connexion dédiée + pool séparé, reconnexion backoff, multi-canaux, dédup |
| `internal/server/` | Routage HTTP (`/api/*`, `/ops/*`) et diffusion SSE : client map, heartbeat, historique + reprise |
| `internal/web/` | IHM embarquée via `go:embed` (`index.html`) |
| `internal/buildinfo/` | Métadonnées de build (version, commit, date) exposées par `/ops/info` |
| `pkg/client/` | Client Go réutilisable : `Listen` (auto-reconnect), `Send`, `Status`, `Subscribe`/`Unsubscribe`, `Channels` |
| `k8s/helm/evt2sse/` | Chart Helm (Deployment, Service, Secret, probes, ingress/HPA/PDB optionnels) — voir `k8s/helm/README.md` |
| `motion/` | Présentation animée Motion Canvas (6 scènes, `scripts/export.mjs` pour le rendu headless) |

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

Sans Docker, avec une base PostgreSQL locale (`make db-up` pour la démarrer) :

```bash
make run        # compile et lance avec pgurl par défaut
go run ./cmd/simulator   # émettre et afficher des événements (1 envoi / 2 s)
```

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
| `/ops/liveness` | `GET` | Le processus répond (`{"status":"alive"}`) — probe de liveness |
| `/ops/readiness` | `GET` | Prêt seulement si la base répond, sinon `503` — probe de readiness |
| `/ops/info` | `GET` | Version, commit, date, runtime Go (ldflags du Makefile) |
| `/api/send` | `POST` | Publie un message. Body `{"channel":"canal","payload":"...","id":"..."}`. Émet un `pg_notify` |
| `/api/listen` | `GET` | Flux SSE. Événements `ready` (connexion), `message`, et `resume` (reprise) |
| `/api/channels` | `GET` | Liste des canaux écoutés : `{"default":"evt2sse","channels":["evt2sse"]}` |
| `/api/channels` | `POST` | S'abonner à un canal : body `{"channel":"notif_jobs"}` (LISTEN) |
| `/api/channels/{name}` | `DELETE` | Se désabonner d'un canal (ferme son LISTEN) |
| `/api/status` | `GET` | JSON : canal, clients, dernier id, état de la base |
| `/` | `GET` | IHM de suivi |

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

## Déploiement

### Image conteneur

`Dockerfile` multi-stage (build golang → distroless **non-root**).
Construite et publiée via le Makefile (multi-arch amd64/arm64) :

```bash
make image                 # image locale
make image-multi           # multi-arch (builder buildx)
make image-push VERSION=0.1.0   # pousse ghcr.io/laurentpoirierfr/evt2sse
```

### Docker Compose

`docker compose up -d --build` démarre la base PostgreSQL et l'application.

### Kubernetes (Helm)

Chart Helm dans `k8s/helm/evt2sse` : deployment de **l'application seule**
(PostgreSQL attendu à l'extérieur du chart), probes `/ops/*`, secrets
`PGURL`, ingress/HPA/PDB/NetworkPolicy optionnels.

```bash
helm upgrade --install evt2sse ./k8s/helm/evt2sse \
  --set postgresUrl='postgres://evt2sse:motdepasse@db-postgres:5432/evt2sse?sslmode=require'
```

→ Documentation complète du chart : [`k8s/helm/README.md`](k8s/helm/README.md).

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
- Le serveur se reconnecte automatiquement à Postgres (backoff exponentiel ± 20 %, plafonné à 30 s) en cas de coupure, et supporte la reprise des événements manqués via `Last-Event-ID`.
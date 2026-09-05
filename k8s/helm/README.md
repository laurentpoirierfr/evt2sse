# Chart Helm evt2sse

Chart Helm pour déployer **uniquement l'application `evt2sse`** sur un cluster
Kubernetes. Le relais écoute les notifications PostgreSQL `NOTIFY`/`LISTEN` et
les expose en Server-Sent Events (SSE).

> Le chart ne déploie **que l'application** : PostgreSQL n'est pas
> déployé par ce chart. Une instance (interne au cluster ou externe) doit déjà
> être disponible, et sa chaîne de connexion est indiquée via les valeurs du
> chart (voir [Connexion à PostgreSQL](#connexion-à-postgresql)).

## Prérequis

- Kubernetes ≥ 1.19 (Deployment `apps/v1`, NetworkPolicy `networking.k8s.io/v1`,
  HPA `autoscaling/v2`, PDB `policy/v1`)
- Helm ≥ 3.8
- Une instance PostgreSQL accessible depuis le cluster (base existante)

## Installation rapide

Depuis la racine du dépôt :

```bash
# Namespace, ici : evt2sse
kubectl create namespace evt2sse

# Cas 1 — chaîne de connexion en paramètre (un Secret est créé par le chart)
helm upgrade --install evt2sse ./k8s/helm/evt2sse -n evt2sse \
  --set postgresUrl='postgres://evt2sse:motdepasse@db-postgres:5432/evt2sse?sslmode=require'

# Cas 2 — Secret existant géré par vos soins (clé PGURL)
kubectl -n evt2sse create secret generic evt2sse-pg \
  --from-literal=PGURL='postgres://evt2sse:motdepasse@db-postgres:5432/evt2sse?sslmode=require'
helm upgrade --install evt2sse ./k8s/helm/evt2sse -n evt2sse \
  --set existingSecret=evt2sse-pg
```

L'installation échoue avec un message explicite si ni `postgresUrl` ni
`existingSecret` n'est renseigné.

## Connexion à PostgreSQL

L'application lit sa connexion via la variable d'environnement `PGURL`
(`postgres://user:password@host:5432/db?sslmode=...`).

Deux façons de la fournir au chart :

| Mode | Valeur(s) | Comportement |
|------|-----------|--------------|
| Chaîne de connexion | `postgresUrl` | Un Secret `<release>-pg` est créé par le chart avec la clé `PGURL` |
| Secret existant | `existingSecret` (+ `existingSecretKey`, défaut `PGURL`) | Aucun Secret créé ; des clés `secretKeyRef` points sur votre secret |

Le Secret n'apparaît jamais en clair dans le manifest du Deployment : le Pod
reçoit `PGURL` via `valueFrom.secretKeyRef`. Si la valeur change
(`postgresUrl` ou `existingSecret`), le Deployment est redéployé
(annotation `checksum/pgurl`).

### Environnement externe vs hébergé

- **Base hébergée en dehors du cluster** : `postgresUrl` doit être publique
  et accessible (`sslmode=require` ou `verify-full` recommandé).
- **Base interne au cluster** (autre namespace) : la valeur attendue du
  `host` est le nom DNS du Service, ex. `postgres-db.ns-db.svc.cluster.local`.
- L'application retente automatiquement la connexion à la base selon un
  backoff exponentiel (1 s → max 30 s, jitter ± 20 %) : elle démarre même si
  la base est momentanément indisponible.

## Paramètres

| Paramètre | Description | Défaut |
|-----------|-------------|--------|
| `replicaCount` | Nombre de réplicas | `1` |
| `image.repository` | Image conteneur | `ghcr.io/laurentpoirierfr/evt2sse` |
| `image.tag` | Tag de l'image (défaut : `appVersion` du chart) | `""` |
| `image.pullPolicy` | Politique de pull | `IfNotPresent` |
| `imagePullSecrets` | Secrets de pull (registres privés) | `[]` |
| `nameOverride` / `fullnameOverride` | Surcharge des noms de ressources | `""` |
| `channel` | Canal `NOTIFY`/`LISTEN` par défaut (arg `-channel`) | `evt2sse` |
| `postgresUrl` | Chaîne de connexion PostgreSQL | `""` |
| `existingSecret` | Secret existant contenant `PGURL` | `""` |
| `existingSecretKey` | Clé à lire dans le secret existant | `PGURL` |
| `serviceAccount.create` | Créer un ServiceAccount | `true` |
| `serviceAccount.name` / `annotations` / `automount` | Réglages du SA | — |
| `podAnnotations` | Annotations des Pods | `{}` |
| `podSecurityContext` | Contexte de sécurité au niveau Pod | `fsGroup: 65532` |
| `securityContext` | Contexte de sécurité du conteneur (non-root) | drop `ALL`, RO fs |
| `service.type` | Type du Service | `ClusterIP` |
| `service.port` | Port HTTP (et `-addr` de l'application) | `8080` |
| `ingress.enabled` | Activer l'Ingress | `false` |
| `ingress.className` / `annotations` / `hosts` / `tls` | Réglages Ingress | — |
| `resources.limits` / `resources.requests` | CPU/mémoire | `500m/128Mi` / `100m/64Mi` |
| `autoscaling.enabled` | Créer un HPA (CPU) | `false` |
| `autoscaling.minReplicas` / `maxReplicas` | Bornes du HPA | `1` / `5` |
| `autoscaling.targetCPU*` / `targetMemory*` | Cibles d'utilisation | `80` / `""` |
| `pdb.enabled` / `pdb.minAvailable` | PodDisruptionBudget | `false` / `1` |
| `networkPolicy.enabled` | Créer les NetworkPolicies | `false` |
| `nodeSelector` / `tolerations` / `affinity` | Placement des Pods | `{}` / `[]` / `{}` |
| `topologySpreadConstraints` | Répartition multi-zone | `[]` |

## Probes (liveness / readiness)

- `livenessProbe` → `GET /ops/liveness` : le processus répond, Pod vivant
  (période 10 s).
- `readinessProbe` → `GET /ops/readiness` : renvoie `200` uniquement si la
  connexion à PostgreSQL est saine (sinon `503`, période 5 s). Le Pod n'est
  donc mis en service que lorsque la base répond.

## Tester le déploiement

```bash
# Suivi des Pods
kubectl get pods -n evt2sse -w
kubectl logs -n evt2sse deploy/evt2sse -f

# Forward vers la machine locale
kubectl port-forward -n evt2sse svc/evt2sse 8080:8080

# Dans un autre terminal — écouter le flux SSE, puis émettre un événement
curl -N http://localhost:8080/api/listen &
curl -X POST http://localhost:8080/api/send \
  -H 'Content-Type: application/json' \
  -d '{"channel":"evt2sse","payload":"{\"bonjour\":\"monde\"}"}'

# IHM de suivi
open http://localhost:8080
```

Le flux SSE reçu ressemble à :

```
id: 1
data: {"id":1,"channel":"evt2sse","payload":"{\"bonjour\":\"monde\"}","time":"2026-09-05T10:00:00.000000000Z"}
```

## Endpoints exposés

| Route | Méthode | Description |
|-------|---------|-------------|
| `/ops/liveness` | `GET` | Vivacité du processus (probe liveness) |
| `/ops/readiness` | `GET` | Prêt si la base répond (probe readiness) |
| `/ops/info` | `GET` | Version, commit, date de build |
| `/api/send` | `POST` | Publier un événement (`{"channel","payload","id"}`) |
| `/api/listen` | `GET` | Flux SSE (`text/event-stream`) |
| `/api/channels` | `GET`/`POST` | Lister / s'abonner à un canal |
| `/api/channels/{name}` | `DELETE` | Se désabonner d'un canal |
| `/api/status` | `GET` | Canal, clients, dernier id, état de la base |
| `/` | `GET` | IHM de suivi |

## Mise à jour, rollback, désinstallation

```bash
# Mise à jour (valeurs ou nouvelle version du chart)
helm upgrade --install evt2sse ./k8s/helm/evt2sse -n evt2sse \
  --reuse-values --set image.tag=0.2.0

# Historique et rollback
helm history evt2sse -n evt2sse
helm rollback evt2sse <REVISION> -n evt2sse

# Désinstallation (le Secret et le SA du chart sont supprimés ; les objets
# que vous avez créés manuellement, comme evt2sse-pg, restent)
helm uninstall evt2sse -n evt2sse
```

## Exemples avancés

### Ingress (traefik / nginx) + TLS

```bash
helm upgrade --install evt2sse ./k8s/helm/evt2sse -n evt2sse \
  --set postgresUrl='postgres://...' \
  --set ingress.enabled=true \
  --set ingress.className=nginx \
  --set ingress.hosts[0].host=evt2sse.example.com \
  --set ingress.tls[0].hosts[0]=evt2sse.example.com \
  --set ingress.tls[0].secretName=evt2sse-tls
```

### Haute disponibilité (HPA + PDB + affinité)

```bash
helm upgrade --install evt2sse ./k8s/helm/evt2sse -n evt2sse \
  --set postgresUrl='postgres://...' \
  --set autoscaling.enabled=true \
  --set autoscaling.minReplicas=2 \
  --set autoscaling.maxReplicas=6 \
  --set pdb.enabled=true \
  --set pdb.minAvailable=1 \
  --set 'affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[0].weight=100' \
  --set 'affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[0].podAffinityTerm.topologyKey=topology.kubernetes.io/zone' \
  --set 'affinity.podAntiAffinity.preferredDuringSchedulingIgnoredDuringExecution[0].podAffinityTerm.labelSelector.matchLabels.app.kubernetes.io/name=evt2sse'
```

### Isolation réseau (NetworkPolicy)

```bash
helm upgrade --install evt2sse ./k8s/helm/evt2sse -n evt2sse \
  --set postgresUrl='postgres://...' \
  --set networkPolicy.enabled=true
```

La NetworkPolicy autorise le trafic HTTP entrant vers le Service et la
résolution DNS sortante. L'accès sortant vers votre base PostgreSQL doit être
ajouté selon votre configuration (le chart ne connaît pas l'emplacement de la
base en mode externe).

## Architecture du chart

```
k8s/helm/evt2sse/
├── Chart.yaml                      # métadonnées du chart
├── values.yaml                     # valeurs par défaut (documentées ci-dessus)
└── templates/
    ├── _helpers.tpl                # helpers de nommage, labels, PGURL
    ├── deployment.yaml             # Deployment + args, env, probes, sécurité
    ├── serviceaccount.yaml         # ServiceAccount
    ├── secret.yaml                 # Secret PGURL (si postgresUrl, pas existingSecret)
    ├── service.yaml                # Service (ClusterIP)
    ├── ingress.yaml                # Ingress optionnel
    ├── hpa.yaml                    # HorizontalPodAutoscaler optionnel
    ├── pdb.yaml                    # PodDisruptionBudget optionnel
    ├── networkpolicy.yaml          # NetworkPolicy optionnelle
    ├── NOTES.txt                   # résumé affiché par helm install
    └── tests/
        └── test-connection.yaml    # test helm (GET /ops/liveness)
```
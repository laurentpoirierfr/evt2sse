# Nomenclature des canaux evt2sse

Cette convention définit la manière de nommer les canaux de notification
(`NOTIFY`/`LISTEN` PostgreSQL) exposés par evt2sse, dans un contexte
professionnel multi-domaines.

Le nom de canal est un élément d'architecture à part entière : il est visible
de **tous** les consommateurs SSE, sert de support au routage, au filtrage et
à l'audit. Il doit donc être unique, lisible et stable.

---

## 1. Format général

```
<domaine>.<contexte>.<événement>
```

| Niveau | Rôle | Exemples |
|---|---|---|
| `domaine` | capacité métier (bounded context) | `orders`, `billing`, `catalog`, `hr`, `auth`, `logistics` |
| `contexte` | aggrégat / entité concerné(e) | `order`, `invoice`, `product`, `employee`, `shipment` |
| `événement` | ce qui s'est produit (participe passé) | `created`, `updated`, `paid`, `cancelled`, `onboarded` |

### 1.1. Exemples

| Canal | Signification |
|---|---|
| `orders.order.created` | une commande a été créée |
| `orders.order.cancelled` | une commande a été annulée |
| `billing.invoice.paid` | une facture a été payée |
| `catalog.product.price_updated` | le prix d'un produit a été modifié |
| `hr.employee.onboarded` | un employé a été intégré |
| `logistics.shipment.delivered` | une livraison est arrivée |
| `system.health` | événement d'exploitation (canal réservé) |

### 1.2. Format étendu (versionnage)

En cas de changement du schéma du payload, on crée un nouveau canal au lieu de
modifier le contrat existant :

```
<domaine>.<contexte>.<événement>.v<N>
```

- `orders.order.created.v1`
- `orders.order.created.v2`

Le versionnage reste **optionnel** : on ne crée `.v1` que lorsque le risque de
rupture est réel. Tant que possible, on préfère faire évoluer le payload sans
changer de canal (ajout de champs non destructifs) et publier une **nouvelle
version** uniquement pour les changements incompatibles.

---

## 2. Règles obligatoires

### 2.1. Format des segments

Chaque segment suit le motif `[a-z][a-z0-9]*` :

- minuscules ASCII uniquement (pas d'accents, pas de majuscules) ;
- chiffres autorisés (`price_updated` non, mais `price2` oui — cf. § 2.4) ;
- pas de tiret `-`, pas d'espace, pas de caractère spécial.

> Rappel technique : PostgreSQL limite le nom de canal à **63 octets**
> (`NAMEDATALEN - 1`). Les points fonctionnent car le canal est passé entre
> guillemets lors du `LISTEN` (`LISTEN "orders.order.created"`).

### 2.2. Nombre de segments

- **3 segments** : standard.
- **4 segments** : réservé au suffixe de version (`v1`, `v2`…).
- Au-delà : signe d'un découpage trop fin → redescendre vers l'aggrégat.

### 2.3. Longueur

Plafond recommandé : **< 50 octets** sur l'ensemble du nom, pour laisser de la
marge vis-à-vis de la limite PostgreSQL.

### 2.4. Séparateurs de mots

Aucun : un segment est un mot unique (`priceupdated`, `customerorder`). Si la
lisibilité impose une séparation, préférer restructurer le nom (réduire à
l'aggrégat) plutôt que d'introduire `_`.

*(Alternative tolérée : `_` entre mots dans un segment, ex.
`catalog.product.price_updated` — si l'équipe la préfère, l'utiliser
uniformément partout.)*

### 2.5. Temps des verbes

On nomme **ce qui s'est produit** (participé passé) :

| ✔ | ✘ |
|---|---|
| `order.paid` | `order.pay` |
| `invoice.issued` | `invoice.issue` |
| `employee.onboarded` | `employee.onboard` |

evt2sse est un bus d'**événements** (faits). Les demandes d'action sont des
**commandes** et n'ont pas leur place dans la nomenclature.

### 2.6. Langue

Une seule langue dans le référentiel : l'**anglais** par défaut (standard
international des équipes et outils). Le français est possible si l'entreprise
est 100 % francophone — l'essentiel est l'uniformité.

---

## 3. Ce qu'on ne met jamais dans un nom de canal

- **Environnement** (`prod.`, `staging.`, `dev.`) : les environnements sont
  séparés par l'infrastructure (instances evt2sse / PostgreSQL distinctes), pas
  par le nom de canal.
- **Données sensibles** : un identifiant client, une raison sociale, tout
  élément qui permettrait d'identifier une personne ou une entité métier.
  Le nom de canal est diffusé à tous les clients SSE.
- **Identifiants d'instances** (`node-3`, `host-42`) : il s'agit de détails
  d'infrastructure.
- **Version d'application** (`app-2.3.1`) : gérée ailleurs.

---

## 4. Canaux réservés

Le préfixe `system.` est réservé au métier d'evt2sse et à l'exploitation :

| Canal | Usage |
|---|---|
| `system.health` | battement de cœur / cœur de l'infra |
| `system.channels` | événements de cycle de vie des canaux (abonnement…) |

Le canal par défaut de l'application (`evt2sse`) reste utilisable pour les
tests ; il n'est pas destiné à la production.

---

## 5. Gouvernance

### 5.1. Catalogue unique

Chaque canal doit être déclaré dans un référentiel unique (`channels.yaml`,
table SQL, ou fichier dans le dépôt) :

```yaml
- name: orders.order.created
  owner: team-orders
  event: Commande créée (ajout au panier validé)
  payload_schema: orders/order_created.v1.json
  status: active
```

### 5.2. Validation automatisée

Le serveur refuse tout abonnement (`POST /api/channels`) dont le nom ne
respecte pas la nomenclature **et/ou** n'est pas déclaré au catalogue.
Expression régulière de référence :

```
^[a-z][a-z0-9]{0,18}(\.[a-z][a-z0-9]{0,18}){1,3}$
```

### 5.3. Abonnement par préfixe

Les consommateurs s'abonnent par **préfixe de domaine** plutôt qu'à des canaux
unitaires, pour être robustes à l'ajout de nouveaux événements :

- `orders.*` couvre `orders.order.created`, `orders.order.cancelled`, …
- nouvelle publication d'événement ⇒ aucun changement côté consommateur.

### 5.4. Cycle de vie

| Statut | Action attendue |
|---|---|
| `active` | canal publié et écouté |
| `deprecated` | plus de publications, mais abonnés encore tolérés |
| `retired` | plus d'abonnements ; libération du nom (pas de réutilisation immédiate) |

---

## 6. Antipatterns fréquents

| ✘ | Pourquoi |
|---|---|
| `notify`, `news`, `message` | trop générique, aucun routage possible |
| `orders.created` | contexte manquant, collision avec d'autres aggrégats du domaine |
| `prod.orders.order.created` | l'environnement n'a pas sa place dans le nom |
| `Order.Order.Created` | majuscules interdites (casse) |
| `commandes.commandes.creees` | mélange français/anglais |
| `a.b.c.d.e.f` | découpage trop fin → redescendre sur l'aggrégat |

---

## 7. Récapitulatif express

```
1. <domaine>.<contexte>.<événement>            (3 segments)
2. minuscules ASCII + chiffres, séparés par des points
3. verbe au participe passé, en anglais
4. ≤ 50 octets, jamais de données sensibles
5. pas d'environnement dans le nom
6. <domaine>.* pour s'abonner à toute une capacité métier
7. versionner avec .vN seulement en cas de rupture
```
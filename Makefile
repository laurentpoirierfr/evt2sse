# Makefile pour evt2sse — développement local, tests, build et conteneur

BINARY     := evt2sse
CMD        := ./cmd/evt2sse
IMG        ?= evt2sse:local
IMG_LABEL  ?= ghcr.io/laurentpoirierfr/evt2sse

# Métadonnées de version injectées à la compilation (endpoint /ops/info)
VERSION    ?= dev
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo n/a)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -s -w \
  -X github.com/laurentpoirierfr/evt2sse/internal/buildinfo.Version=$(VERSION) \
  -X github.com/laurentpoirierfr/evt2sse/internal/buildinfo.Commit=$(COMMIT) \
  -X github.com/laurentpoirierfr/evt2sse/internal/buildinfo.Date=$(BUILD_DATE)

# Nombre de CPU pour la compilation
JOBS ?= $(shell nproc 2>/dev/null || echo 4)

.PHONY: all build test vet fmt lint run db-up db-down compose-up compose-down \
	clean image

all: vet fmt test build

## build : compile le binaire dans ./evt2sse
build:
	GOFLAGS=-mod=mod CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) $(CMD)

## test : lance les tests Go
test:
	go test ./...

## vet : analyse statique du code
vet:
	go vet ./...

## fmt : vérifie le formatage (affiche les fichiers non conformes)
fmt:
	@test -z "$$(gofmt -l .)" || { echo "fichiers non formatés:"; gofmt -l .; exit 1; }

## lint : gofmt puis golangci-lint (v1.60+ requis pour go1.25), sinon go vet
lint: fmt
	@if command -v golangci-lint >/dev/null 2>&1; then \
		VER="$$(golangci-lint --version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+' | head -1 | tr -d v)"; \
		NEED="1.60"; \
		if [ "$$(printf '%s\n%s\n' "$$NEED" "$$VER" | sort -V | head -1)" = "$$NEED" ]; then \
			golangci-lint run ./...; \
		else \
			echo "golangci-lint $${VER} trop ancien (>=$${NEED} requis) — utilisation de go vet"; \
			go vet ./...; \
		fi \
	else \
		echo "golangci-lint non installé — utilisation de go vet"; \
		go vet ./...; \
	fi

## run : compile et lance en local (postgres requis, voir db-up)
run: build
	./$(BINARY) -db '$(PGURL)'

## db-up : démarre une base PostgreSQL locale via docker
db-up:
	docker run -d --rm --name evt2sse-pg \
		-e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=postgres -e POSTGRES_DB=postgres \
		-p 5432:5432 postgres:17-alpine

## db-down : arrête la base locale
db-down:
	docker rm -f evt2sse-pg 2>/dev/null || true

## compose-up : lance toute la stack (db + app) via docker compose
compose-up:
	docker compose up -d --build

## compose-down : arrête la stack
compose-down:
	docker compose down

## image : construit l'image conteneur (plateforme hôte)
image:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMG) .

## image-multi : construit l'image multi-arch (nécessite un builder buildx)
image-multi:
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMG) .

## image-push : publie l'image multi-arch sur le registre $(IMG_LABEL)
image-push: image-multi
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMG_LABEL):$(VERSION) \
		-t $(IMG_LABEL):latest \
		--push .

## clean : supprime les artefacts de build
clean:
	rm -f $(BINARY) rescheck simulator integtest

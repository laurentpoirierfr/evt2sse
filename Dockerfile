# ---- Étage de construction -------------------------------------------------
FROM golang:1.25-alpine AS build
ARG VERSION=dev
ARG COMMIT=n/a
ARG BUILD_DATE=
WORKDIR /src

# Cache des dépendances
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags "-s -w \
      -X github.com/laurentpoirierfr/evt2sse/internal/buildinfo.Version=${VERSION} \
      -X github.com/laurentpoirierfr/evt2sse/internal/buildinfo.Commit=${COMMIT} \
      -X github.com/laurentpoirierfr/evt2sse/internal/buildinfo.Date=${BUILD_DATE}" \
    -o /out/evt2sse ./cmd/evt2sse

# ---- Étage final (distroless, non-root, multi-arch) ------------------------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/evt2sse /usr/local/bin/evt2sse
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/evt2sse"]

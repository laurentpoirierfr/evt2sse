FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/evt2sse ./cmd/evt2sse

FROM alpine:3.20
RUN adduser -D -u 10001 app
USER app
COPY --from=build /out/evt2sse /usr/local/bin/evt2sse
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/evt2sse"]

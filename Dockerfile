FROM golang:1.21-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -a -installsuffix cgo \
    -ldflags='-w -s -extldflags "-static"' \
    -o ogrok-server ./cmd/server

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata wget && \
    update-ca-certificates

RUN addgroup -g 1001 ogrok && \
    adduser -D -s /bin/sh -u 1001 -G ogrok ogrok

RUN mkdir -p /var/lib/ogrok/certs /etc/ogrok && \
    chown -R ogrok:ogrok /var/lib/ogrok

COPY --from=builder /app/ogrok-server /usr/local/bin/ogrok-server
RUN chmod +x /usr/local/bin/ogrok-server

COPY --chown=ogrok:ogrok configs/server.yaml /etc/ogrok/server.yaml

USER ogrok:ogrok
WORKDIR /var/lib/ogrok

EXPOSE 80 443 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

ENTRYPOINT ["/usr/local/bin/ogrok-server"]
CMD ["-config", "/etc/ogrok/server.yaml"]

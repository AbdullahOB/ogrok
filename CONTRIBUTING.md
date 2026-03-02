# Contributing

## Setup

```bash
git clone https://github.com/yourusername/ogrok.git
cd ogrok
make build-all
```

## Development

```bash
# run server
./bin/ogrok-server -config configs/server.yaml

# run client (separate terminal)
./bin/ogrok http 3000 --server localhost:8080 --token dev-token-123
```

## Submitting changes

1. Fork and create a feature branch
2. Make your changes
3. Run `go fmt ./...` and `go vet ./...`
4. Run `make test`
5. Open a pull request

## Bug reports

Use the GitHub issue tracker. Include steps to reproduce, expected vs actual
behavior, and your environment (OS, Go version, ogrok version).

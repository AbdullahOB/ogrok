# ogrok

Self-hosted tunnel server. Expose local services to the internet over HTTPS.

[![Release](https://img.shields.io/github/v/release/AbdullahOB/ogrok?style=flat-square)](https://github.com/AbdullahOB/ogrok/releases)
[![License](https://img.shields.io/github/license/AbdullahOB/ogrok?style=flat-square)](LICENSE)
[![Build](https://img.shields.io/github/actions/workflow/status/AbdullahOB/ogrok/release.yml?style=flat-square)](https://github.com/AbdullahOB/ogrok/actions)

## Quick start

**1. Install the client**

```bash
curl -fsSL https://raw.githubusercontent.com/AbdullahOB/ogrok/main/install.sh | bash
```

**2. Save your token**

Get a token from whoever runs the ogrok server (or generate one if you're
self-hosting), then save it so the client can authenticate:

```bash
mkdir -p ~/.ogrok
echo 'token: YOUR_TOKEN' > ~/.ogrok/config.yaml
```

**3. Expose a local port**

```bash
ogrok http 3000
```

That's it. You'll get a public URL like `https://a1b2c3d4.ogrok.dev` that
forwards traffic to `localhost:3000`.

## More examples

```bash
# pick a specific subdomain
ogrok http 3000 --subdomain myapp
# -> https://myapp.ogrok.dev -> localhost:3000

# use a custom domain (requires a CNAME pointing to the server)
ogrok http 3000 --domain dev.mysite.com

# pass the token inline instead of using a config file
ogrok http 8080 --token YOUR_TOKEN

# connect to a different server
ogrok http 3000 --server tunnel.mycompany.com
```

### Token setup

The client looks for a token in this order:

1. `--token` flag
2. `OGROK_TOKEN` environment variable
3. `~/.ogrok/config.yaml`

Config file format:

```yaml
token: YOUR_TOKEN
server: ogrok.dev   # optional, this is the default
```

### All flags

```
ogrok http <port> [flags]

  --subdomain <name>    Request a specific subdomain
  --domain <domain>     Use a custom domain
  --server <address>    Server address (default: ogrok.dev)
  --token <token>       Auth token
```

## Self-hosting

If you want to run your own ogrok server instead of using `ogrok.dev`:

**1. Install the server binary**

```bash
curl -fsSL https://raw.githubusercontent.com/AbdullahOB/ogrok/main/install.sh | bash -s -- --server
```

**2. Set up DNS**

Point your domain and a wildcard subdomain to your server's IP:

```
yourdomain.com      A    YOUR_SERVER_IP
*.yourdomain.com    A    YOUR_SERVER_IP
```

**3. Create a config file**

```yaml
server:
  http_port: 80
  https_port: 443
  base_domain: "yourdomain.com"
  admin_port: 8080

auth:
  tokens:
    - "generate-a-random-token-here"

tls:
  autocert: true
  cert_cache_dir: "/var/lib/ogrok/certs"
```

**4. Start the server**

```bash
ogrok-server -config server.yaml
```

**5. Give your token to clients**

Clients connect using:

```bash
ogrok http 3000 --server yourdomain.com --token THE_TOKEN_FROM_YOUR_CONFIG
```

See [deploy/](deploy/) for systemd and Docker deployment options.

## How it works

```
Client                    Server                    Internet
+-----------+  WebSocket  +--------------+  HTTPS  +---------+
| ogrok     |<----------->| ogrok-server |<--------| Browser |
| localhost |             | :80/:443     |         |         |
| :3000     |             |              |         +---------+
+-----------+             +--------------+
```

The client opens a WebSocket control channel to the server. When the server
receives an HTTP request for a tunnel's hostname, it serializes the request
and forwards it over the WebSocket. The client proxies it to the local port
and sends the response back.

- Automatic TLS via Let's Encrypt
- Token-based auth
- Rate limiting (100 req/s per tunnel)
- Auto-reconnect with exponential backoff

## Build from source

```bash
git clone https://github.com/AbdullahOB/ogrok.git
cd ogrok
make build-all
# binaries in ./bin/
```

## License

MIT

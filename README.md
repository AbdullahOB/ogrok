# ogrok

Self-hosted tunnel server. Expose local services to the internet over HTTPS.

[![Release](https://img.shields.io/github/v/release/AbdullahOB/ogrok?style=flat-square)](https://github.com/AbdullahOB/ogrok/releases)
[![License](https://img.shields.io/github/license/AbdullahOB/ogrok?style=flat-square)](LICENSE)
[![Build](https://img.shields.io/github/actions/workflow/status/AbdullahOB/ogrok/release.yml?style=flat-square)](https://github.com/AbdullahOB/ogrok/actions)

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/AbdullahOB/ogrok/main/install.sh | bash
```

Or build from source:

```bash
git clone https://github.com/AbdullahOB/ogrok.git
cd ogrok
make build-all
```

## Usage

```bash
# random subdomain
ogrok http 3000
# -> https://a1b2c3d4.ogrok.dev -> localhost:3000

# named subdomain
ogrok http 3000 --subdomain myapp
# -> https://myapp.ogrok.dev -> localhost:3000

# custom domain (requires CNAME to your server)
ogrok http 3000 --domain dev.mysite.com
```

### Authentication

Token can be provided via flag, env var, or config file:

```bash
ogrok http 3000 --token your-token
# or
export OGROK_TOKEN=your-token
# or
echo 'token: your-token' > ~/.ogrok/config.yaml
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

1. Set up DNS: point `yourdomain.com` and `*.yourdomain.com` to your server IP.

2. Create a config:

```yaml
server:
  http_port: 80
  https_port: 443
  base_domain: "yourdomain.com"
  admin_port: 8080

auth:
  tokens:
    - "your-secret-token"

tls:
  autocert: true
  cert_cache_dir: "/var/lib/ogrok/certs"
```

3. Run the server:

```bash
ogrok-server -config server.yaml
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

## License

MIT

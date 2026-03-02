# ogrok

Self-hosted tunnel server. Expose local services to the internet over HTTPS.

[![Release](https://img.shields.io/github/v/release/AbdullahOB/ogrok?style=flat-square)](https://github.com/AbdullahOB/ogrok/releases)
[![License](https://img.shields.io/github/license/AbdullahOB/ogrok?style=flat-square)](LICENSE)
[![Build](https://img.shields.io/github/actions/workflow/status/AbdullahOB/ogrok/release.yml?style=flat-square)](https://github.com/AbdullahOB/ogrok/actions)

## Quick start (client)

**1. Install**

```bash
curl -fsSL https://raw.githubusercontent.com/AbdullahOB/ogrok/main/install.sh | bash
```

**2. Save your token**

Get a token from whoever runs the ogrok server (or generate one if you're
self-hosting), then save it:

```bash
mkdir -p ~/.ogrok
echo 'token: YOUR_TOKEN' > ~/.ogrok/config.yaml
```

**3. Expose a local port**

```bash
ogrok http 3000
```

You'll get a public URL like `https://a1b2c3d4.ogrok.dev` that
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

This section walks through setting up your own ogrok server from scratch.

### 1. Install the server binary

Note the `--server` flag -- without it you get the client.

```bash
curl -fsSL https://raw.githubusercontent.com/AbdullahOB/ogrok/main/install.sh | bash -s -- --server
```

### 2. Set up DNS

Point your domain and a wildcard subdomain to your server's IP:

```
yourdomain.com      A    YOUR_SERVER_IP
*.yourdomain.com    A    YOUR_SERVER_IP
```

### 3. Create a config file

```bash
mkdir -p /etc/ogrok
nano /etc/ogrok/server.yaml
```

If ogrok-server is the only thing on the machine (listening on 80/443 directly):

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

If you're running behind a reverse proxy like nginx (which already occupies
80/443), use different ports and let nginx forward traffic:

```yaml
server:
  http_port: 8081
  https_port: 8443
  base_domain: "yourdomain.com"
  admin_port: 8080

auth:
  tokens:
    - "generate-a-random-token-here"

tls:
  autocert: true
  cert_cache_dir: "/var/lib/ogrok/certs"
```

You can generate a random token with `openssl rand -hex 32`.

### 4. Create required directories

```bash
mkdir -p /var/lib/ogrok/certs
```

### 5. Start the server

```bash
ogrok-server -config /etc/ogrok/server.yaml
```

This runs in the foreground. You should see log output like:

```
2026/03/02 12:00:00 Starting ogrok server...
2026/03/02 12:00:00 HTTP server on :80
2026/03/02 12:00:00 HTTPS server on :443
2026/03/02 12:00:00 Admin server on :8080
```

Press Ctrl+C to stop it. Once you've confirmed it works, set it up as a
systemd service so it runs in the background and starts on boot:

```bash
sudo cp deploy/ogrok-server.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now ogrok-server
```

See [deploy/](deploy/) for the full systemd service file and Docker options.

### 6. Test from a client machine

```bash
ogrok http 3000 --server yourdomain.com --token THE_TOKEN_FROM_YOUR_CONFIG
```

### Running behind nginx

If nginx handles TLS on 80/443 and forwards to ogrok on 8081, your nginx
config needs to proxy both tunnel traffic and the WebSocket control channel:

```nginx
# WebSocket + tunnel traffic on the base domain
server {
    server_name yourdomain.com;
    listen 443 ssl;
    # ... your ssl certs ...

    # route tunnel control channel to ogrok
    location /_tunnel/ {
        proxy_pass http://127.0.0.1:8081;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 86400s;
        proxy_send_timeout 86400s;
    }

    # everything else (landing page, etc)
    location / {
        # your landing page or default handler
    }
}

# wildcard subdomains -> ogrok
server {
    server_name *.yourdomain.com;
    listen 443 ssl;
    # ... wildcard ssl cert ...

    location / {
        proxy_pass http://127.0.0.1:8081;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_read_timeout 86400s;
        proxy_send_timeout 86400s;
    }
}
```

The key details: `proxy_read_timeout 86400s` keeps the WebSocket alive, and
the `Upgrade`/`Connection` headers are required for WebSocket proxying.

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

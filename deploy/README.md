# Deployment

## Systemd (production)

```bash
make build-all
sudo ./deploy/setup.sh
```

The setup script creates a dedicated user, installs the binary, generates an
auth token, configures the firewall, and starts the service.

Manage with:

```bash
sudo systemctl status ogrok-server
sudo journalctl -u ogrok-server -f
sudo systemctl restart ogrok-server
```

## Docker

```bash
cd deploy/
./docker-deploy.sh
```

Or manually:

```bash
docker-compose up --build -d
docker-compose logs -f
```

## Configuration

Edit `configs/server.yaml` (systemd: `/opt/ogrok/configs/server.yaml`):

```yaml
server:
  base_domain: "tunnel.yourdomain.com"

auth:
  tokens:
    - "your-token"

tls:
  autocert: true
  cert_cache_dir: "/var/lib/ogrok/certs"
```

See [dns-setup.md](dns-setup.md) for DNS configuration.

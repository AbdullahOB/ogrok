# DNS Setup

## Required records

Point your base domain and a wildcard subdomain to your server's IP:

```
tunnel.yourdomain.com      A    YOUR_SERVER_IP
*.tunnel.yourdomain.com    A    YOUR_SERVER_IP
```

This lets ogrok create subdomains like `myapp.tunnel.yourdomain.com` on the fly.

## Custom domains

Clients who want to use their own domain (e.g. `dev.example.com`) need to create
a CNAME pointing to your tunnel base domain:

```
dev.example.com    CNAME    tunnel.yourdomain.com
```

## Verification

```bash
dig tunnel.yourdomain.com A
dig test.tunnel.yourdomain.com A
curl -I https://tunnel.yourdomain.com
```

After DNS is set up, update `base_domain` in your server config and restart.

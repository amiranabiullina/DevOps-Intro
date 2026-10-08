# Cloudflare quick tunnel (Bonus)

```bash
brew install cloudflared hyperfine

docker run -d --name qn-tunnel --platform linux/amd64 -p 8090:8080 \
  -e DATA_PATH=/data/notes.json -e SEED_PATH=/app/seed.json \
  ghcr.io/amiranabiullina/devops-intro/quicknotes:v0.1.0

cloudflared tunnel --protocol http2 --url http://localhost:8090

hyperfine --runs 50 --warmup 3 -N "curl -s -o /dev/null https://<random>.trycloudflare.com/health"
```

- Default QUIC was blocked by the VPN (Error 1033); `--protocol http2` works.
- The URL is ephemeral and changes on every `cloudflared` restart.
- `/` returns QuickNotes' `404 page not found`; use `/health`, `/notes`.

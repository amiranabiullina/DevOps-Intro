# Teardown

| Resource | Command |
|----------|---------|
| Codespace | `gh codespace delete -c bug-free-spork-7v4x45jp597x3w5wv` |
| Cloudflare tunnel | `Ctrl+C` in the `cloudflared` terminal |
| Local container | `docker rm -f qn-tunnel` |
| ghcr.io image | kept as the release; delete via GitHub → Packages → Package settings |
| Render | not created (card verification) |

Total cost: $0.

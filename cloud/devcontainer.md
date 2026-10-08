# Codespaces (Option B)

Render asked for card verification, so Task 2 uses GitHub Codespaces.

## devcontainer.json

```json
{
  "name": "QuickNotes (Lab 10)",
  "image": "mcr.microsoft.com/devcontainers/base:ubuntu-24.04",
  "features": {
    "ghcr.io/devcontainers/features/docker-in-docker:2": {}
  },
  "forwardPorts": [8080],
  "portsAttributes": {
    "8080": { "label": "QuickNotes", "onAutoForward": "silent" }
  },
  "postStartCommand": "bash -c 'until docker info >/dev/null 2>&1; do sleep 1; done; docker rm -f quicknotes >/dev/null 2>&1; docker run -d --name quicknotes -p 8080:8080 -e ADDR=:8080 -e DATA_PATH=/data/notes.json -e SEED_PATH=/app/seed.json -v quicknotes-data:/data ghcr.io/amiranabiullina/devops-intro/quicknotes:v0.1.0'"
}
```

## Commands

```bash
gh auth refresh -h github.com -s codespace
gh codespace create -R amiranabiullina/DevOps-Intro -b feature/lab10 -m basicLinux32gb
CS=bug-free-spork-7v4x45jp597x3w5wv
gh codespace ports visibility 8080:public -c $CS
gh codespace ports -c $CS
curl -v https://$CS-8080.app.github.dev/health
```

Port visibility cannot be set in `devcontainer.json` and resets to private after a restart, so the `visibility` command has to be run again after each start.

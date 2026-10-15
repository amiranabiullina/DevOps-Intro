# Lab 11 — Bonus: Reproducible Builds of QuickNotes with Nix


## Task 1 — Reproducible Go Build via Nix Flake

### flake.nix

[`flake.nix`](../flake.nix), [`flake.lock`](../flake.lock) (nixpkgs `nixos-25.11` locked to `b6018f87da91d19d0ab4cf979885689b469cdd41`, Go 1.25.10 ≥ 1.24 required by `app/go.mod`).

```nix
{
  description = "QuickNotes — reproducible build (Lab 11)";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";

  outputs = { self, nixpkgs }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAllSystems (pkgs: rec {
        quicknotes = pkgs.buildGoModule {
          pname = "quicknotes";
          version = "0.1.0";
          src = ./app;

          # app/go.mod has no requirements (no go.sum), so there is nothing to vendor:
          # nixpkgs rejects any hash here with "vendor folder is empty, please set 'vendorHash = null;'".
          vendorHash = null;

          env.CGO_ENABLED = 0;
          ldflags = [ "-s" "-w" ];
        };

        default = quicknotes;

        docker = { ... };  # see Task 2
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = [ pkgs.go pkgs.gopls pkgs.golangci-lint ];
        };
      });
    };
}
```

**vendorHash.** The first build with `lib.fakeHash` did not give a `got:` hash. It failed like this instead, because QuickNotes uses only the standard library:

```
quicknotes> go: no dependencies to vendor
quicknotes> vendor folder is empty, please set 'vendorHash = null;' in your expression
error: Cannot build '/nix/store/xha77ccj1qrv058azcdpxw6ifkp55v9n-quicknotes-0.1.0-go-modules.drv'.
```

So `null` is the pinned value here. It says the module has no dependencies, and Nix enforces it: if a dependency is added, the sandboxed build (no network) fails until a real hash is set.

**Why `buildGoModule`:** it ships with nixpkgs, so I need no extra flake input (`gomod2nix`) and no generated `gomod2nix.toml`. `buildGoApplication` is better when a project has many dependencies (see d), but QuickNotes has none.

### `nix build .#quicknotes` (environment A)

```
building '/nix/store/j1n9nh8vx9g8gh71k4i41jfqfnqi6in9-quicknotes-0.1.0.drv'...
quicknotes> Running phase: unpackPhase
quicknotes> unpacking source archive /nix/store/apz8fpz9xx3rh927h3jwdnfp1wa9wvf1-app
quicknotes> Running phase: buildPhase
quicknotes> Building subPackage .
quicknotes> Running phase: checkPhase
quicknotes> ok          quicknotes      0.003s
quicknotes> Running phase: installPhase
quicknotes> Running phase: fixupPhase
quicknotes> patchelf: cannot find section '.dynamic'. The input file is most likely statically linked
quicknotes> stripping (with command strip and flags -S -p) in  /nix/store/y3x2g7yww2dia7p9gzpa0ib2dv0vnpvh-quicknotes-0.1.0/bin
```

`patchelf: cannot find section '.dynamic'` confirms that `CGO_ENABLED = 0` produced a static binary.

### Store hashes from two environments

```
# environment A
$ readlink result
/nix/store/y3x2g7yww2dia7p9gzpa0ib2dv0vnpvh-quicknotes-0.1.0
$ nix-store --query --hash $(readlink result)
sha256:0qd4klg5q3hn2malql089wdq1cm39ndclxbj10689vzdqwg82f62

# environment B (fresh container, empty store)
$ git clone -b feature/lab11 https://github.com/amiranabiullina/DevOps-Intro.git qn-fresh && cd qn-fresh
$ nix build .#quicknotes
building '/nix/store/j1n9nh8vx9g8gh71k4i41jfqfnqi6in9-quicknotes-0.1.0.drv'...
$ readlink result
/nix/store/y3x2g7yww2dia7p9gzpa0ib2dv0vnpvh-quicknotes-0.1.0
$ nix-store --query --hash $(readlink result)
sha256:0qd4klg5q3hn2malql089wdq1cm39ndclxbj10689vzdqwg82f62
```

✅ Identical. `nix build .#quicknotes --rebuild` (Nix rebuilds and compares bit-for-bit) also exited with 0.

### It runs

```
$ cp app/seed.json . && ./result/bin/quicknotes &
2026/10/15 10:33:40 quicknotes listening on :8080 (notes loaded: 4)
$ curl -i http://localhost:8080/health
HTTP/1.1 200 OK
Content-Type: application/json
Content-Length: 26

{"notes":4,"status":"ok"}
```

### Design questions

#### a) Why does `go build` not produce bit-identical outputs on two machines?

- **Absolute paths.** Without `-trimpath`, the binary embeds the build directory and `GOPATH`/module cache paths, and those differ per machine and user.
- **VCS stamping.** `-buildvcs` (the default inside a git checkout) embeds the commit, commit time and a `vcs.modified` flag, so a dirty tree or a different checkout changes the binary.
- **Toolchain and environment.** A different Go version (or a `GOTOOLCHAIN` auto-download), `GOAMD64`/`GOARM`, `GOFLAGS`, and with CGO the host C compiler and libc all change the output.
- **Dependency resolution.** Without `go.sum`/vendoring, or with a different `GOPROXY`, a different module version can be resolved.
- **Build IDs.** The Go build ID is a hash of the inputs above, so any of these differences also changes the build ID in the ELF.

Nix removes all of these: fixed toolchain from `flake.lock`, sandbox at `/build`, no `.git` in `src`, no network, fixed environment.

#### b) What is `vendorHash` a SHA over? What does `vendorHash = null` do?

`buildGoModule` first runs a separate fixed-output derivation (`quicknotes-0.1.0-go-modules`). It runs `go mod vendor` with network access, and `vendorHash` is the SRI SHA-256 of the **NAR serialisation of the resulting `vendor/` directory**: all dependency sources plus `vendor/modules.txt`. Network access is allowed only because the output is checked against that hash, so the dependency tree is pinned byte for byte.

With `vendorHash = null`, that derivation is skipped. Go builds with `-mod=vendor` from a `vendor/` already in `src`, or with no dependencies at all. QuickNotes is the second case. If a project has dependencies but no committed `vendor/`, `null` makes the build fail, because the sandbox has no network.

#### c) Why is `flake.lock` the most important file? What if it is deleted?

`flake.nix` only says `nixos-25.11`, which is a moving branch. `flake.lock` pins it to one commit (`b6018f87…`) plus its `narHash`. That one revision fixes the whole build closure: Go, stdenv, coreutils, patchelf, tzdata, the `buildGoModule` and `dockerTools` code itself. Every store hash is computed from those inputs.

If it is deleted before the second build, Nix re-resolves `nixos-25.11` to whatever the branch tip is *now*, then writes a new lock. Once the channel has moved, Go, stdenv and the other inputs differ, so the derivation and output hashes differ and the proof is gone. Same `flake.nix`, different build.

#### d) `buildGoModule` vs `buildGoApplication`

- **`buildGoModule`** (nixpkgs) builds all dependencies as **one** fixed-output vendor derivation guarded by a single `vendorHash`. It is simple, but any change to `go.mod` invalidates the whole hash, and the hash has to be refreshed by hand (the "fail, copy `got:`" loop).
- **`buildGoApplication`** (from `gomod2nix`) reads a generated `gomod2nix.toml` with one hash **per module**. Each module is its own derivation, so you get granular caching and sharing between projects and no single opaque hash. The cost is an extra flake input, a generated file to keep in sync, and a tool to run.

For QuickNotes I picked `buildGoModule`: there are zero dependencies, so per-module granularity gives nothing, and staying with plain nixpkgs keeps the flake to one input.

## Task 2 — Deterministic OCI Image

### flake.nix (docker output)

```nix
        # Built by Nix alone (no Docker daemon). `created` defaults to the epoch, so the tarball is deterministic.
        docker = pkgs.dockerTools.buildImage {
          name = "quicknotes";
          tag = quicknotes.version;
          copyToRoot = [ quicknotes ];
          extraCommands = ''
            mkdir -p app data etc
            cp ${./app/seed.json} app/seed.json
            echo 'nonroot:x:65532:65532:nonroot:/home/nonroot:/sbin/nologin' > etc/passwd
            echo 'nonroot:x:65532:' > etc/group
            chmod 1777 data
          '';
          config = {
            Entrypoint = [ "/bin/quicknotes" ];
            ExposedPorts = { "8080/tcp" = { }; };
            User = "65532:65532";
            WorkingDir = "/app";
            Env = [ "DATA_PATH=/data/notes.json" "SEED_PATH=/app/seed.json" ];
          };
        };
```

- Exec-form `Entrypoint`, `8080/tcp` exposed.
- Runs as `nonroot` (65532, same UID as distroless `:nonroot` in Lab 6). `/etc/passwd` and `/etc/group` contain only that user.
- `extraCommands` runs as the unprivileged build user, so it cannot `chown` the data dir. I made `/data` sticky and world-writable (`1777`) so 65532 can write `notes.json` without `runAsRoot`, which would need a VM.
- No Docker daemon is involved: the layer tarball and config JSON are produced by Nix derivations.

### Loadable and running

```
$ docker load < result
Loaded image: quicknotes:0.1.0
$ docker run -d --rm -p 18080:8080 quicknotes:0.1.0
$ curl -s http://localhost:18080/health
{"notes":4,"status":"ok"}
$ docker inspect quicknotes:0.1.0 --format 'User={{.Config.User}} Entrypoint={{.Config.Entrypoint}} Ports={{.Config.ExposedPorts}} Created={{.Created}}'
User=65532:65532 Entrypoint=[/bin/quicknotes] Ports=map[8080/tcp:{}] Created=1970-01-01T00:00:01Z
$ docker logs <id>
quicknotes listening on :8080 (notes loaded: 4)
```

### Digests from two environments

```
# environment A
$ nix build .#docker
/nix/store/yii491gy985x1walmqv48x0gby9ngmjw-docker-image-quicknotes.tar.gz
$ sha256sum result
a4abc9fa209dd3fe43d6728ebd0f35b355c9839af215157bb2c2cadda9956900  result

# environment B (fresh container, fresh clone)
$ nix build .#docker
building '/nix/store/1jwdxm9dsk6qx355g0mdvz153n67ipf4-docker-image-quicknotes.tar.gz.drv'...
/nix/store/yii491gy985x1walmqv48x0gby9ngmjw-docker-image-quicknotes.tar.gz
$ sha256sum result
a4abc9fa209dd3fe43d6728ebd0f35b355c9839af215157bb2c2cadda9956900  result
```

✅ Identical (`--rebuild` also passes). The CI digest in the Bonus task is different because CI builds for `x86_64-linux`, not `aarch64-linux`.

### Lab 6 Dockerfile, built twice with `--no-cache`

The Dockerfile is from `feature/lab6` (`golang:1.26.9-alpine` builder, `-trimpath -ldflags="-s -w"`, `distroless/static:nonroot`).

```
$ docker build --no-cache -t qn-lab6:run1 ./app
$ docker build --no-cache -t qn-lab6:run2 ./app
$ docker images --no-trunc qn-lab6
REPOSITORY   TAG       IMAGE ID                                                                  SIZE
qn-lab6      run2      sha256:99c8e6252ac8c21a2c1fc8ad21b841daf6824c5f8de02cc855cb34fcb70cad6b   14.5MB
qn-lab6      run1      sha256:cc4ac0178a04acb43f03c5a2fe404b36669d40e60a461f92f8bad8e1406f9f48   14.5MB
```

❌ The digests differ. `docker inspect` shows that the first 14 layers (the distroless base) are identical, and the **last 3 layers** (the `COPY --from=builder` of the binary, `seed.json` and `/data`) differ:

```
run1 Created=2026-10-15T10:35:30.789687801Z  layers 1-14 e842bc95… … 7ea76385…  |  15-17 af4348ce… 33335015… 4df441f8…
run2 Created=2026-10-15T10:35:35.929593845Z  layers 1-14 e842bc95… … 7ea76385…  |  15-17 1e1e24f1… 8ee6c9c1… d9e919dc…
```

### Image size

| | Nix (`dockerTools.buildImage`) | Lab 6 (Dockerfile) |
|---|---:|---:|
| Compressed (Nix `.tar.gz` / `docker save \| gzip -n`) | **2.92 MB** | 3.15 MB |
| Content size (`docker images`) | 9.19 MB¹ | 3.16 MB |
| Disk usage (`docker images`) | 20.8 MB | 14.5 MB |
| Layers | 1 | 17 |
| Contents | binary + `tzdata`, `iana-etc`, `mailcap` (Go's runtime closure in nixpkgs) + `seed.json` | distroless static (ca-certs, tzdata, passwd) + binary + `seed.json` |

¹ `docker load` of a docker-archive stores the layer uncompressed, so Docker reports the 9.19 MB raw `layer.tar` as content size. Lab 6 layers are counted gzip-compressed. Compressed to compressed, the Nix image is slightly smaller.

### Design questions

#### e) What does `docker build` do that introduces non-determinism?

- **Timestamps everywhere.** The image config `created` and every `history[].created` hold the build time, so the config digest and therefore the image ID always change. File mtimes inside layers come from `COPY` and `RUN` at build time. This is exactly what I observed: the three `COPY --from=builder` layers changed, because the builder stage produced `/out/quicknotes` and `/out/data` with fresh mtimes.
- **Mutable inputs.** `FROM golang:1.26.9-alpine` and `distroless/static:nonroot` are tags, not digests, and can point to new images tomorrow. `RUN apk add` or `go mod download` fetch from the network at build time.
- **Environment leaks into `RUN`.** Host time, hostname, random IDs and build-tool metadata (BuildKit provenance and attestations) end up in the output.
- **Tar details.** Layer tar ordering, uid/gid and xattrs depend on the builder and the storage driver.

#### f) What can a security auditor prove with a reproducible image that a signed-but-non-reproducible one cannot?

A signature (cosign or sigstore) proves **who** produced the artifact: "this digest came out of our CI identity". It says nothing about **what is inside**. If the build pipeline is compromised (SolarWinds, xz-utils style), the malicious image is signed just as validly.

With a reproducible image the auditor can take the reviewed Git SHA plus `flake.lock`, rebuild on their own machine, and get the **same SHA-256**. That proves the shipped bits are exactly a function of the reviewed source and the pinned toolchain, with nothing injected between source and artifact. It does not require trusting our CI at all. With a non-reproducible image, a mismatch between the auditor's rebuild and the shipped image is expected, so a backdoor cannot be told apart from normal noise.

#### g) What's the trade-off of Nix? Why is `docker build` still the default?

- **Learning curve.** Nix is its own lazy functional language, with unfamiliar concepts (derivations, closures, fixed-output derivations) and error messages that are often cryptic.
- **Pinning chores.** `vendorHash` (and similar hashes for npm, cargo and so on) must be updated on every dependency change, and the fail-then-paste loop is friction.
- **Cost of the store.** The first build downloads a large closure (hundreds of MB here for perl, coreutils, Go and the rest), the store keeps growing until `nix store gc`, and you need a binary cache (Cachix or Attic) to be fast in CI.
- **Platform friction.** Building a Linux image on macOS needs a Linux builder (I used a container). Installing Nix needs root, a daemon, and on WSL2 some fiddling.
- **Ecosystem.** Dockerfiles are what every tutorial, base image, scanner, IDE and teammate already understands. Docker's layer cache is "good enough" for daily work, and most teams value familiarity and speed over bit-for-bit identical builds until an audit or supply-chain incident makes it matter.

## Bonus Task — CI-Verified Reproducibility

### Workflow

[`.github/workflows/nix-repro.yml`](../.github/workflows/nix-repro.yml): `build-a` and `build-b` run in parallel on separate fresh `ubuntu-24.04` runners, and `compare` fails if the digests differ. Actions are pinned by 40-char SHA.

```yaml
name: Nix reproducibility

on:
  push:
    branches: ["**"]
  pull_request:

permissions:
  contents: read

jobs:
  build-a:
    name: nix build .#docker (runner A)
    runs-on: ubuntu-24.04
    outputs:
      digest: ${{ steps.digest.outputs.digest }}
    steps:
      - uses: actions/checkout@08eba0b27e820071cde6df949e0beb9ba4906955 # v4.3.0
      - uses: DeterminateSystems/nix-installer-action@3138316df39ed29be04236d7ffc686fa525866aa # v23
      - run: nix build .#docker -L
      - id: digest
        run: |
          digest="$(sha256sum result | awk '{print $1}')"
          echo "digest=${digest}" >> "$GITHUB_OUTPUT"
          echo "runner A digest: ${digest}"

  build-b:
    name: nix build .#docker (runner B)
    runs-on: ubuntu-24.04
    outputs:
      digest: ${{ steps.digest.outputs.digest }}
    steps:
      - uses: actions/checkout@08eba0b27e820071cde6df949e0beb9ba4906955 # v4.3.0
      - uses: DeterminateSystems/nix-installer-action@3138316df39ed29be04236d7ffc686fa525866aa # v23
      - run: nix build .#docker -L
      - id: digest
        run: |
          digest="$(sha256sum result | awk '{print $1}')"
          echo "digest=${digest}" >> "$GITHUB_OUTPUT"
          echo "runner B digest: ${digest}"

  compare:
    name: compare digests
    runs-on: ubuntu-24.04
    needs: [build-a, build-b]
    steps:
      - env:
          A: ${{ needs.build-a.outputs.digest }}
          B: ${{ needs.build-b.outputs.digest }}
        run: |
          echo "A=${A}"
          echo "B=${B}"
          if [ -z "${A}" ] || [ "${A}" != "${B}" ]; then
            echo "::error::Image digests differ — build is not reproducible"
            exit 1
          fi
          echo "Digests match: ${A}"
```

### 🔴 Red run: deliberate divergence

Commit `12d3682` gave **only runner A** `SOURCE_DATE_EPOCH=1700000000` and a step that turns it into the image's `created` field:

```yaml
    env:
      SOURCE_DATE_EPOCH: "1700000000"   # deliberate divergence: only runner A gets a different timestamp
    ...
      - name: Inject SOURCE_DATE_EPOCH into image timestamp
        run: |
          created="$(date -u -d "@${SOURCE_DATE_EPOCH}" +%Y-%m-%dT%H:%M:%SZ)"
          sed -i "s|tag = quicknotes.version;|tag = quicknotes.version; created = \"${created}\";|" flake.nix
```

Run: https://github.com/amiranabiullina/DevOps-Intro/actions/runs/38721591767

```
nix build .#docker (runner A)  31:          tag = quicknotes.version; created = "2023-11-14T22:13:20Z";
nix build .#docker (runner A)  runner A digest: edf4312a0e601567edbeae21b67bf47c55f4261d84218ed08d754f84c83458d5
nix build .#docker (runner B)  runner B digest: 7b531f883612c73806dbae02263035395065376b53356f709af36def34781f19

X compare digests
  X Image digests differ — build is not reproducible
  X Process completed with exit code 1.
```

### 🟢 Green run: fix

Commit `4e767c2` removes the env var and the injection step from runner A.

Run: https://github.com/amiranabiullina/DevOps-Intro/actions/runs/38722423953

```
✓ nix build .#docker (runner A) in 52s
✓ nix build .#docker (runner B) in 53s
✓ compare digests in 2s

nix build .#docker (runner A)  runner A digest: 7b531f883612c73806dbae02263035395065376b53356f709af36def34781f19
nix build .#docker (runner B)  runner B digest: 7b531f883612c73806dbae02263035395065376b53356f709af36def34781f19
compare digests                A=7b531f883612c73806dbae02263035395065376b53356f709af36def34781f19
compare digests                B=7b531f883612c73806dbae02263035395065376b53356f709af36def34781f19
compare digests                Digests match: 7b531f883612c73806dbae02263035395065376b53356f709af36def34781f19
```

Runner B produced the same digest (`7b531f88…`) in both the red and the green run, and in the green run runner A matches it. The `x86_64-linux` image is reproducible across runners and across runs.

### Design questions

#### h) "Reproducible on my laptop" vs "reproducible in CI"

On a laptop the claim rests on my word and my machine's state. The store may already contain the outputs (Nix then just returns them without building), the tree can be dirty (Nix even warned `Git tree '/repo' is dirty` in environment A), and local config or substituters may differ. Nobody else can see the run.

In CI every run starts on fresh, ephemeral runners from the **committed** SHA, with an empty store for our derivations. The run is automatic on every push and leaves public logs that anyone can inspect and re-run. The check also gates merges, so a regression in reproducibility is caught at the commit that caused it, not months later during an audit. That independent, repeatable, recorded evidence is what an auditor can rely on.

#### i) Why two parallel jobs instead of one job building twice?

In one job, the second `nix build` is a no-op: the output path is already in the store, so Nix returns the same path and `sha256sum` hashes the same file twice. That proves nothing. Even with `--rebuild`, both builds share the same machine, kernel, CPU features, filesystem, hostname, time window and the same substituted dependencies. Non-determinism that comes from the environment (a different runner image, a different CPU, a timestamp across a minute or day boundary, a dependency fetched from a different cache) would not show up.

Two separate runners are two independent builders that share only the source and `flake.lock`, which is what "anyone can reproduce it" means.

#### j) Where would a timestamp leak in, and how does `dockerTools.buildImage` handle it?

- **Store files.** Every path added to `/nix/store` has its mtime normalised to `1` (1970-01-01 00:00:01), as seen in `ls -la result/bin`.
- **Go binary.** Go embeds no build time by itself. VCS stamping would add the commit time, but `src = ./app` contains no `.git`, so nothing leaks.
- **Inside builders.** stdenv exports `SOURCE_DATE_EPOCH=315532800` (1980-01-01), and tools that honour it use that value instead of "now".
- **`dockerTools.buildImage`:**
  - It packs the layer and the final image with `tar --hard-dereference --sort=name --mtime="@$SOURCE_DATE_EPOCH" --owner=0 --group=0` (`pkgs/build-support/docker/default.nix` in the pinned nixpkgs). That is why every entry in my `layer.tar` shows `Jan 1 1980` and `root root`.
  - The image config `created` defaults to `1970-01-01T00:00:01Z` (`docker inspect` showed exactly that).
  - The leak point is the `created` attribute. `created = "now"`, or any value that differs between builds, changes the config JSON and therefore the digest. That is what the red run did.
  - Setting `SOURCE_DATE_EPOCH` in the runner's shell alone would **not** have broken anything: the Nix sandbox does not inherit the caller's environment, and a pure flake cannot read env vars. So the red run had to inject it into the flake explicitly.

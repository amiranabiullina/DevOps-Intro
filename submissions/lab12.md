# Lab 12 — Bonus: WebAssembly Containers — A QuickNotes Endpoint on Spin


## Task 1 — WASM Endpoint with the Spin SDK

### Toolchain

| Tool | Version |
|---|---|
| Spin | **3.4.1** |
| TinyGo | **0.41.1** (go1.25.14, LLVM 20.1.1) |
| Spin Go SDK | `github.com/spinframework/spin-go-sdk/v2` **v2.2.1** |
| wasmtime (Bonus) | **49.0.3** |

I did not install Spin, TinyGo or wasmtime on the Mac. All three run in one Linux container with pinned versions; the repo is bind-mounted at `/repo`.

```dockerfile
FROM golang:1.25-bookworm

ARG SPIN=v3.4.1
ARG TINYGO=0.41.1
ARG WASMTIME=v49.0.3

RUN apt-get update && apt-get install -y --no-install-recommends xz-utils python3 && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL "https://github.com/spinframework/spin/releases/download/${SPIN}/spin-${SPIN}-linux-aarch64.tar.gz" | tar -xz -C /usr/local/bin spin \
 && curl -fsSLo /tmp/tinygo.deb "https://github.com/tinygo-org/tinygo/releases/download/v${TINYGO}/tinygo_${TINYGO}_arm64.deb" \
 && dpkg -i /tmp/tinygo.deb && rm /tmp/tinygo.deb \
 && curl -fsSL "https://github.com/bytecodealliance/wasmtime/releases/download/${WASMTIME}/wasmtime-${WASMTIME}-aarch64-linux.tar.xz" | tar -xJ -C /tmp \
 && mv /tmp/wasmtime-${WASMTIME}-aarch64-linux/wasmtime /usr/local/bin/ && rm -rf /tmp/wasmtime-*

WORKDIR /repo
```

```bash
docker build -t wasm-toolbox .
docker run -d --name wasm-lab -p 127.0.0.1:3001:3000 -v "$PWD:/repo" -w /repo wasm-toolbox sleep infinity
```

Why the host port is `3001` is explained in Task 2's test rig.

```
$ spin --version
spin 3.4.1 (3ab5404 2025-08-28)
$ tinygo version
tinygo version 0.41.1 linux/arm64 (using go version go1.25.14 and LLVM version 20.1.1)
$ wasmtime --version
wasmtime 49.0.3 (f96eabaa1 2026-10-14)
```

### Scaffold

```
$ spin templates install --git https://github.com/spinframework/spin --upgrade
$ spin templates list | grep http-go
| http-go             HTTP request handler using (Tiny)Go                  https://github.com/spinframework/spin |
$ mkdir -p wasm && cd wasm
$ spin new -t http-go moscow-time --accept-defaults
```

The scaffold created `moscow-time/{main.go,go.mod,go.sum,spin.toml,.gitignore}` with the SDK `github.com/spinframework/spin-go-sdk/v2 v2.2.1`. I moved these files one level up into `wasm/`, the layout the lab's submission list expects. My only changes: the handler, the route (`/...` → `/time`), and `authors`/`description` in `spin.toml`. The build command and `allowed_outbound_hosts` are exactly as generated.

### [`wasm/main.go`](../wasm/main.go)

```go
package main

import (
	"fmt"
	"net/http"
	"time"

	spinhttp "github.com/spinframework/spin-go-sdk/v2/http"
)

// TinyGo has no embedded tzdata, so time.LoadLocation("Europe/Moscow") fails; Moscow is a fixed UTC+3.
var moscow = time.FixedZone("MSK", 3*60*60)

func init() {
	spinhttp.Handle(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		now := time.Now().In(moscow)
		w.Header().Set("Content-Type", "application/json")
		// fmt instead of encoding/json: TinyGo's reflection support is limited.
		fmt.Fprintf(w, "{\"timezone\":%q,\"unix\":%d,\"iso\":%q,\"hour_minute\":%q}\n",
			"Europe/Moscow (UTC+3)", now.Unix(), now.Format(time.RFC3339), now.Format("15:04"))
	})
}
```

### [`wasm/spin.toml`](../wasm/spin.toml)

```toml
#:schema https://schemas.spinframework.dev/spin/manifest-v2/latest.json

spin_manifest_version = 2

[application]
name = "moscow-time"
version = "0.1.0"
authors = ["amiranabiullina <amiranabiullina@gmail.com>"]
description = "Moscow time as JSON at /time"

[[trigger.http]]
route = "/time"
component = "moscow-time"

[component.moscow-time]
source = "main.wasm"
allowed_outbound_hosts = []
[component.moscow-time.build]
command = "tinygo build -target=wasip1 -buildmode=c-shared -no-debug -o main.wasm ."
watch = ["**/*.go", "go.mod"]
```

### `spin build`

```
$ time spin build
Building component moscow-time with `tinygo build -target=wasip1 -buildmode=c-shared -no-debug -o main.wasm .`
go: downloading github.com/spinframework/spin-go-sdk/v2 v2.2.1
go: downloading github.com/julienschmidt/httprouter v1.3.0
Finished building all Spin components

real	0m14.773s
$ ls -l main.wasm
-rwxr-xr-x 1 root root 363558 Oct 15 11:44 main.wasm        # 355 KiB
```

### `spin up` + `curl`

```
$ spin up --listen 0.0.0.0:3000
Logging component stdio to ".spin/logs/"

Serving http://0.0.0.0:3000
Available Routes:
  moscow-time: http://0.0.0.0:3000/time
```

```
$ curl -s -i http://127.0.0.1:3000/time
HTTP/1.1 200 OK
content-type: application/json
content-length: 111
date: Thu, 15 Oct 2026 11:44:26 GMT

{"timezone":"Europe/Moscow (UTC+3)","unix":1792064666,"iso":"2026-10-15T14:44:26+03:00","hour_minute":"14:44"}

$ curl -s http://127.0.0.1:3000/time | python3 -m json.tool
{
    "timezone": "Europe/Moscow (UTC+3)",
    "unix": 1792064666,
    "iso": "2026-10-15T14:44:26+03:00",
    "hour_minute": "14:44"
}

$ curl -s -o /dev/null -w "%{http_code}\n" -X POST http://127.0.0.1:3000/time
405
$ curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:3000/other
404
```

`date -u` at the same moment printed `11:44:26 UTC`, so `14:44` is correct Moscow time (UTC+3). The 404 for `/other` comes from Spin's router; my code is never called for that path.

### Design questions

#### a) Browser WASM vs server WASM

`GOOS=js GOARCH=wasm go build` targets a **JavaScript host**. The module imports Go's JS glue (`wasm_exec.js`, `syscall/js`), and every "syscall" is a call into the browser or Node runtime: DOM, `fetch`, timers, the JS event loop.

`tinygo build -target=wasip1` targets **WASI**. The module imports only a small, standard set of host functions (`fd_write`, `clock_time_get`, `random_get`, `environ_get`, preopened file descriptors) and runs on any WASI runtime: wasmtime, Spin, WasmEdge.

- **Missing in the server target:** no JS interop, no DOM, no `fetch`, no event loop. `wasip1` has no sockets and no threads either: `net.Listen` is unavailable, so the HTTP server loop has to be provided by the host (Spin). The module only implements a handler.
- **Gained:** the module runs outside the browser on a standard ABI, with no JS engine and no glue file. Every capability is explicitly granted by the host (see c). TinyGo also makes it small: 355 KiB here, versus 3.3 MB for the small `GOOS=wasip1` test program I built with upstream Go for d.

#### b) Why `-buildmode=c-shared`?

It makes TinyGo emit a **reactor** module instead of a **command** module. I compared the exports of both builds with a small export-section parser:

| | with `-buildmode=c-shared` (as generated) | without it |
|---|---|---|
| Entry export | `_initialize` | `_start` |
| Handler export | `handle-http-request`, `spin_http_handle_http_request` | same |
| Needs `func main()` | no | yes |

- A **command** (`_start`) is "run `main` once, then exit". That is the CLI model of the Bonus task.
- A **reactor** (`_initialize`) is "initialise once (run Go's package `init()`, which is where `spinhttp.Handle` registers the handler), then stay loaded and let the host call exports repeatedly". The Spin host expects exactly that: it calls `_initialize`, then calls `handle-http-request` for every request.

What happened when I removed the flag (in a scratch copy, not in the repo):

1. With the scaffolded code, the build itself fails: `wasm-ld: error: lto.tmp: undefined symbol: main.main`. A command module must have a `main`, and the Spin template has none because the handler is registered in `init()`.
2. After adding an empty `func main() {}`, it builds (363 822 bytes, exporting `_start`), and with Spin 3.4.1 + TinyGo 0.41.1 `spin up` **still served HTTP 200**. I could not reproduce the "HTTP 500 with empty component logs" from the lab's pitfalls with these versions. The handler export is identical in both builds, and the response was correct, so the runtime and `init()` evidently get initialised when the export is first called. The reactor build is still the correct contract: it matches what Spin expects, does not need a dummy `main`, and does not rely on a toolchain detail.

#### c) `allowed_outbound_hosts = []` and capability-based security

A WASM module has **no ambient authority**. It cannot make syscalls. It can only call the functions the host chooses to link into it, and every resource has to be granted explicitly:

- outbound network: per host and scheme in `allowed_outbound_hosts`;
- files: only the directories listed in `files`;
- variables, key-value stores, databases: only when declared.

With `allowed_outbound_hosts = []`, any outbound HTTP or TCP call from the component is rejected by the host at call time. The guest has no other way to reach the network: there is no socket API to fall back on.

Docker's `--network none` is a coarse, kernel-level switch. The container gets a network namespace with only `lo`. Everything else stays: the full Linux syscall surface, the whole image filesystem, `/proc`, any mounted volumes or sockets. Two differences:

1. **Granularity.** Spin can allow `https://api.example.com` and nothing else. In Docker that needs an egress proxy or firewall rules outside the container.
2. **Mechanism.** Docker relies on the shared kernel enforcing namespaces correctly. In WASM, a capability that is not linked simply does not exist for the guest.

#### d) TinyGo stdlib gaps I hit

I tested this in a scratch program, built once with TinyGo (`-target=wasip1`) and once with upstream Go (`GOOS=wasip1`), and ran both under wasmtime:

```
# TinyGo 0.41.1
LoadLocation: loc=UTC err=unknown time zone Europe/Moscow
json.Marshal(map[string]any): {"iso":"2026-10-15T11:45:51Z","unix":1792064751} err=<nil>
json.Unmarshal into map[string]any: map[a:1 b:[true x]] err=<nil>
# TinyGo, with /usr/share/zoneinfo preopened into the sandbox
LoadLocation: loc=UTC err=unknown time zone Europe/Moscow
# upstream Go 1.25 GOOS=wasip1 (3.3 MB module)
LoadLocation: loc=UTC err=Bad file number
```

- **Time-zone data.** This one I actually hit. `time.LoadLocation("Europe/Moscow")` fails under TinyGo: no embedded tzdata, and it does not read zoneinfo from a preopened directory either. I use `time.FixedZone("MSK", 3*60*60)` instead. Moscow has had no DST since 2014, so a fixed `+03:00` is correct, and the output carries the proper `+03:00` offset in `iso`.
- **Reflection / `encoding/json`.** In TinyGo 0.41 the simple `map[string]any` round trip works. Earlier TinyGo versions broke here. It still pulls in a lot of reflection code: the test module was 490 KiB versus 355 KiB for the whole Spin component. TinyGo's `reflect` is also still incomplete for more complex types. So I build the JSON with `fmt` and `%q`, which needs no reflection at all.

## Task 2 — Perf Comparison vs the Lab 6 Container

The baseline is the Lab 6 image built from `feature/lab6`'s `app/Dockerfile` (`golang:1.26.9-alpine` builder → `distroless/static:nonroot`), tagged `qn-lab6:baseline`. Its endpoint is `/health`.

### Test rig

| | |
|---|---|
| Machine | Apple M4, 24 GB RAM |
| Host OS | macOS 26.6.2 |
| Container runtime | Docker Desktop 29.0.1 (Linux VM, `aarch64`) |
| Benchmarks | hyperfine 2.0.0 (host), 1.15.0 (inside the toolbox container) |

The Spin toolbox container from Task 1 and the Lab 6 container run in the same Docker Desktop VM, so both are measured on the same kernel and hardware. Inside the toolbox container Spin listens on `:3000`; on the host it is published as `127.0.0.1:3001`, because `:3000` is already taken by Grafana from my Lab 8 stack.

### How I measured

- **Warm latency.** From the macOS host, with hyperfine 2.0.0 in `-N` mode (no shell), `--warmup 5 --runs 50`. Spin was reached at `127.0.0.1:3001/time`, Docker at `127.0.0.1:18080/health`, so both pass through the same Docker Desktop port forwarder. p50 and p95 were computed from the exported per-run times. Each sample includes spawning `curl`, which costs ~4.5 ms on its own (`hyperfine -N 'curl --version'` = 4.5 ± 0.3 ms). That cost is the same for both, so the **difference** between the two is the real server-side cost.
- **Cold start.** A Python script stops the runtime, waits until the port is down, starts the runtime, and polls the URL every 2 ms until the first HTTP 200. 7 samples each:
  - Docker: `docker rm -f` → `docker run -d -p 127.0.0.1:18080:8080 qn-lab6:baseline`.
  - Spin, from the host: `pkill spin` → `docker exec -d … spin up`. This includes ~40 ms for the `docker exec` round trip itself (`docker exec wasm-lab true` = 0.04 s).
  - Spin, inside the toolbox container: `pkill spin` → `spin up`. This measures only the Spin runtime, without the Docker CLI.
- **Size.** `ls -l main.wasm` vs `docker images` and `docker save | gzip -n | wc -c`.

### Raw results

```
Benchmark 1: Lab 6 Docker /health (50 runs)
  Wall Time      5.7 ms  ±     0.4 ms       4.9 ms  …     6.7 ms
Benchmark 2: Spin /time (50 runs)
  Wall Time      7.7 ms  ±     1.0 ms       6.1 ms  …     9.4 ms           +34.7%  (1.3x slower)

Lab 6 Docker /health: n=50 p50=5.56 ms p95=6.54 ms mean=5.69 ms
Spin /time:           n=50 p50=7.72 ms p95=9.15 ms mean=7.67 ms
```

```
docker            samples_ms [181.5, 150.0, 121.1, 113.5, 120.3, 120.9, 115.6]  p50 120.9  min 113.5  max 181.5
spin (host)       samples_ms [141.6, 142.7, 154.6, 135.0, 130.4, 144.2, 127.1]  p50 141.6  min 127.1  max 154.6
spin-in-container samples_ms [56.9, 61.8, 69.7, 74.1, 64.7, 58.6, 67.1]         p50 64.7   min 56.9   max 74.1
```

```
$ ls -l wasm/main.wasm
363558 bytes
$ docker images qn-lab6:baseline
IMAGE              ID             DISK USAGE   CONTENT SIZE
qn-lab6:baseline   2a120e1431df       14.5MB         3.16MB
$ docker save qn-lab6:baseline | gzip -n | wc -c
3151727
```

### Table

| Dimension | Lab 6 Docker | Lab 12 WASM/Spin |
|---|---:|---:|
| Artifact size | 14.5 MB unpacked / 3.16 MB compressed (17 layers) | **0.36 MB** (`main.wasm`, 363 558 B) |
| Cold start (p50, 7 samples) | 120.9 ms (`docker run`) | 141.6 ms via `docker exec` (incl. ~40 ms CLI) / **64.7 ms** runtime only |
| Warm latency p50 (incl. ~4.5 ms `curl` spawn) | **5.56 ms** | 7.72 ms |
| Warm latency p95 | **6.54 ms** | 9.15 ms |

**What the numbers say:**

- **Size.** The WASM artifact is ~40× smaller than the unpacked image and ~9× smaller than the compressed one. It also contains no OS layer at all.
- **Cold start.** Without the Docker CLI hop, Spin is ready about 2× faster than the container (65 vs 121 ms). The container is already fast here: the image is cached and extracted, the binary is static, and distroless has nothing to initialise. On a fresh node, `docker pull` plus extraction would add seconds. For `main.wasm` it adds one 355 KiB file.
- **Warm latency.** The native Go server is faster per request: ~1 ms vs ~3 ms once the ~4.5 ms `curl` spawn is subtracted. Spin creates a fresh, isolated WASM instance for every request and runs TinyGo-compiled code through the Spin HTTP ABI. The Go server reuses one long-running process with native code.

### Design questions

#### e) What dominates each platform's cold start?

**Docker (≈120 ms).** The chain is `docker` CLI → dockerd → containerd → runc. It creates the namespaces and cgroups, mounts the overlayfs snapshot of the image, sets up the veth interface and the port-forward proxy (the published port), and only then execs the static binary. The Go runtime plus reading `seed.json` take a few milliseconds of that. Image download and extraction did not count here because the image was cached. On a cold node they dominate, at seconds for a pull.

**Spin (≈65 ms).** It starts the `spin` process (a large Rust binary), parses `spin.toml`, loads `main.wasm`, and turns the core module into a component with the WASI preview1 adapter. Then wasmtime/Cranelift **compiles the module to native code** and the HTTP listener binds. Compilation is the big part: in the Bonus, the same wasmtime spends ~20 ms more per run on a module half this size when its compile cache is off. After startup, creating an instance for a request takes well under a millisecond, which is where the "~1 ms" in Reading 12 comes from. It describes the per-instance cost, not the whole `spin up` process.

#### f) Where WASM is clearly better, and where Docker is still right

**WASM/Spin is clearly better for:**

- Short, stateless, bursty request handlers and scale-to-zero functions: edge or serverless handlers like this `/time`, webhooks, image or text transforms. A 355 KiB artifact and a fast cold start make "start on request, then drop" cheap.
- Running untrusted or multi-tenant code: plugins, user-defined functions, policy hooks.
- One portable artifact for every CPU: the same `main.wasm` runs on arm64 and x86_64, while Docker needs a per-arch image.

**Docker is still right for:**

- QuickNotes itself: a long-running, stateful server that writes `data/notes.json`, uses goroutines and the full Go stdlib.
- Anything that needs OS packages (`apt install`), cgo, raw sockets or threads, or mature drivers (databases, observability agents).
- Teams that rely on the existing ecosystem: Compose, Kubernetes, scanners, debuggers.

My own numbers show the other cost too: for warm per-request throughput, the native container won.

#### g) Multi-tenant safety: what concrete attack does WASM make harder?

**Container escape through the shared kernel.**

A container process makes real syscalls into the host kernel it shares with every other tenant. One kernel or runtime bug is enough to break out:

- Dirty Pipe (CVE-2022-0847): overwrite read-only files from inside a container.
- runc CVE-2019-5736: overwrite the host's `runc` binary via `/proc/self/exe`.

A WASM guest cannot issue syscalls at all. It can only call typed host imports, and its memory is a bounds-checked linear memory with no pointer into host memory. So the whole kernel attack surface is out of reach for tenant code.

The second concrete case is **SSRF / credential theft**. A malicious tenant handler tries to call the cloud metadata endpoint `http://169.254.169.254/` to steal instance credentials, or scans other tenants on the internal network. A container on the default bridge network can do that. A Spin component with `allowed_outbound_hosts = []` cannot even open the connection.

## Bonus Task — Two WASM Execution Models

### [`wasm-cli/main.go`](../wasm-cli/main.go)

```go
// CGI/WAGI-shaped WASI module: the request comes in as env vars, the response goes to stdout.
package main

import (
	"fmt"
	"os"
	"time"
)

// Same logic as the Spin component in wasm/: fixed UTC+3, JSON built with fmt.
var moscow = time.FixedZone("MSK", 3*60*60)

func main() {
	method, path := os.Getenv("REQUEST_METHOD"), os.Getenv("PATH_INFO")

	switch {
	case path != "/time":
		fmt.Print("Status: 404 Not Found\nContent-Type: text/plain\n\nnot found\n")
	case method != "GET":
		fmt.Print("Status: 405 Method Not Allowed\nAllow: GET\nContent-Type: text/plain\n\nmethod not allowed\n")
	default:
		now := time.Now().In(moscow)
		fmt.Print("Content-Type: application/json\n\n")
		fmt.Printf("{\"timezone\":%q,\"unix\":%d,\"iso\":%q,\"hour_minute\":%q}\n",
			"Europe/Moscow (UTC+3)", now.Unix(), now.Format(time.RFC3339), now.Format("15:04"))
	}
}
```

It writes a CGI-style response: headers, a blank line, then the body.

### Build + run

```
# Spin component (Task 1)
$ cd wasm && spin build
  → tinygo build -target=wasip1 -buildmode=c-shared -no-debug -o main.wasm .
$ spin up

# standalone WASI CLI module
$ cd wasm-cli && tinygo build -o main.wasm -target=wasi -no-debug ./main.go
$ ls -l main.wasm
-rwxr-xr-x 1 root root 196847 Oct 15 11:48 main.wasm

$ wasmtime run --env REQUEST_METHOD=GET --env PATH_INFO=/time main.wasm
Content-Type: application/json

{"timezone":"Europe/Moscow (UTC+3)","unix":1792064881,"iso":"2026-10-15T14:48:01+03:00","hour_minute":"14:48"}

$ wasmtime run --env REQUEST_METHOD=POST --env PATH_INFO=/time main.wasm
Status: 405 Method Not Allowed
Allow: GET
Content-Type: text/plain

method not allowed

$ wasmtime run --env REQUEST_METHOD=GET --env PATH_INFO=/x main.wasm
Status: 404 Not Found
Content-Type: text/plain

not found
```

### Size + cold-start comparison

Measured inside the toolbox container with hyperfine 1.15.0 (`-N --warmup 3 --runs 30`):

```
wasmtime run (default, compile cache on)          p50=25.0 ms  mean=25.9 ms  min=18.4 ms
wasmtime run -C cache=n (compile every time)      p50=45.9 ms  mean=48.9 ms  min=35.0 ms
curl http://127.0.0.1:3000/time (warm Spin)       p50=3.1 ms   mean=3.2 ms   min=2.7 ms
```

| | Spin component (`wasm/`) | WASI CLI module (`wasm-cli/`) |
|---|---:|---:|
| Module size | 363 558 B (355 KiB) | **196 847 B (192 KiB)** |
| Exports | `_initialize`, `handle-http-request` | `_start` |
| Cold start | 64.7 ms once (`spin up` → first 200) | 25.0 ms p50 **on every request** (45.9 ms without the compile cache) |
| Per request after startup | **3.1 ms** p50 (incl. `curl` spawn) | 25.0 ms (each request is a cold start) |

- **Size.** The CLI module is ~half the size because it has no Spin SDK, no `net/http` and no `httprouter`, only `os`, `fmt` and `time`.
- **Speed.** A single `wasmtime run` is faster than bringing up Spin (25 vs 65 ms), because there is no server, router or listener to set up. But it pays that price on **every** request: process start, module load, compile-cache lookup, instantiation, exit. Spin pays once, then serves each request from a precompiled module with a fresh instance, ~8× faster per request.

### Design questions

#### h) Why can't the Task 1 Spin component run under bare `wasmtime run`?

`wasmtime run` executes a **command**: it calls `_start` (or a component's `wasi:cli/run`) and exits when `main` returns. The Spin module exports no `_start`, only `_initialize` plus the Spin HTTP handler `handle-http-request`, which expects the host to pass a request record. What I saw:

```
$ wasmtime run wasm/main.wasm
(no output, exit 0: there is no _start to run, so no request is ever delivered)

$ wasmtime run --invoke handle-http-request wasm/main.wasm
Error: failed to run main module `main.wasm`
Caused by:
    not enough arguments for `handle-http-request`

$ wasmtime serve wasm/main.wasm
Error: The serve command currently requires a component
```

`wasmtime serve` would host a real `wasi:http/incoming-handler` **component**. TinyGo + `spin-go-sdk/v2` produce a **core module** with Spin's own HTTP export, and Spin itself wraps it into a component at load time. So with this toolchain the module only runs under Spin.

#### i) Spin uses wasmtime internally. What does Spin add?

- **The HTTP server loop.** A real listener, plus the host side of the Spin HTTP / wasi-http interfaces that turns a socket request into a call to the guest's handler.
- **The manifest and routing layer.** `spin.toml` maps `/time` to the component. The `404` for `/other` came from Spin, not from my code.
- **Instance management.** The module is compiled once at startup, then each request gets a fresh, isolated instance from a pooling allocator. That gives the 3 ms vs 25 ms gap.
- **Componentization.** Plain `wasip1` core modules are adapted into components.
- **Capability policy from the manifest.** `allowed_outbound_hosts`, `files`, variables, key-value stores and SQLite are granted per component.
- **Developer tooling.** `spin new` templates, `spin build`, `spin up`, component stdio logs in `.spin/logs/`, and `spin deploy`/registry push.

Bare wasmtime only provides the engine and WASI. Everything around it would be up to me.

#### j) When does each execution model fit?

- **Per-invocation `wasmtime run` (CGI-like).** Fits work that happens rarely or once per event, where a ~25 ms start does not matter and nothing should stay resident. Example: a nightly cron job that generates a report, or a sandboxed CI/plugin step (a formatter or a data-transform filter run once per file). Each run is fully isolated, and there is no server to keep alive or secure.
- **Spin's persistent wasi-http server.** Fits request/response APIs with steady or bursty traffic, where per-request latency matters. Example: this `/time` endpoint, or any QuickNotes-style read endpoint. Startup cost is paid once, every request still gets a fresh sandboxed instance, and routing and the outbound policy come from the manifest.

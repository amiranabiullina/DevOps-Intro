# Lab 4 — OS & Networking: Trace, Debug, and Read the Substrate

## Task 1 — Trace a Request End-to-End

### TCP Capture

QuickNotes was started locally:

```text
quicknotes listening on :8080
```

On macOS the loopback interface is `lo0`.

Capture command:

```bash
sudo tcpdump -i lo0 -nn -s 0 -A 'tcp port 8080' -w lab4-trace.pcap
```

A request was sent:

```http
POST /notes HTTP/1.1
```

Request body:

```json
{"title":"trace me","body":"in flight"}
```

The server returned:

```http
HTTP/1.1 201 Created
```

Response:

```json
{"id":7,"title":"trace me","body":"in flight"}
```

---

### Packet Analysis

The capture was decoded using:

```bash
sudo tcpdump -r lab4-trace.pcap -nn -A | tee lab4-trace.txt
```

The capture contains:

- TCP three-way handshake:
  - SYN
  - SYN/ACK
  - ACK

- HTTP request:

```http
POST /notes HTTP/1.1
```

- JSON request body:

```json
{"title":"trace me","body":"in flight"}
```

- HTTP response:

```http
HTTP/1.1 201 Created
```

- Connection termination using TCP FIN packets.

---

## Debugging Commands

### 1. Listening process

Linux command:

```bash
ss -tlnp | grep :8080
```

macOS equivalent:

```bash
lsof -i :8080
```

Output:

```text
quicknote 15373 TCP *:http-alt (LISTEN)
```

Decision:

QuickNotes is listening on port 8080.

---

### 2. Routing table

Linux command:

```bash
ip route show
```

macOS equivalent:

```bash
netstat -rn
```

Decision:

The routing table contains default routes and loopback routes. Local traffic uses the loopback interface.

---

### 3. Reachability

Command:

```bash
sudo /opt/homebrew/Cellar/mtr/0.96/sbin/mtr -rwc 5 localhost
```

Output:

```text
localhost 0.0% loss
```

Decision:

The host is reachable through the loopback interface.

---

### 4. DNS

Command:

```bash
dig +short example.com @1.1.1.1
```

Output:

```text
172.66.147.243
104.20.23.154
```

Decision:

DNS resolution works correctly.

---

### 5. Logs

Linux command:

```bash
journalctl --user -u quicknotes -n 20 || true
```

Result on macOS:

```text
zsh: command not found: journalctl
```

Decision:

macOS does not use systemd. QuickNotes was started manually with `go run .`, therefore logs were available in the application terminal.

---

## 502 Debug Reflection

If QuickNotes returned HTTP 502, I would first check whether the service is running and listening on the expected port. Then I would verify connectivity and inspect application logs. After that, I would check network configuration, routing, DNS, and proxy settings to identify where the failure occurs.

---

# Task 2 — Outside-In Debugging on a Broken Deploy

## Broken Instance

A second QuickNotes instance was started on the same port:

```bash
ADDR=:8080 go run .
```

The application failed with:

```text
listen tcp :8080: bind: address already in use
```

Root cause:

Another QuickNotes process was already listening on port 8080, so the second instance could not bind to the same address.

---

## Outside-In Debugging Chain

### 1. Is it running?

Command:

```bash
ps -ef | grep quicknotes
```

Result:

```text
quicknote 15373
```

Decision:

The QuickNotes process is running.

---

### 2. Is it listening?

Command:

```bash
lsof -i :8080
```

Output:

```text
quicknote 15373 TCP *:http-alt (LISTEN)
```

Decision:

The service is listening on port 8080.

---

### 3. Is it reachable?

Command:

```bash
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/health
```

Output:

```text
200
```

Decision:

The service responds successfully.

---

### 4. Firewall

Command:

```bash
sudo /usr/libexec/ApplicationFirewall/socketfilterfw --getglobalstate
```

Output:

```text
Firewall is disabled. (State = 0)
```

Decision:

Firewall is not blocking traffic.

---

### 5. DNS

Command:

```bash
dig +short localhost
```

Output:

```text
127.0.0.1
```

Decision:

localhost resolves correctly.

---

## Repair and Verification

The conflicting process was stopped:

```bash
kill 15373
```

QuickNotes was started again:

```bash
ADDR=:8080 go run .
```

Health check:

```bash
curl -s http://localhost:8080/health
```

Response:

```json
{"notes":7,"status":"ok"}
```

---

## Mini Postmortem

The failure was caused by multiple QuickNotes instances attempting to use the same port. The application itself was working correctly, but the operating system prevented a second process from binding to an already occupied address. This type of failure can be prevented by using service management tools, health checks, and deployment automation. The issue was resolved by identifying the process using port 8080, stopping the conflicting instance, and verifying the service health.

---

# Bonus — TLS Handshake

## HTTPS Layer

Caddy was configured as a TLS reverse proxy:

```text
localhost:8443 {
    reverse_proxy localhost:8080
}
```

Caddy successfully installed the local certificate and started.

---

## TLS Capture

Capture command:

```bash
sudo tcpdump -i lo0 -nn -s 0 -w lab4-tls.pcap 'tcp port 8443'
```

HTTPS request:

```bash
curl -vk https://localhost:8443/health
```

Response:

```json
{"notes":7,"status":"ok"}
```

---

## TLS Negotiation

ClientHello:

```text
TLS handshake, Client hello (1)
```

ServerHello:

```text
TLS handshake, Server hello (2)
```

Negotiated protocol:

```text
TLSv1.3
```

Cipher:

```text
TLS_AES_128_GCM_SHA256
```

---

## Certificate Chain

Certificate chain obtained using:

```bash
openssl s_client -connect localhost:8443 -servername localhost -showcerts </dev/null
```

Chain:

```text
Caddy Local Authority - 2026 ECC Root
        |
Caddy Local Authority - ECC Intermediate
        |
localhost certificate
```

---

## TLS 1.0 / 1.1 Deprecation

TLS versions are negotiated during ClientHello and ServerHello exchange. Modern TLS implementations do not select TLS 1.0 and TLS 1.1 because these versions are deprecated. In this capture, TLS 1.3 was selected during negotiation.

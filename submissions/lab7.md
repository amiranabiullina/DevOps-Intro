# Lab 7 — Configuration Management with Ansible

## Environment and artifacts

- Controller: macOS ARM64, Python 3.11.14, Ansible 10.7.0, ansible-core 2.17.14.
- Target: the Lab 5 Ubuntu 24.04 ARM64 VM, managed by Vagrant and VirtualBox.
- SSH: vagrant@127.0.0.1:2222, using the Vagrant-generated private key.
- Port forwarding: 127.0.0.1:18080 on the host to port 8080 in the VM.

Files:

- [Playbook](../ansible/playbook.yaml)
- [Inventory](../ansible/inventory.ini)
- [Systemd template](../ansible/templates/quicknotes.service.j2)
- [Static binary](../ansible/files/quicknotes)
- [Seed data](../ansible/files/seed.json)
- [Vagrantfile reused from Lab 5](../Vagrantfile)

The inventory uses a key path relative to the repository root. Run Ansible commands from that directory.

Build command, executed inside app/:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o ../ansible/files/quicknotes .
```

The resulting executable was verified as an ARM aarch64 ELF binary, statically linked.

## Task 1 — Deploy QuickNotes

The playbook creates a system group and user without an interactive shell or home directory. It manages the data directory, binary, seed file, and systemd unit with dedicated Ansible modules.

The service runs as quicknotes. Its ADDR, DATA_PATH, SEED_PATH, working directory, executable path, and restart delay come from playbook variables.

The data directory has mode 0750, the binary 0755, and seed.json 0640. The data directory and seed file belong to quicknotes:quicknotes.

### First deployment

```bash
ansible-playbook -i ansible/inventory.ini ansible/playbook.yaml
```

```text
PLAY RECAP *********************************************************************
quicknotes_vm              : ok=8    changed=8    unreachable=0    failed=0    skipped=0    rescued=0    ignored=0
```

[Full first-run output](lab7-logs/01-first-run.txt).

Before deployment, syntax validation passed and Ansible ping returned pong. The initial --check run predicted file and user changes but failed at the service task because check mode had not actually installed the new unit. The real deployment succeeded.

### Service verification

```bash
curl -fsS http://localhost:18080/health
curl -fsS http://localhost:18080/notes
```

The health response was:

```json
{"notes":4,"status":"ok"}
```

The notes response contained all four seed notes. Captured service and HTTP outputs are included below.

### a) command versus dedicated modules

The command module executes a program and normally reports changed whenever it runs; it does not automatically compare the desired state with the current state. Guards such as creates/removes can make particular commands repeatable.

Dedicated modules understand their resources. For example, file checks filesystem attributes, copy compares content and attributes, apt can ensure a package is present, and systemd_service with state: started starts a service only when needed. They are idempotent when used with suitable options; state: restarted deliberately restarts every time it executes.

Idempotency makes repeated deployments safe and prevents unnecessary changes and restarts.

### b) notify and handlers

A task notifies a handler when it reports changed. An unchanged or skipped task does not notify it. On a successful run, notified handlers normally execute after the tasks, and repeated notifications to the same handler produce one execution.

In this playbook, only the binary and unit-template tasks notify restart quicknotes. Updating seed.json alone does not trigger it. This avoids unnecessary interruptions when the deployed configuration is already correct.

### c) Variable organization and precedence

My three preferred locations are:

1. Playbook vars for this small lab's explicit deployment settings, such as paths, listen_addr, and restart_delay. This is what the implementation uses.
2. group_vars/quicknotes.yml for environment-specific settings shared by a group of VMs if the deployment grows.
3. Role defaults for reusable baseline settings if the deployment is later extracted into a role.

Among these locations, playbook vars override inventory group_vars, which override role defaults. A setting intended to be configurable through group_vars should therefore not also be fixed in playbook vars.

### d) Fact gathering

This playbook does not need gathered facts: it targets a known Ubuntu VM, uses explicit paths, and receives a prebuilt ARM64 binary. Therefore gather_facts is false.

This avoids running the setup module and collecting and transferring system facts on every run. It saves processing and connection time; the exact saving depends on the host and network.

## Task 2 — Idempotency and selective changes

### Second run without changes

```text
PLAY RECAP *********************************************************************
quicknotes_vm              : ok=7    changed=0    unreachable=0    failed=0    skipped=0    rescued=0    ignored=0
```

Every task reported ok, and no handler ran.

[Full second-run output](lab7-logs/02-second-run.txt).

### Change one variable

I changed restart_delay from 2 to 3. This changed RestartSec in the generated unit without changing the application's port.

Only Render QuickNotes systemd unit and the restart quicknotes handler reported changed. All other tasks reported ok.

```text
PLAY RECAP *********************************************************************
quicknotes_vm              : ok=8    changed=2    unreachable=0    failed=0    skipped=0    rescued=0    ignored=0
```

The total changed=2 counts one changed template task and one handler execution.

[Full selective-change output](lab7-logs/03-variable-change.txt).

### Preview another change

I changed restart_delay from 3 to 4 and ran:

```bash
ansible-playbook -i ansible/inventory.ini ansible/playbook.yaml --check --diff
```

The diff included:

```diff
-RestartSec=3
+RestartSec=4
```

The predicted recap was:

```text
PLAY RECAP *********************************************************************
quicknotes_vm              : ok=8    changed=2    unreachable=0    failed=0    skipped=0    rescued=0    ignored=0
```

After this preview, systemctl show quicknotes -p RestartUSec still returned RestartUSec=3s. No actual configuration change or restart occurred during check mode.

[Full check-mode diff](lab7-logs/04-check-diff.txt).

I then applied the change normally. The run succeeded with changed=2 and failed=0, and the VM reported RestartUSec=4s.

[Apply output](lab7-logs/05-apply.txt).

### e) Why does the second run report changed=0?

The existing resources already match the requested state. The file module checks the path's existence, type, owner, group, and permissions. The template task renders the desired content and compares it with the destination, also checking managed file attributes.

The binary and seed content are unchanged, and the service is already enabled and running. No notifying task changes, so the restart handler does not execute.

### f) What if shell echo replaced template?

A shell command using > rewrites the file on every normal run and normally reports changed, causing unnecessary handler executions if notify is attached.

Writing only ADDR=... to quicknotes.service would also replace the complete unit with invalid unit content: it would lack the required sections and ExecStart. An existing process might keep running until a reload or restart exposes the broken configuration.

Shell quoting and expansion introduce further risks. The command would not automatically manage ownership and permissions, compare rendered content, or provide the same useful check-mode and diff behavior as template.

### g) What can --check --diff reveal?

Plain --check can report that a template would change without showing the proposed values. --diff lets a reviewer notice a wrong port, incorrect DATA_PATH, or missing SEED_PATH before applying the change.

For example, an accidental DATA_PATH change could make the app use a different notes store. Seeing the exact changed line makes this easier to catch. Neither mode proves that the resulting application will run correctly; runtime checks are still necessary.

## Captured verification output

### Service state after the final apply

```text
active
enabled
[?1h=
RestartUSec=4s[m

[K[?1l>
```

### GET /health

```json
{"notes":4,"status":"ok"}
```

### GET /notes

```json
[{"id":3,"title":"DevOps mantra","body":"If it hurts, do it more often.","created_at":"2026-01-15T10:10:00Z"},{"id":4,"title":"Endpoint cheat-sheet","body":"GET /notes  GET /notes/{id}  POST /notes  DELETE /notes/{id}  GET /health  GET /metrics","created_at":"2026-01-15T10:15:00Z"},{"id":1,"title":"Welcome to QuickNotes","body":"This is the project you'll containerize, deploy, monitor, and harden across all 10 labs.","created_at":"2026-01-15T10:00:00Z"},{"id":2,"title":"Read app/main.go first","body":"Start by understanding the entry point — env vars, signal handling, graceful shutdown.","created_at":"2026-01-15T10:05:00Z"}]
```

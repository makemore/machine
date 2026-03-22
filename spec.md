# mach — Feature Spec for Conduit Integration

These features turn `mach` from a VM launcher into a complete dev environment provisioner.
The goal: a single Machinefile replaces a 270-line startup script + Terraform modules.

---

## 1. `harden: true`

**What:** One flag that locks down a fresh VM — SSH hardening, fail2ban, UFW defaults, unattended upgrades.

**Why:** Every cloud dev box needs this. Currently 40 lines of bash.

```yaml
harden: true
```

**What it does:**
- `PermitRootLogin no`, `PasswordAuthentication no`, `MaxAuthTries 3` in sshd_config
- Install + configure fail2ban (aggressive SSH mode, 24h ban)
- UFW: deny incoming, allow outgoing, allow 22/tcp + any `expose:` ports
- Enable unattended-upgrades for security patches

**Adapters:** Hetzner, DigitalOcean, GCP (skip for local/Lima).

---

## 2. `swap: 4gb`

**What:** Provision swap space.

**Why:** Small cloud VMs (2-4GB RAM) need swap for Node.js builds, AI tools, etc. Currently 6 lines of bash.

```yaml
swap: 4gb
```

**What it does:**
- `fallocate -l 4G /swapfile && mkswap && swapon`
- Add to `/etc/fstab`
- Set `vm.swappiness=10`
- Idempotent — skip if `/swapfile` already exists

**Adapters:** All Linux providers. Skip for macOS/Windows.

---

## 3. `install: docker` (first-class)

**What:** Recognise `docker` as a special install target, not just an apt package.

**Why:** Docker install requires `curl | bash` + adding users to the docker group. Currently a conditional block in the startup script.

```yaml
setup:
  - install: docker
```

**What it does:**
- `curl -fsSL https://get.docker.com | bash`
- Add all `ssh_keys:` / `users:` users to the `docker` group
- Enable + start the docker service

---

## 4. `install: postgres`

**What:** Recognise `postgres` as a special install target.

```yaml
setup:
  - install: postgres
```

**What it does:**
- `apt-get install postgresql postgresql-contrib libpq-dev`
- Create a default dev user + database:
  ```sql
  CREATE USER devuser WITH PASSWORD 'devpass' CREATEDB;
  CREATE DATABASE devdb OWNER devuser;
  ```
- The user/password/dbname can be overridden via env vars: `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`

---

## 5. `install: nodejs` / `install: nodejs-22`

**What:** Recognise Node.js as a special install target with version support.

```yaml
setup:
  - install: nodejs       # latest LTS
  - install: nodejs-22    # specific major version
```

**What it does:**
- Add NodeSource repo for the specified version
- `apt-get install nodejs`
- Verify with `node --version`

---

## 6. `services:`

**What:** Declare long-running daemons. mach creates systemd units, enables, and starts them.

**Why:** The conduitd install is 45 lines of bash (download binary, write config, write systemd unit, enable, start). This pattern repeats for any daemon.

```yaml
services:
  - name: conduitd
    install: https://downloads.myconduit.io/agent/latest/conduitd-linux-amd64
    cmd: /usr/local/bin/conduitd run
    workdir: /opt/project
    env:
      CONDUIT_DEVICE_ID: "abc-123"
      CONDUIT_GATEWAY_URL: "wss://gateway.myconduit.io"
    restart: always

  - name: my-api
    cmd: /opt/project/.venv/bin/python manage.py runserver 0.0.0.0:8000
    workdir: /opt/project
    user: chris
    restart: on-failure
```

**Fields:**
| Field | Required | Description |
|---|---|---|
| `name` | yes | Service name (used for systemd unit) |
| `cmd` | yes | Command to run |
| `install` | no | URL to download binary to `/usr/local/bin/<name>` |
| `workdir` | no | Working directory (default: `/opt/project`) |
| `user` | no | Run as this user (default: root) |
| `env` | no | Environment variables for this service |
| `restart` | no | Restart policy: `always`, `on-failure`, `no` (default: `always`) |

**What it does:**
- If `install` is a URL: download to `/usr/local/bin/<name>`, chmod +x
- Write `/etc/systemd/system/<name>.service`
- `systemctl daemon-reload && systemctl enable <name> && systemctl start <name>`

---

## 7. `reverse_proxy:`

**What:** Auto-configure Caddy as a reverse proxy.

**Why:** Most dev environments have a frontend (port 3000) and API (port 8000). Currently 25 lines of bash to install Caddy + write a Caddyfile.

```yaml
reverse_proxy:
  domain: myapp.dev.myconduit.io    # optional, defaults to :443 with self-signed TLS
  routes:
    - path: /api/*
      target: localhost:8000
    - path: /
      target: localhost:3000
```

**What it does:**
- Install Caddy via official repo
- Generate `/etc/caddy/Caddyfile` from the routes
- If `domain` is set, use it as the host. Otherwise use `:443` with `tls internal`
- Enable + start Caddy

---

## 8. `users:` (enhanced ssh_keys)

**What:** Extend `ssh_keys:` to also set up user accounts properly — home dir, sudo, git credentials, shell config.

**Why:** Currently 50 lines of bash to create users, set up SSH, configure git credentials, inject API keys into bashrc.

```yaml
users:
  - username: chris
    public_key: "ssh-ed25519 AAAA..."
    github_pat: "ghp_..."           # optional, configures git credential store
    sudo: true                      # default: true
    shell: /bin/bash                # default: /bin/bash
```

**What it does:**
- `useradd -m -s <shell> <username>`
- Add to sudo group + NOPASSWD sudoers entry (if `sudo: true`)
- Write `~/.ssh/authorized_keys`
- If `github_pat`: write `~/.git-credentials` + configure `credential.helper = store`
- `chown -R` the home directory

This replaces `ssh_keys:` — or `ssh_keys:` stays as the simple version and `users:` is the full version. Either works.

---

## 9. `motd:`

**What:** Set the message of the day.

```yaml
motd: |
  ════════════════════════════════════════
    Dev Environment: {{ .Name }}
  ════════════════════════════════════════
    START CODING:    code
    PROJECT DIR:     /opt/project
  ════════════════════════════════════════
```

**What it does:**
- Write to `/etc/motd`
- `chmod -x /etc/update-motd.d/*` to suppress default Ubuntu MOTD
- Template variables: `{{ .Name }}`, `{{ .Provider }}`, `{{ .IP }}`

---

## 10. `clone:` with auth

**What:** Enhance the existing `clone:` step to support private repos via PAT or SSH key.

```yaml
setup:
  - clone:
      repo: https://github.com/org/private-repo
      dest: /opt/project
      branch: main
      pat_env: GITHUB_PAT           # use this env var as the PAT for cloning
```

**What it does:**
- If `pat_env` is set: rewrite the URL to `https://pat:<token>@github.com/...` for cloning, then reset the remote URL after
- Support `branch:` field (default: `main`)

---

## Priority Order

| Priority | Feature | Lines of bash it replaces | Complexity |
|---|---|---|---|
| P0 | `harden: true` | 40 | Low |
| P0 | `services:` | 45 per daemon | Medium |
| P1 | `users:` (enhanced) | 50 | Medium |
| P1 | `swap:` | 6 | Trivial |
| P1 | `install: docker` | 10 | Low |
| P1 | `install: postgres` | 5 | Low |
| P1 | `install: nodejs-22` | 4 | Low |
| P2 | `reverse_proxy:` | 25 | Medium |
| P2 | `clone:` with auth | 15 | Low |
| P2 | `motd:` | 15 | Trivial |

---

## End State

After all features are implemented, the Conduit startup script disappears entirely. A complete dev environment is described in ~40 lines of YAML:

```yaml
name: my-dev-box
os: linux
provider: hetzner
region: eu-central
harden: true
swap: 4gb

resources:
  cpu: 4
  memory: 8gb
  disk: 50gb

users:
  - username: chris
    public_key: "ssh-ed25519 AAAA..."
    github_pat: "ghp_..."

env:
  ANTHROPIC_API_KEY: "sk-ant-..."

setup:
  - install: git curl build-essential tmux htop python3
  - install: docker
  - install: postgres
  - install: nodejs-22
  - cmd: npm install -g @anthropic-ai/claude-code
  - clone:
      repo: https://github.com/org/repo
      dest: /opt/project
      branch: main
      pat_env: GITHUB_PAT

services:
  - name: conduitd
    install: https://downloads.myconduit.io/agent/latest/conduitd-linux-amd64
    cmd: /usr/local/bin/conduitd run
    env:
      CONDUIT_DEVICE_ID: "abc-123"

reverse_proxy:
  routes:
    - path: /api/*
      target: localhost:8000
    - path: /
      target: localhost:3000

expose:
  - 3000
  - 8000

motd: |
  Dev Environment: {{ .Name }}
  Run `code` to start coding.
```

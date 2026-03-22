# mach

One command to spin up a dev machine — local VM or cloud server. Linux, macOS, or Windows. Same config file, same workflow.

```
mach up                              # local Linux VM (Lima)
mach -f Machinefile.osx up           # local macOS VM (Tart)
mach -f Machinefile.windows up       # local Windows VM (QEMU)
mach -f Machinefile.hetzner up       # Hetzner Cloud server
mach -f Machinefile.do up            # DigitalOcean droplet
mach -f Machinefile.gcp up           # Google Cloud instance
```

## Install

```bash
curl -fsSL https://makemore.github.io/machine/install.sh | sh
```

Or build from source:

```bash
go build -o mach .
sudo mv mach /usr/local/bin/
```

Check dependencies:

```bash
mach doctor
```


## Machinefile

A `Machinefile` is a YAML file that describes a machine — OS, resources, packages, users, services, everything.

```yaml
name: my-app
os: linux
provider: hetzner          # local (default), hetzner, digitalocean, gcp
region: eu-central         # friendly names mapped to provider slugs
harden: true               # SSH lockdown, fail2ban, UFW, unattended-upgrades
swap: 4gb                  # provision swap space
cloud_init: ./cloud-init.yaml  # raw cloud-init user-data

motd: |
  ╔══════════════════════════════════╗
  ║  {{.Name}} — powered by mach    ║
  ║  IP: {{.IP}}                    ║
  ╚══════════════════════════════════╝

users:                     # full user accounts with SSH, sudo, git creds
  - username: deploy
    public_key: "ssh-ed25519 AAAA..."
    sudo: true
    github_pat: "ghp_..."
  - username: alice
    public_key: "ssh-ed25519 AAAA..."
    shell: /bin/zsh

env:                       # injected into /etc/environment
  NODE_ENV: production
  POSTGRES_USER: appuser
  POSTGRES_PASSWORD: s3cret
  POSTGRES_DB: appdb

resources:
  cpu: 4
  memory: 8gb

setup:
  - install: docker        # first-class: get.docker.com + docker group
  - install: nodejs-22     # first-class: NodeSource + specific version
  - install: postgres      # first-class: apt + creates user/db from env
  - install: git curl      # standard apt packages
  - script: ./scripts/startup.sh   # copy & run a local script
  - clone:
      repo: https://github.com/org/repo
      dest: ~/app
      branch: main         # specific branch
      pat_env: GITHUB_TOKEN  # private repo auth via env var
  - cmd: cd ~/app && make build

services:                  # systemd units, auto-generated
  - name: myapp
    cmd: /usr/local/bin/myapp serve
    workdir: /opt/app
    user: deploy
    restart: always
    env:
      PORT: "8000"
      DATABASE_URL: "postgres://appuser:s3cret@localhost/appdb"
  - name: worker
    cmd: /usr/local/bin/myapp worker
    install: https://example.com/myapp  # download binary

reverse_proxy:             # Caddy auto-config
  domain: myapp.example.com  # omit for self-signed TLS
  routes:
    - path: /
      target: localhost:8000
    - path: /api/*
      target: localhost:3000

expose:                    # firewall rules on Hetzner/DO
  - 80
  - 443
  - 8000
```

## Commands

| Command | Description |
|---|---|
| `mach up` | Create, provision, and start the machine |
| `mach down` | Stop the machine |
| `mach destroy` | Delete the machine (stops cloud billing) |
| `mach ssh` | SSH into the machine |
| `mach status` | Show machine details |
| `mach list` | List all machines across all providers |
| `mach doctor` | Check that all dependencies are installed |

All commands accept `-f <file>`, `--json`, and `--state-file`.

### JSON output

```bash
$ mach -f Machinefile.hetzner up --json
{"name":"dev","ip":"1.2.3.4","status":"running","provider":"hetzner","region":"fsn1-dc14","cpus":4,"memory_mb":8192}

$ mach list --json
[{"name":"dev","status":"running","os":"hetzner","cpus":"4","memory":"8 GB"}]
```

## Features

### First-class installs

| Package | What happens |
|---|---|
| `install: docker` | Installs via [get.docker.com](https://get.docker.com), adds all `users:` to docker group, enables systemd |
| `install: postgres` | Installs PostgreSQL + libpq-dev, creates user/db from `POSTGRES_USER`/`POSTGRES_PASSWORD`/`POSTGRES_DB` env vars |
| `install: nodejs` | Installs latest LTS via [NodeSource](https://deb.nodesource.com) |
| `install: nodejs-22` | Installs Node.js 22.x specifically |
| Anything else | Standard `apt-get install` |

### Security hardening

`harden: true` runs a full security lockdown:

- **SSH** — key-only auth, MaxAuthTries 3
- **fail2ban** — 3 retries, 24-hour ban, SSH jail
- **UFW** — deny all inbound, allow SSH + `expose:` ports
- **unattended-upgrades** — automatic security updates

### Users

```yaml
users:
  - username: deploy
    public_key: "ssh-ed25519 AAAA..."
    sudo: true               # NOPASSWD sudo (default: true)
    github_pat: "ghp_..."    # stored in ~/.git-credentials
    shell: /bin/bash          # default
```

Creates accounts with SSH keys, passwordless sudo, Docker group membership, and git credential storage.

### Services (systemd)

```yaml
services:
  - name: myapp
    cmd: /usr/local/bin/myapp serve
    workdir: /opt/app
    user: deploy
    restart: always
    env:
      PORT: "8000"
    install: https://example.com/myapp  # optional: download binary
```

Generates `/etc/systemd/system/<name>.service`, enables and starts it.

### Reverse proxy (Caddy)

```yaml
reverse_proxy:
  domain: myapp.example.com   # omit for self-signed TLS
  routes:
    - path: /
      target: localhost:8000
    - path: /api/*
      target: localhost:3000
```

Installs Caddy from official repo, generates Caddyfile, auto-TLS.

### Clone with auth

```yaml
- clone:
    repo: https://github.com/org/private-repo
    dest: ~/app
    branch: main
    pat_env: GITHUB_TOKEN    # rewrites URL to https://pat:<token>@github.com/...
```

### Other features

| Feature | Description |
|---|---|
| `swap: 4gb` | Creates swapfile, adds to fstab, sets swappiness to 10 |
| `motd: "..."` | Custom MOTD with `{{.Name}}`, `{{.Provider}}`, `{{.IP}}` template vars |
| `env:` | Injects into `/etc/environment` — persists across reboots |
| `expose:` | Creates firewall rules on Hetzner/DO (SSH always allowed) |
| `cloud_init:` | Raw user-data passed to cloud provider at create time |
| `- script: ./path.sh` | SCP + execute a local script on the VM |
| `--json` | Machine-readable output for CI/CD (IP, status, provider) |
| `--state-file` | Persist machine state for idempotent CI runs |

## Providers

### Local VMs

| OS | Adapter | How it works |
|---|---|---|
| `linux` | [Lima](https://lima-vm.io) | Lightweight Linux VMs on macOS |
| `macos` | [Tart](https://tart.run) | macOS VMs using Apple Virtualization.framework |
| `windows` | QEMU | Windows 11 ARM via QEMU with auto ISO download |

### Cloud Servers

| Provider | `provider:` | Auth | Instance types |
|---|---|---|---|
| Hetzner | `hetzner` | `HETZNER_API_TOKEN` | cax11–cax41 (ARM, from €3.29/mo) |
| DigitalOcean | `digitalocean` | `DIGITALOCEAN_TOKEN` | s-1vcpu-1gb to s-8vcpu-16gb |
| GCP | `gcp` | `gcloud auth login` | t2a-standard-1 to t2a-standard-8 (ARM) |

### Regions

| Region | Hetzner | DigitalOcean | GCP |
|---|---|---|---|
| `eu-central` | fsn1 (Falkenstein) | fra1 (Frankfurt) | europe-west3-a |
| `eu-west` | hel1 (Helsinki) | ams3 (Amsterdam) | europe-west1-b |
| `london` / `uk` | — | lon1 | europe-west2-a |
| `us` / `us-east` | ash (Ashburn) | nyc3 | us-central1-a |
| `us-west` | hil (Hillsboro) | sfo3 | us-west1-a |

## API Keys

Create a `.env` file in your project root (automatically gitignored):

```
HETZNER_API_TOKEN=your-token-here
DIGITALOCEAN_TOKEN=your-token-here
```

GCP uses `gcloud auth login`. `.env` is loaded automatically.

## Architecture

```
Machinefile → mach up → Adapter → CommandRunner → VM/Server
                         │              │
                         │              ├── SSHRunner   (cloud VMs)
                         │              └── LimaRunner  (local Linux)
                         │
                         ├── LimaAdapter         (local Linux)
                         ├── TartAdapter          (local macOS)
                         ├── QemuAdapter          (local Windows)
                         ├── HetznerAdapter       (Hetzner Cloud API)
                         ├── DigitalOceanAdapter   (DO API)
                         └── GCPAdapter           (gcloud CLI)
                         │
                         └── provisionAll(runner)
                              ├── env vars → /etc/environment
                              ├── users → accounts, SSH, sudo, git creds
                              ├── swap → swapfile
                              ├── harden → SSH, fail2ban, UFW, upgrades
                              ├── setup → install (docker/postgres/nodejs), clone, cmd, script
                              ├── services → systemd units
                              ├── reverse_proxy → Caddy + Caddyfile
                              └── motd → /etc/motd
```

The `CommandRunner` interface lets the same provisioning pipeline work across SSH (cloud) and `limactl shell` (local), and enables unit testing via a mock runner.

## Testing

```bash
go test ./...
```

51 unit tests covering Machinefile parsing, state management, and the full provisioning pipeline (using a mock runner that records commands without executing them).

## Examples

See [`examples/`](examples/) for ready-to-use Machinefiles including a full [Conduit dev environment](examples/conduit-dev/Machinefile).
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
go build -o mach .
sudo mv mach /usr/local/bin/
```

Check dependencies:

```bash
mach doctor
```

## Machinefile

A `Machinefile` is a YAML file that describes a machine — what OS, how many CPUs, what to install, what to run.

```yaml
name: dev-box

os: linux
provider: hetzner        # local (default), hetzner, digitalocean, gcp
region: eu-central       # mapped to provider-specific datacenters
cloud_init: ./cloud-init.yaml  # raw user-data for cloud providers

ssh_keys:                # multi-user SSH access
  - username: chris
    public_key: "ssh-ed25519 AAAA..."
  - username: alice
    public_key: "ssh-ed25519 AAAA..."

env:                     # injected into /etc/environment
  API_KEY: "sk-..."
  NODE_ENV: "production"

resources:
  cpu: 4
  memory: 8gb

setup:
  - install: git curl build-essential
  - script: ./scripts/startup.sh   # copy & run a local script on the VM
  - clone:
      repo: https://github.com/your-org/your-repo
      dest: ~/project
  - cmd: cd ~/project && make build

run:
  - cmd: cd ~/project && make serve

expose:                  # creates firewall rules on Hetzner/DO
  - 3000
  - 8080
```

## Commands

| Command | Description |
|---|---|
| `mach up` | Create, provision, and start the machine |
| `mach down` | Stop the machine |
| `mach destroy` | Delete the machine completely |
| `mach ssh` | SSH into the machine |
| `mach status` | Show machine details |
| `mach list` | List all machines across all providers |
| `mach doctor` | Check that all dependencies are installed |

All commands accept `-f <file>` to specify a Machinefile (defaults to `Machinefile`).

### Flags

| Flag | Description |
|---|---|
| `--json` | Machine-readable JSON output (IP, status, provider) |
| `--state-file <path>` | Custom state file path for CI/CD (default: `~/.config/machine/state/<name>.json`) |

### JSON output

```bash
$ mach -f Machinefile.hetzner up --json
{"name":"dev","ip":"1.2.3.4","status":"running","provider":"hetzner","region":"fsn1-dc14","cpus":4,"memory_mb":8192}

$ mach list --json
[{"name":"dev","status":"running","os":"hetzner","cpus":"4","memory":"8 GB"}]
```

## Providers

### Local VMs

| OS | Adapter | How it works |
|---|---|---|
| `linux` | [Lima](https://lima-vm.io) | Lightweight Linux VMs on macOS |
| `macos` | [Tart](https://tart.run) | macOS VMs using Apple Virtualization.framework |
| `windows` | QEMU | Windows 11 ARM via QEMU with auto ISO download |

No `provider` field needed — mach picks the right adapter from the `os` field.

### Cloud Servers

| Provider | `provider:` | Auth | Instance types |
|---|---|---|---|
| Hetzner | `hetzner` | `HETZNER_API_TOKEN` | cax11–cax41 (ARM, from €3.29/mo) |
| DigitalOcean | `digitalocean` | `DIGITALOCEAN_TOKEN` | s-1vcpu-1gb to s-8vcpu-16gb |
| GCP | `gcp` | `gcloud auth login` | t2a-standard-1 to t2a-standard-8 (ARM) |

Resources are auto-mapped to the cheapest matching instance type. Your `~/.ssh/id_ed25519.pub` is uploaded automatically.

### Regions

Use friendly names — mach maps them to provider-specific slugs:

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

GCP uses `gcloud auth login` — no token needed.

Environment variables work too. `.env` is loaded automatically and never overrides existing env vars.

## Examples

```bash
# Spin up a Linux dev box locally
mach up

# SSH in
mach ssh

# See everything running
mach list

# Tear it down
mach destroy

# Cloud: spin up on Hetzner in Germany
mach -f Machinefile.hetzner up

# Cloud: destroy when done (stops billing)
mach -f Machinefile.hetzner destroy
```

## Architecture

```
Machinefile → mach → Adapter → VM/Server
                      ├── LimaAdapter      (local Linux)
                      ├── TartAdapter       (local macOS)
                      ├── QemuAdapter       (local Windows)
                      ├── HetznerAdapter    (Hetzner Cloud API)
                      ├── DigitalOceanAdapter (DO API)
                      └── GCPAdapter        (gcloud CLI)
```

Same interface, same commands, same Machinefile format — whether it's a local VM or a cloud server on the other side of the world.


# Machine / Machinefile Specification (v2 – Cross-Platform, First-Class OS Support)

## Overview

Machine is a CLI tool for defining and launching **persistent compute environments (VMs)** using a single declarative file: **Machinefile**.

Unlike container-only systems, Machine supports:
- Linux
- Windows
- macOS

All as **first-class citizens**, with no reliance on emulation layers like WSL.

---

## Core Philosophy

1. **Single Declarative Spec**
   - Machinefile defines intent, not implementation

2. **Adapter-Based Execution**
   - Each OS/platform has a native adapter

3. **First-Class OS Support**
   - Linux, Windows, macOS treated equally

4. **Minimal Primitive Set**
   - Avoid shell-specific commands in spec

---

## CLI Design

```
machine up          # Create + start machine
machine down        # Stop machine
machine destroy     # Delete machine

machine ssh         # Connect (SSH / WinRM / RDP abstraction)
machine logs        # View logs

machine build       # Optional pre-build step
machine doctor      # Validate environment
```

---

## Machinefile (v2)

### Example

```yaml
name: my-machine

os: windows   # linux | windows | macos

resources:
  cpu: 2
  memory: 4gb

setup:
  - install: git
  - install: python
  - clone:
      repo: https://github.com/example/repo
      dest: C:\\app

run:
  - cmd: python C:\\app\\main.py

expose:
  - 80
```

---

## Core Operations (v1)

Machinefile uses **abstract operations**, not shell commands.

### install

Install a package using platform-native package manager

```yaml
- install: python
```

### clone

Clone a git repository

```yaml
- clone:
    repo: https://github.com/user/repo
    dest: /app
```

### run / cmd

Execute a command

```yaml
- cmd: python main.py
```

### write_file (future)

```yaml
- write_file:
    path: /etc/config
    content: "..."
```

---

## Adapter Mapping

### Linux

- Package manager: apt / yum
- Runtime: Lima (local), GCP / DO (cloud)
- Access: SSH

Example translation:

```bash
apt install -y python3 git
git clone ...
python3 main.py
```

---

### Windows

- Package manager: Chocolatey (choco)
- Runtime: Hyper-V / Parallels / cloud VM
- Access: WinRM / RDP

Example:

```powershell
choco install python git -y
git clone ...
python main.py
```

---

### macOS

- Package manager: Homebrew
- Runtime: UTM / Parallels
- Access: SSH

Example:

```bash
brew install python git
git clone ...
python3 main.py
```

---

## Architecture

```
Machinefile
     ↓
 Translator (intent → ops)
     ↓
Adapters (per OS)
     ↓
VM Runtime (local/cloud)
```

---

## Execution Flow

1. Parse Machinefile
2. Validate spec
3. Select adapter (OS + target)
4. Translate operations
5. Create VM
6. Provision (setup)
7. Execute run commands
8. Expose access (ssh / rdp / winrm)

---

## Targets

### Local

- Linux → Lima
- Windows → Hyper-V / Parallels
- macOS → UTM / Parallels

### Cloud

- Linux → GCP / DO / AWS
- Windows → GCP / Azure
- macOS → niche providers

---

## Design Constraints

- No shell-specific commands in Machinefile
- No provider-specific configuration
- Minimal abstraction surface
- Adapters handle complexity

---

## Future Extensions

- Image caching / snapshots
- Multi-machine orchestration
- Remote dev streaming (Hydra integration)
- Plugin system
- Secrets management
- Networking primitives

---

## Summary

Machine provides:

- Dockerfile-like UX for VMs
- Cross-platform compute definition
- Local-first development
- Native OS execution (no emulation)

---

Generated: 2026-03-21T16:03:42.693307+00:00

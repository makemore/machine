package machinefile

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "Machinefile")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	return path
}

func TestLoad_MinimalValid(t *testing.T) {
	path := writeTempFile(t, `
name: test-vm
os: linux
resources:
  cpu: 2
  memory: 4gb
`)
	mf, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mf.Name != "test-vm" {
		t.Errorf("Name = %q, want %q", mf.Name, "test-vm")
	}
	if mf.OS != "linux" {
		t.Errorf("OS = %q, want %q", mf.OS, "linux")
	}
	if mf.Resources.CPU != 2 {
		t.Errorf("CPU = %d, want 2", mf.Resources.CPU)
	}
	if mf.Resources.Memory != "4gb" {
		t.Errorf("Memory = %q, want %q", mf.Resources.Memory, "4gb")
	}
	// Check defaults
	if mf.Provider != "local" {
		t.Errorf("Provider default = %q, want %q", mf.Provider, "local")
	}
	if mf.PackageManager != "apt" {
		t.Errorf("PackageManager default = %q, want %q", mf.PackageManager, "apt")
	}
}

func TestLoad_MissingName(t *testing.T) {
	path := writeTempFile(t, `
os: linux
resources:
  cpu: 1
  memory: 1gb
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestLoad_MissingOS(t *testing.T) {
	path := writeTempFile(t, `
name: test
resources:
  cpu: 1
  memory: 1gb
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing os")
	}
}

func TestLoad_DefaultCPUAndMemory(t *testing.T) {
	path := writeTempFile(t, `
name: test
os: linux
`)
	mf, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mf.Resources.CPU != 2 {
		t.Errorf("default CPU = %d, want 2", mf.Resources.CPU)
	}
	if mf.Resources.Memory != "4gb" {
		t.Errorf("default Memory = %q, want %q", mf.Resources.Memory, "4gb")
	}
}

func TestLoad_Harden(t *testing.T) {
	path := writeTempFile(t, `
name: test
os: linux
harden: true
`)
	mf, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mf.Harden {
		t.Error("Harden should be true")
	}
}

func TestLoad_Swap(t *testing.T) {
	path := writeTempFile(t, `
name: test
os: linux
swap: 4gb
`)
	mf, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mf.Swap != "4gb" {
		t.Errorf("Swap = %q, want %q", mf.Swap, "4gb")
	}
}

func TestLoad_Users(t *testing.T) {
	path := writeTempFile(t, `
name: test
os: linux
users:
  - username: alice
    public_key: "ssh-ed25519 AAAA..."
    github_pat: "ghp_abc123"
    sudo: true
    shell: /bin/zsh
  - username: bob
    public_key: "ssh-rsa BBBB..."
`)
	mf, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mf.Users) != 2 {
		t.Fatalf("Users count = %d, want 2", len(mf.Users))
	}
	alice := mf.Users[0]
	if alice.Username != "alice" {
		t.Errorf("User[0].Username = %q, want %q", alice.Username, "alice")
	}
	if alice.GithubPAT != "ghp_abc123" {
		t.Errorf("User[0].GithubPAT = %q, want %q", alice.GithubPAT, "ghp_abc123")
	}
	if alice.Sudo == nil || !*alice.Sudo {
		t.Error("User[0].Sudo should be true")
	}
	if alice.Shell != "/bin/zsh" {
		t.Errorf("User[0].Shell = %q, want %q", alice.Shell, "/bin/zsh")
	}
	bob := mf.Users[1]
	if bob.Username != "bob" {
		t.Errorf("User[1].Username = %q, want %q", bob.Username, "bob")
	}
	if bob.Sudo != nil {
		t.Error("User[1].Sudo should be nil (default)")
	}
}

func TestLoad_SSHKeys(t *testing.T) {
	path := writeTempFile(t, `
name: test
os: linux
ssh_keys:
  - username: deploy
    public_key: "ssh-ed25519 AAAA..."
`)
	mf, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mf.SSHKeys) != 1 {
		t.Fatalf("SSHKeys count = %d, want 1", len(mf.SSHKeys))
	}
	if mf.SSHKeys[0].Username != "deploy" {
		t.Errorf("SSHKeys[0].Username = %q, want %q", mf.SSHKeys[0].Username, "deploy")
	}
}

func TestLoad_Env(t *testing.T) {
	path := writeTempFile(t, `
name: test
os: linux
env:
  NODE_ENV: production
  API_KEY: "secret-123"
`)
	mf, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mf.Env) != 2 {
		t.Fatalf("Env count = %d, want 2", len(mf.Env))
	}
	if mf.Env["NODE_ENV"] != "production" {
		t.Errorf("Env[NODE_ENV] = %q, want %q", mf.Env["NODE_ENV"], "production")
	}
	if mf.Env["API_KEY"] != "secret-123" {
		t.Errorf("Env[API_KEY] = %q, want %q", mf.Env["API_KEY"], "secret-123")
	}
}

func TestLoad_SetupSteps(t *testing.T) {
	path := writeTempFile(t, `
name: test
os: linux
setup:
  - install: git curl
  - install: docker
  - install: postgres
  - install: nodejs-22
  - cmd: npm install -g typescript
  - clone:
      repo: https://github.com/org/repo
      dest: /opt/project
      branch: develop
      pat_env: GITHUB_PAT
  - script: ./setup.sh
`)
	mf, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mf.Setup) != 7 {
		t.Fatalf("Setup count = %d, want 7", len(mf.Setup))
	}
	// install steps
	if mf.Setup[0].Install != "git curl" {
		t.Errorf("Setup[0].Install = %q, want %q", mf.Setup[0].Install, "git curl")
	}
	if mf.Setup[1].Install != "docker" {
		t.Errorf("Setup[1].Install = %q, want %q", mf.Setup[1].Install, "docker")
	}
	if mf.Setup[2].Install != "postgres" {
		t.Errorf("Setup[2].Install = %q, want %q", mf.Setup[2].Install, "postgres")
	}
	if mf.Setup[3].Install != "nodejs-22" {
		t.Errorf("Setup[3].Install = %q, want %q", mf.Setup[3].Install, "nodejs-22")
	}
	// cmd step
	if mf.Setup[4].Cmd != "npm install -g typescript" {
		t.Errorf("Setup[4].Cmd = %q", mf.Setup[4].Cmd)
	}
	// clone step
	clone := mf.Setup[5].Clone
	if clone == nil {
		t.Fatal("Setup[5].Clone should not be nil")
	}
	if clone.Repo != "https://github.com/org/repo" {
		t.Errorf("Clone.Repo = %q", clone.Repo)
	}
	if clone.Dest != "/opt/project" {
		t.Errorf("Clone.Dest = %q", clone.Dest)
	}
	if clone.Branch != "develop" {
		t.Errorf("Clone.Branch = %q", clone.Branch)
	}
	if clone.PATEnv != "GITHUB_PAT" {
		t.Errorf("Clone.PATEnv = %q", clone.PATEnv)
	}
	// script step
	if mf.Setup[6].Script != "./setup.sh" {
		t.Errorf("Setup[6].Script = %q", mf.Setup[6].Script)
	}
}

func TestLoad_Services(t *testing.T) {
	path := writeTempFile(t, `
name: test
os: linux
services:
  - name: myapi
    cmd: /opt/project/run.sh
    install: https://example.com/bin/myapi
    workdir: /opt/project
    user: deploy
    restart: on-failure
    env:
      PORT: "8000"
      DB_URL: "postgres://localhost/mydb"
  - name: worker
    cmd: /usr/local/bin/worker run
`)
	mf, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mf.Services) != 2 {
		t.Fatalf("Services count = %d, want 2", len(mf.Services))
	}
	svc := mf.Services[0]
	if svc.Name != "myapi" {
		t.Errorf("Services[0].Name = %q", svc.Name)
	}
	if svc.Cmd != "/opt/project/run.sh" {
		t.Errorf("Services[0].Cmd = %q", svc.Cmd)
	}
	if svc.Install != "https://example.com/bin/myapi" {
		t.Errorf("Services[0].Install = %q", svc.Install)
	}
	if svc.Workdir != "/opt/project" {
		t.Errorf("Services[0].Workdir = %q", svc.Workdir)
	}
	if svc.User != "deploy" {
		t.Errorf("Services[0].User = %q", svc.User)
	}
	if svc.Restart != "on-failure" {
		t.Errorf("Services[0].Restart = %q", svc.Restart)
	}
	if len(svc.Env) != 2 {
		t.Fatalf("Services[0].Env count = %d, want 2", len(svc.Env))
	}
	// Minimal service
	worker := mf.Services[1]
	if worker.Name != "worker" {
		t.Errorf("Services[1].Name = %q", worker.Name)
	}
	if worker.Workdir != "" {
		t.Errorf("Services[1].Workdir should be empty, got %q", worker.Workdir)
	}
}

func TestLoad_ReverseProxy(t *testing.T) {
	path := writeTempFile(t, `
name: test
os: linux
reverse_proxy:
  domain: myapp.example.com
  routes:
    - path: /api/*
      target: localhost:8000
    - path: /
      target: localhost:3000
`)
	mf, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mf.ReverseProxy == nil {
		t.Fatal("ReverseProxy should not be nil")
	}
	if mf.ReverseProxy.Domain != "myapp.example.com" {
		t.Errorf("ReverseProxy.Domain = %q", mf.ReverseProxy.Domain)
	}
	if len(mf.ReverseProxy.Routes) != 2 {
		t.Fatalf("Routes count = %d, want 2", len(mf.ReverseProxy.Routes))
	}
	if mf.ReverseProxy.Routes[0].Path != "/api/*" {
		t.Errorf("Routes[0].Path = %q", mf.ReverseProxy.Routes[0].Path)
	}
	if mf.ReverseProxy.Routes[0].Target != "localhost:8000" {
		t.Errorf("Routes[0].Target = %q", mf.ReverseProxy.Routes[0].Target)
	}
}

func TestLoad_ReverseProxy_NoDomain(t *testing.T) {
	path := writeTempFile(t, `
name: test
os: linux
reverse_proxy:
  routes:
    - path: /
      target: localhost:3000
`)
	mf, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mf.ReverseProxy.Domain != "" {
		t.Errorf("ReverseProxy.Domain should be empty, got %q", mf.ReverseProxy.Domain)
	}
}

func TestLoad_Expose(t *testing.T) {
	path := writeTempFile(t, `
name: test
os: linux
expose:
  - 3000
  - 8000
  - 443
`)
	mf, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mf.Expose) != 3 {
		t.Fatalf("Expose count = %d, want 3", len(mf.Expose))
	}
	expected := []int{3000, 8000, 443}
	for i, want := range expected {
		if mf.Expose[i] != want {
			t.Errorf("Expose[%d] = %d, want %d", i, mf.Expose[i], want)
		}
	}
}

func TestLoad_MOTD(t *testing.T) {
	path := writeTempFile(t, `
name: test-box
os: linux
motd: |
  Welcome to {{.Name}}
  Provider: {{.Provider}}
`)
	mf, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mf.MOTD == "" {
		t.Fatal("MOTD should not be empty")
	}
	if mf.MOTD != "Welcome to {{.Name}}\nProvider: {{.Provider}}\n" {
		t.Errorf("MOTD = %q", mf.MOTD)
	}
}

func TestLoad_CloudInit(t *testing.T) {
	path := writeTempFile(t, `
name: test
os: linux
cloud_init: |
  #cloud-config
  packages:
    - nginx
`)
	mf, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mf.CloudInit == "" {
		t.Fatal("CloudInit should not be empty")
	}
}

func TestLoad_ProviderAndRegion(t *testing.T) {
	path := writeTempFile(t, `
name: test
os: linux
provider: hetzner
region: eu-central
`)
	mf, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mf.Provider != "hetzner" {
		t.Errorf("Provider = %q, want %q", mf.Provider, "hetzner")
	}
	if mf.Region != "eu-central" {
		t.Errorf("Region = %q, want %q", mf.Region, "eu-central")
	}
}

func TestLoad_Disk(t *testing.T) {
	path := writeTempFile(t, `
name: test
os: linux
resources:
  cpu: 4
  memory: 8gb
  disk: 50gb
`)
	mf, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mf.Resources.Disk != "50gb" {
		t.Errorf("Disk = %q, want %q", mf.Resources.Disk, "50gb")
	}
}

func TestLoad_FullMachinefile(t *testing.T) {
	path := writeTempFile(t, `
name: full-test
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
    github_pat: "ghp_test"

env:
  ANTHROPIC_API_KEY: "sk-ant-test"

setup:
  - install: git curl build-essential
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
  Dev Environment: {{.Name}}
  Run 'code' to start coding.
`)
	mf, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify everything parsed
	if mf.Name != "full-test" {
		t.Errorf("Name = %q", mf.Name)
	}
	if mf.Provider != "hetzner" {
		t.Errorf("Provider = %q", mf.Provider)
	}
	if !mf.Harden {
		t.Error("Harden should be true")
	}
	if mf.Swap != "4gb" {
		t.Errorf("Swap = %q", mf.Swap)
	}
	if len(mf.Users) != 1 {
		t.Errorf("Users count = %d", len(mf.Users))
	}
	if len(mf.Env) != 1 {
		t.Errorf("Env count = %d", len(mf.Env))
	}
	if len(mf.Setup) != 6 {
		t.Errorf("Setup count = %d", len(mf.Setup))
	}
	if len(mf.Services) != 1 {
		t.Errorf("Services count = %d", len(mf.Services))
	}
	if mf.ReverseProxy == nil {
		t.Error("ReverseProxy should not be nil")
	}
	if len(mf.Expose) != 2 {
		t.Errorf("Expose count = %d", len(mf.Expose))
	}
	if mf.MOTD == "" {
		t.Error("MOTD should not be empty")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/Machinefile")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	path := writeTempFile(t, `
name: test
os: linux
  this is invalid:
    - yaml {{{
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

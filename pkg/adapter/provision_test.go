package adapter

import (
	"testing"

	"github.com/makemore/machine/pkg/machinefile"
)

// mockRunner records commands without executing them
type mockRunner struct {
	commands []string
}

func (m *mockRunner) Exec(command string) error {
	m.commands = append(m.commands, command)
	return nil
}

func (m *mockRunner) CopyFile(localPath, remotePath string) error {
	m.commands = append(m.commands, "COPY:"+localPath+"->"+remotePath)
	return nil
}

func TestCollectUsernames(t *testing.T) {
	mf := &machinefile.Machinefile{
		Users: []machinefile.UserSpec{
			{Username: "alice"},
			{Username: "bob"},
		},
		SSHKeys: []machinefile.SSHKeySpec{
			{Username: "charlie"},
		},
	}
	names := collectUsernames(mf)
	if len(names) != 3 {
		t.Fatalf("got %d names, want 3", len(names))
	}
	expected := map[string]bool{"alice": true, "bob": true, "charlie": true}
	for _, name := range names {
		if !expected[name] {
			t.Errorf("unexpected username: %q", name)
		}
	}
}

func TestCollectUsernames_Empty(t *testing.T) {
	mf := &machinefile.Machinefile{}
	names := collectUsernames(mf)
	if len(names) != 0 {
		t.Errorf("expected 0 names, got %d", len(names))
	}
}

func TestWaitForApt(t *testing.T) {
	cmd := waitForApt("sudo ")
	if cmd == "" {
		t.Error("waitForApt returned empty string")
	}
}

func TestAptInstall(t *testing.T) {
	cmd := aptInstall("sudo ", "git curl")
	if cmd == "" {
		t.Error("aptInstall returned empty string")
	}
	if !containsStr(cmd, "git curl") {
		t.Errorf("command should contain 'git curl': %q", cmd)
	}
	if !containsStr(cmd, "sudo") {
		t.Errorf("command should contain 'sudo': %q", cmd)
	}
}

func TestAptInstall_NoSudo(t *testing.T) {
	cmd := aptInstall("", "vim")
	if !containsStr(cmd, "vim") {
		t.Errorf("command should contain 'vim': %q", cmd)
	}
}

func TestProvisionEnv_Empty(t *testing.T) {
	mf := &machinefile.Machinefile{}
	run := &mockRunner{}
	err := provisionEnv(mf, run, "")
	if err != nil {
		t.Errorf("expected nil for empty env, got: %v", err)
	}
	if len(run.commands) != 0 {
		t.Errorf("expected no commands, got %d", len(run.commands))
	}
}

func TestProvisionEnv_SetsVars(t *testing.T) {
	mf := &machinefile.Machinefile{
		Env: map[string]string{"FOO": "bar", "BAZ": "qux"},
	}
	run := &mockRunner{}
	err := provisionEnv(mf, run, "sudo ")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(run.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(run.commands))
	}
}

func TestProvisionUsers_Empty(t *testing.T) {
	mf := &machinefile.Machinefile{}
	run := &mockRunner{}
	err := provisionUsers(mf, run, "")
	if err != nil {
		t.Errorf("expected nil for empty users, got: %v", err)
	}
}

func TestProvisionUsers_CreatesUser(t *testing.T) {
	mf := &machinefile.Machinefile{
		Users: []machinefile.UserSpec{
			{Username: "alice", PublicKey: "ssh-ed25519 AAAA"},
		},
	}
	run := &mockRunner{}
	err := provisionUsers(mf, run, "sudo ")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(run.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(run.commands))
	}
	if !containsStr(run.commands[0], "alice") {
		t.Errorf("command should contain 'alice': %q", run.commands[0])
	}
}

func TestProvisionSwap_Empty(t *testing.T) {
	mf := &machinefile.Machinefile{}
	run := &mockRunner{}
	err := provisionSwap(mf, run, "")
	if err != nil {
		t.Errorf("expected nil for empty swap, got: %v", err)
	}
}

func TestProvisionSwap_Creates(t *testing.T) {
	mf := &machinefile.Machinefile{Swap: "4gb"}
	run := &mockRunner{}
	err := provisionSwap(mf, run, "sudo ")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(run.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(run.commands))
	}
	if !containsStr(run.commands[0], "4G") {
		t.Errorf("command should contain '4G': %q", run.commands[0])
	}
}

func TestProvisionHarden_Disabled(t *testing.T) {
	mf := &machinefile.Machinefile{Harden: false}
	run := &mockRunner{}
	err := provisionHarden(mf, run, "")
	if err != nil {
		t.Errorf("expected nil for harden=false, got: %v", err)
	}
	if len(run.commands) != 0 {
		t.Errorf("expected no commands, got %d", len(run.commands))
	}
}

func TestProvisionHarden_Enabled(t *testing.T) {
	mf := &machinefile.Machinefile{Harden: true, Expose: []int{3000}}
	run := &mockRunner{}
	err := provisionHarden(mf, run, "sudo ")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Should run: SSH hardening, fail2ban, UFW, unattended-upgrades = 4 commands
	if len(run.commands) != 4 {
		t.Errorf("expected 4 commands, got %d", len(run.commands))
	}
}

func TestProvisionServices_Empty(t *testing.T) {
	mf := &machinefile.Machinefile{}
	run := &mockRunner{}
	err := provisionServices(mf, run, "")
	if err != nil {
		t.Errorf("expected nil for empty services, got: %v", err)
	}
}

func TestProvisionServices_CreatesUnit(t *testing.T) {
	mf := &machinefile.Machinefile{
		Services: []machinefile.ServiceSpec{
			{Name: "myapi", Cmd: "/usr/bin/myapi run"},
		},
	}
	run := &mockRunner{}
	err := provisionServices(mf, run, "sudo ")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Should run: write unit + daemon-reload/enable/start = 2 commands
	if len(run.commands) != 2 {
		t.Fatalf("expected 2 commands, got %d", len(run.commands))
	}
	if !containsStr(run.commands[0], "myapi.service") {
		t.Errorf("should write myapi.service: %q", run.commands[0])
	}
}

func TestProvisionReverseProxy_Nil(t *testing.T) {
	mf := &machinefile.Machinefile{}
	run := &mockRunner{}
	err := provisionReverseProxy(mf, run, "")
	if err != nil {
		t.Errorf("expected nil for nil reverse_proxy, got: %v", err)
	}
}

func TestProvisionReverseProxy_EmptyRoutes(t *testing.T) {
	mf := &machinefile.Machinefile{
		ReverseProxy: &machinefile.ReverseProxy{
			Routes: []machinefile.ProxyRoute{},
		},
	}
	run := &mockRunner{}
	err := provisionReverseProxy(mf, run, "")
	if err != nil {
		t.Errorf("expected nil for empty routes, got: %v", err)
	}
}

func TestProvisionMOTD_Empty(t *testing.T) {
	mf := &machinefile.Machinefile{}
	run := &mockRunner{}
	err := provisionMOTD(mf, run, "1.2.3.4", "")
	if err != nil {
		t.Errorf("expected nil for empty motd, got: %v", err)
	}
}

func TestProvisionMOTD_TemplatesVars(t *testing.T) {
	mf := &machinefile.Machinefile{
		Name:     "test-box",
		Provider: "hetzner",
		MOTD:     "Welcome to {{.Name}} on {{.Provider}} at {{.IP}}",
	}
	run := &mockRunner{}
	err := provisionMOTD(mf, run, "1.2.3.4", "")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(run.commands) != 1 {
		t.Fatalf("expected 1 command, got %d", len(run.commands))
	}
	if !containsStr(run.commands[0], "test-box") {
		t.Errorf("should contain 'test-box': %q", run.commands[0])
	}
	if !containsStr(run.commands[0], "hetzner") {
		t.Errorf("should contain 'hetzner': %q", run.commands[0])
	}
	if !containsStr(run.commands[0], "1.2.3.4") {
		t.Errorf("should contain '1.2.3.4': %q", run.commands[0])
	}
}

func TestProvisionSetup_DockerInstall(t *testing.T) {
	mf := &machinefile.Machinefile{
		Setup: []machinefile.Step{
			{Install: "docker"},
		},
	}
	run := &mockRunner{}
	err := provisionSetup(mf, run, "sudo ")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Docker install should use get.docker.com, not apt
	found := false
	for _, cmd := range run.commands {
		if containsStr(cmd, "get.docker.com") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("docker install should use get.docker.com, got: %v", run.commands)
	}
}

func TestProvisionSetup_NodeJSInstall(t *testing.T) {
	mf := &machinefile.Machinefile{
		Setup: []machinefile.Step{
			{Install: "nodejs-22"},
		},
	}
	run := &mockRunner{}
	err := provisionSetup(mf, run, "sudo ")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	found := false
	for _, cmd := range run.commands {
		if containsStr(cmd, "nodesource") || containsStr(cmd, "setup_22") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("nodejs-22 install should use nodesource, got: %v", run.commands)
	}
}

func TestProvisionAll_FullPipeline(t *testing.T) {
	mf := &machinefile.Machinefile{
		Name:   "test",
		Harden: true,
		Swap:   "2gb",
		Env:    map[string]string{"FOO": "bar"},
		Users: []machinefile.UserSpec{
			{Username: "dev", PublicKey: "ssh-ed25519 AAAA"},
		},
		Setup: []machinefile.Step{
			{Install: "git"},
			{Cmd: "echo hello"},
		},
		Services: []machinefile.ServiceSpec{
			{Name: "myapp", Cmd: "/usr/bin/myapp"},
		},
		MOTD:   "Welcome",
		Expose: []int{3000},
	}
	run := &mockRunner{}
	err := provisionAll(mf, run, "1.2.3.4", true)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Should have run many commands
	if len(run.commands) < 5 {
		t.Errorf("expected at least 5 commands, got %d", len(run.commands))
	}
}

// helper
func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

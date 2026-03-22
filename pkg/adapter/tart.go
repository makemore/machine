package adapter

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/makemore/machine/pkg/machinefile"
)

// TartAdapter implements Adapter for macOS using Tart
type TartAdapter struct{}

// NewTartAdapter creates a new Tart adapter
func NewTartAdapter() *TartAdapter {
	return &TartAdapter{}
}

// Create creates a macOS VM via Tart
func (t *TartAdapter) Create(mf *machinefile.Machinefile) error {
	// Check if VM already exists
	checkCmd := exec.Command("tart", "get", mf.Name)
	if err := checkCmd.Run(); err == nil {
		fmt.Printf("   VM '%s' already exists, starting...\n", mf.Name)
		return t.startVM(mf)
	}

	// Clone from the image (Tart uses OCI images or "latest" IPSW)
	image := mf.Image
	if image == "" {
		image = "ghcr.io/cirruslabs/macos-sequoia-base:latest"
	}

	fmt.Printf("   📦 Cloning VM '%s' from %s...\n", mf.Name, image)
	cloneCmd := exec.Command("tart", "clone", image, mf.Name)
	cloneCmd.Stdout = os.Stdout
	cloneCmd.Stderr = os.Stderr
	if err := cloneCmd.Run(); err != nil {
		return fmt.Errorf("cloning VM: %w", err)
	}

	// Configure CPU and memory
	memoryMB := parseMemoryMB(mf.Resources.Memory)
	setCmd := exec.Command("tart", "set", mf.Name,
		"--cpu", strconv.Itoa(mf.Resources.CPU),
		"--memory", strconv.Itoa(memoryMB))
	if err := setCmd.Run(); err != nil {
		return fmt.Errorf("configuring VM: %w", err)
	}

	return t.startVM(mf)
}

// startVM starts the Tart VM in the background (headless)
func (t *TartAdapter) startVM(mf *machinefile.Machinefile) error {
	cmd := exec.Command("tart", "run", "--no-graphics", mf.Name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting VM: %w", err)
	}

	// Wait for VM to get an IP (indicates it's booted)
	fmt.Printf("   ⏳ Waiting for VM to boot...\n")
	for i := 0; i < 60; i++ {
		time.Sleep(2 * time.Second)
		ipCmd := exec.Command("tart", "ip", mf.Name)
		if output, err := ipCmd.Output(); err == nil && strings.TrimSpace(string(output)) != "" {
			fmt.Printf("   ✅ VM is ready at %s\n", strings.TrimSpace(string(output)))
			return nil
		}
	}

	return fmt.Errorf("timed out waiting for VM to boot")
}

// parseMemoryMB converts "8gb" to 8192
func parseMemoryMB(mem string) int {
	mem = strings.ToLower(strings.TrimSpace(mem))
	mem = strings.TrimSuffix(mem, "gb")
	gb, err := strconv.Atoi(mem)
	if err != nil {
		return 4096 // default 4GB
	}
	return gb * 1024
}

// getInstallCommand returns the install command for macOS package managers
func (t *TartAdapter) getInstallCommand(pm, pkg string) string {
	switch pm {
	case "brew":
		return fmt.Sprintf("eval \"$(/opt/homebrew/bin/brew shellenv)\" && brew install %s", pkg)
	default:
		return fmt.Sprintf("eval \"$(/opt/homebrew/bin/brew shellenv)\" && brew install %s", pkg)
	}
}

// ensurePackageManager checks if the package manager is available and installs it if needed
func (t *TartAdapter) ensurePackageManager(mf *machinefile.Machinefile) error {
	if mf.PackageManager == "brew" {
		// Check if brew is installed
		if err := t.execInVM(mf.Name, "which brew"); err != nil {
			fmt.Printf("   🍺 Installing Homebrew...\n")
			installCmd := `NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`
			if err := t.execInVM(mf.Name, installCmd); err != nil {
				return fmt.Errorf("installing Homebrew: %w", err)
			}
			// Add brew to PATH for this session and future shells
			addPathCmd := `echo 'eval "$(/opt/homebrew/bin/brew shellenv)"' >> ~/.zprofile && eval "$(/opt/homebrew/bin/brew shellenv)"`
			if err := t.execInVM(mf.Name, addPathCmd); err != nil {
				return fmt.Errorf("configuring Homebrew PATH: %w", err)
			}
		}
	}
	return nil
}

// Provision runs setup steps inside the VM
func (t *TartAdapter) Provision(mf *machinefile.Machinefile) error {
	// Ensure the package manager is available before running setup steps
	if err := t.ensurePackageManager(mf); err != nil {
		return err
	}

	for _, step := range mf.Setup {
		if step.Install != "" {
			fmt.Printf("   📦 Installing %s...\n", step.Install)
			cmd := t.getInstallCommand(mf.PackageManager, step.Install)
			if err := t.execInVM(mf.Name, cmd); err != nil {
				return fmt.Errorf("installing %s: %w", step.Install, err)
			}
		}
		if step.Clone != nil {
			fmt.Printf("   📥 Cloning %s...\n", step.Clone.Repo)
			cmd := fmt.Sprintf("git clone %s %s", step.Clone.Repo, step.Clone.Dest)
			if err := t.execInVM(mf.Name, cmd); err != nil {
				return fmt.Errorf("cloning %s: %w", step.Clone.Repo, err)
			}
		}
		if step.Cmd != "" {
			fmt.Printf("   ▶️  Running: %s\n", step.Cmd)
			if err := t.execInVM(mf.Name, step.Cmd); err != nil {
				return fmt.Errorf("running cmd: %w", err)
			}
		}
	}
	return nil
}

// Run executes run commands
func (t *TartAdapter) Run(mf *machinefile.Machinefile) error {
	for _, step := range mf.Run {
		if step.Cmd != "" {
			fmt.Printf("   ▶️  %s\n", step.Cmd)
			if err := t.execInVM(mf.Name, step.Cmd); err != nil {
				return err
			}
		}
	}
	return nil
}



// execInVM runs a command inside the Tart VM via SSH
func (t *TartAdapter) execInVM(name, command string) error {
	// Get VM IP
	ipCmd := exec.Command("tart", "ip", name)
	ipOutput, err := ipCmd.Output()
	if err != nil {
		return fmt.Errorf("getting VM IP: %w", err)
	}
	ip := strings.TrimSpace(string(ipOutput))

	// Wrap command to source brew environment if available
	wrappedCmd := fmt.Sprintf(`if [ -x /opt/homebrew/bin/brew ]; then eval "$(/opt/homebrew/bin/brew shellenv)"; fi; %s`, command)

	// SSH into the VM (Tart macOS VMs have SSH enabled with admin/admin by default)
	cmd := exec.Command("sshpass", "-p", "admin",
		"ssh", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		fmt.Sprintf("admin@%s", ip), wrappedCmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Stop stops the VM
func (t *TartAdapter) Stop(mf *machinefile.Machinefile) error {
	cmd := exec.Command("tart", "stop", mf.Name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Destroy deletes the VM
func (t *TartAdapter) Destroy(mf *machinefile.Machinefile) error {
	// Stop first, ignore errors if already stopped
	_ = t.Stop(mf)
	cmd := exec.Command("tart", "delete", mf.Name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Connect opens an SSH session to the VM
func (t *TartAdapter) Connect(mf *machinefile.Machinefile) error {
	ipCmd := exec.Command("tart", "ip", mf.Name)
	ipOutput, err := ipCmd.Output()
	if err != nil {
		return fmt.Errorf("getting VM IP (is the VM running?): %w", err)
	}
	ip := strings.TrimSpace(string(ipOutput))

	cmd := exec.Command("sshpass", "-p", "admin",
		"ssh", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		fmt.Sprintf("admin@%s", ip))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// tartListEntry represents a VM in tart list --format json output
type tartListEntry struct {
	Name   string `json:"Name"`
	State  string `json:"State"`
	CPU    int    `json:"CPU"`
	Memory int    `json:"Memory"`
	Disk   int    `json:"Disk"`
}

// Status prints the current status of the VM
func (t *TartAdapter) Status(mf *machinefile.Machinefile) error {
	cmd := exec.Command("tart", "list", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("listing VMs: %w", err)
	}

	var vms []tartListEntry
	if err := json.Unmarshal(output, &vms); err != nil {
		return fmt.Errorf("parsing VM list: %w", err)
	}

	for _, vm := range vms {
		if vm.Name == mf.Name {
			icon := "⚪"
			switch vm.State {
			case "running":
				icon = "🟢"
			case "stopped":
				icon = "🔴"
			case "suspended":
				icon = "🟡"
			}

			fmt.Printf("%s  Machine: %s\n", icon, vm.Name)
			fmt.Printf("   Status:  %s\n", vm.State)
			fmt.Printf("   OS:      macOS\n")
			fmt.Printf("   CPUs:    %d\n", vm.CPU)
			fmt.Printf("   Memory:  %d MB\n", vm.Memory)
			fmt.Printf("   Disk:    %d GB\n", vm.Disk)
			return nil
		}
	}

	return fmt.Errorf("machine '%s' not found (has it been created?)", mf.Name)
}

// Info returns structured machine info
func (t *TartAdapter) Info(mf *machinefile.Machinefile) (*MachineInfo, error) {
	return &MachineInfo{
		Name:     mf.Name,
		Status:   "unknown",
		Provider: "local",
		OS:       "macos",
		CPUs:     mf.Resources.CPU,
	}, nil
}
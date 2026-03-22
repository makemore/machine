package adapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/makemore/machine/pkg/machinefile"
)

const hetznerAPI = "https://api.hetzner.cloud/v1"

// HetznerAdapter implements Adapter for Hetzner Cloud
type HetznerAdapter struct {
	token string
}

// NewHetznerAdapter creates a new Hetzner adapter
func NewHetznerAdapter() *HetznerAdapter {
	return &HetznerAdapter{
		token: os.Getenv("HETZNER_API_TOKEN"),
	}
}

// hetznerRequest makes an authenticated request to the Hetzner API
func (h *HetznerAdapter) hetznerRequest(method, path string, body interface{}) ([]byte, int, error) {
	if h.token == "" {
		return nil, 0, fmt.Errorf("HETZNER_API_TOKEN not set (add it to .env or export it)")
	}

	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, hetznerAPI+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}

	return respBody, resp.StatusCode, nil
}

// mapResources maps Machinefile resources to a Hetzner server type
func (h *HetznerAdapter) mapServerType(mf *machinefile.Machinefile) string {
	cpu := mf.Resources.CPU
	mem := parseMemoryMB(mf.Resources.Memory) / 1024 // GB

	// Hetzner ARM (Ampere) server types — cheapest
	if cpu <= 2 && mem <= 4 {
		return "cax11" // 2 vCPU, 4GB, €3.29/mo
	} else if cpu <= 4 && mem <= 8 {
		return "cax21" // 4 vCPU, 8GB, €5.49/mo
	} else if cpu <= 8 && mem <= 16 {
		return "cax31" // 8 vCPU, 16GB, €10.49/mo
	} else if cpu <= 16 && mem <= 32 {
		return "cax41" // 16 vCPU, 32GB, €18.49/mo
	}
	return "cax11"
}

// mapRegion maps a region string to a Hetzner datacenter
func (h *HetznerAdapter) mapRegion(region string) string {
	switch strings.ToLower(region) {
	case "eu", "eu-central", "germany", "de":
		return "fsn1"
	case "eu-west", "finland", "fi":
		return "hel1"
	case "us", "us-east", "virginia":
		return "ash"
	case "us-west":
		return "hil"
	default:
		if region != "" {
			return region // pass through if it's already a Hetzner DC name
		}
		return "fsn1" // default to Falkenstein
	}
}

// mapImage maps an image string to a Hetzner image
func (h *HetznerAdapter) mapImage(image string) string {
	switch strings.ToLower(image) {
	case "", "ubuntu":
		return "ubuntu-24.04"
	case "debian":
		return "debian-12"
	case "fedora":
		return "fedora-41"
	case "alpine":
		return "ubuntu-24.04" // Alpine not available on Hetzner, use Ubuntu
	default:
		return image
	}
}

// getSSHKeyID uploads or finds the SSH key on Hetzner, returns its ID
func (h *HetznerAdapter) getSSHKeyID(mf *machinefile.Machinefile) (int64, error) {
	if mf.SSHKey == "" {
		return 0, fmt.Errorf("no SSH key found — create one with: ssh-keygen -t ed25519")
	}

	pubKeyData, err := os.ReadFile(mf.SSHKey)
	if err != nil {
		return 0, fmt.Errorf("reading SSH key %s: %w", mf.SSHKey, err)
	}
	pubKey := strings.TrimSpace(string(pubKeyData))
	keyName := fmt.Sprintf("mach-%s", mf.Name)

	// Check if key already exists
	resp, status, err := h.hetznerRequest("GET", "/ssh_keys?name="+keyName, nil)
	if err != nil {
		return 0, err
	}
	if status == 200 {
		var result struct {
			SSHKeys []struct {
				ID int64 `json:"id"`
			} `json:"ssh_keys"`
		}
		if json.Unmarshal(resp, &result) == nil && len(result.SSHKeys) > 0 {
			return result.SSHKeys[0].ID, nil
		}
	}



	// Create key
	createResp, status, err := h.hetznerRequest("POST", "/ssh_keys", map[string]string{
		"name":       keyName,
		"public_key": pubKey,
	})
	if err != nil {
		return 0, err
	}
	if status != 201 {
		return 0, fmt.Errorf("creating SSH key: HTTP %d: %s", status, string(createResp))
	}

	var createResult struct {
		SSHKey struct {
			ID int64 `json:"id"`
		} `json:"ssh_key"`
	}
	if err := json.Unmarshal(createResp, &createResult); err != nil {
		return 0, err
	}
	return createResult.SSHKey.ID, nil
}

// Create creates a Hetzner Cloud server
func (h *HetznerAdapter) Create(mf *machinefile.Machinefile) error {
	// Check if server already exists
	resp, _, err := h.hetznerRequest("GET", "/servers?name="+mf.Name, nil)
	if err != nil {
		return err
	}
	var existing struct {
		Servers []struct {
			ID     int64  `json:"id"`
			Status string `json:"status"`
		} `json:"servers"`
	}
	if json.Unmarshal(resp, &existing) == nil && len(existing.Servers) > 0 {
		srv := existing.Servers[0]
		if srv.Status == "running" {
			fmt.Printf("   Server '%s' already running\n", mf.Name)
			return nil
		}
		fmt.Printf("   Server '%s' exists, powering on...\n", mf.Name)
		_, _, err := h.hetznerRequest("POST", fmt.Sprintf("/servers/%d/actions/poweron", srv.ID), nil)
		return err
	}

	// Get SSH key
	fmt.Printf("   🔑 Setting up SSH key...\n")
	sshKeyID, err := h.getSSHKeyID(mf)
	if err != nil {
		return err
	}

	serverType := h.mapServerType(mf)
	location := h.mapRegion(mf.Region)
	image := h.mapImage(mf.Image)

	fmt.Printf("   🌍 Creating %s in %s (image: %s)...\n", serverType, location, image)

	createBody := map[string]interface{}{
		"name":               mf.Name,
		"server_type":        serverType,
		"location":           location,
		"image":              image,
		"ssh_keys":           []int64{sshKeyID},
		"start_after_create": true,
	}

	createResp, status, err := h.hetznerRequest("POST", "/servers", createBody)
	if err != nil {
		return err
	}
	if status != 201 {
		return fmt.Errorf("creating server: HTTP %d: %s", status, string(createResp))
	}

	var createResult struct {
		Server struct {
			ID        int64 `json:"id"`
			PublicNet struct {
				IPv4 struct {
					IP string `json:"ip"`
				} `json:"ipv4"`
			} `json:"public_net"`
		} `json:"server"`
	}
	if err := json.Unmarshal(createResp, &createResult); err != nil {
		return err
	}

	ip := createResult.Server.PublicNet.IPv4.IP
	fmt.Printf("   📡 Server created (ID: %d, IP: %s)\n", createResult.Server.ID, ip)

	// Wait for SSH
	fmt.Printf("   ⏳ Waiting for SSH...\n")
	for i := 0; i < 60; i++ {
		time.Sleep(3 * time.Second)
		check := exec.Command("ssh", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
			"-o", "ConnectTimeout=3", "-o", "BatchMode=yes", fmt.Sprintf("root@%s", ip), "echo ok")
		if output, err := check.Output(); err == nil && strings.TrimSpace(string(output)) == "ok" {
			fmt.Printf("   ✅ Server ready at %s\n", ip)
			return nil
		}
	}

	return fmt.Errorf("timed out waiting for SSH on %s", ip)
}

// getServerIP returns the IP and ID of the server
func (h *HetznerAdapter) getServerIP(name string) (string, int64, error) {
	resp, _, err := h.hetznerRequest("GET", "/servers?name="+name, nil)
	if err != nil {
		return "", 0, err
	}
	var result struct {
		Servers []struct {
			ID        int64 `json:"id"`
			PublicNet struct {
				IPv4 struct {
					IP string `json:"ip"`
				} `json:"ipv4"`
			} `json:"public_net"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return "", 0, err
	}
	if len(result.Servers) == 0 {
		return "", 0, fmt.Errorf("server '%s' not found", name)
	}
	return result.Servers[0].PublicNet.IPv4.IP, result.Servers[0].ID, nil
}

// execInVM runs a command on the server via SSH
func (h *HetznerAdapter) execInVM(ip, command string) error {
	cmd := exec.Command("ssh", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		fmt.Sprintf("root@%s", ip), command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Provision runs setup steps on the server
func (h *HetznerAdapter) Provision(mf *machinefile.Machinefile) error {
	ip, _, err := h.getServerIP(mf.Name)
	if err != nil {
		return err
	}

	for _, step := range mf.Setup {
		if step.Install != "" {
			fmt.Printf("   📦 Installing %s...\n", step.Install)
			cmd := fmt.Sprintf("while fuser /var/lib/dpkg/lock-frontend /var/lib/apt/lists/lock /var/cache/apt/archives/lock >/dev/null 2>&1; do echo '   ⏳ Waiting for apt lock...'; sleep 3; done && apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y %s", step.Install)
			if err := h.execInVM(ip, cmd); err != nil {
				return fmt.Errorf("installing %s: %w", step.Install, err)
			}
		}
		if step.Clone != nil {
			fmt.Printf("   📥 Cloning %s...\n", step.Clone.Repo)
			cmd := fmt.Sprintf("git clone %s %s", step.Clone.Repo, step.Clone.Dest)
			if err := h.execInVM(ip, cmd); err != nil {
				return fmt.Errorf("cloning %s: %w", step.Clone.Repo, err)
			}
		}
		if step.Cmd != "" {
			fmt.Printf("   ▶️  Running: %s\n", step.Cmd)
			if err := h.execInVM(ip, step.Cmd); err != nil {
				return fmt.Errorf("running cmd: %w", err)
			}
		}
	}
	return nil
}

// Run executes run commands
func (h *HetznerAdapter) Run(mf *machinefile.Machinefile) error {
	ip, _, err := h.getServerIP(mf.Name)
	if err != nil {
		return err
	}
	for _, step := range mf.Run {
		if step.Cmd != "" {
			fmt.Printf("   ▶️  %s\n", step.Cmd)
			if err := h.execInVM(ip, step.Cmd); err != nil {
				return err
			}
		}
	}
	return nil
}

// Stop powers off the server (keeps it allocated — still billed)
func (h *HetznerAdapter) Stop(mf *machinefile.Machinefile) error {
	_, id, err := h.getServerIP(mf.Name)
	if err != nil {
		return err
	}
	_, _, err = h.hetznerRequest("POST", fmt.Sprintf("/servers/%d/actions/shutdown", id), nil)
	return err
}

// Destroy deletes the server completely (stops billing)
func (h *HetznerAdapter) Destroy(mf *machinefile.Machinefile) error {
	_, id, err := h.getServerIP(mf.Name)
	if err != nil {
		return err
	}
	_, status, err := h.hetznerRequest("DELETE", fmt.Sprintf("/servers/%d", id), nil)
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("deleting server: HTTP %d", status)
	}
	return nil
}

// Connect opens an SSH session to the server
func (h *HetznerAdapter) Connect(mf *machinefile.Machinefile) error {
	ip, _, err := h.getServerIP(mf.Name)
	if err != nil {
		return err
	}
	cmd := exec.Command("ssh", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		fmt.Sprintf("root@%s", ip))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Status shows the server status
func (h *HetznerAdapter) Status(mf *machinefile.Machinefile) error {
	resp, _, err := h.hetznerRequest("GET", "/servers?name="+mf.Name, nil)
	if err != nil {
		return err
	}
	var result struct {
		Servers []struct {
			ID         int64  `json:"id"`
			Name       string `json:"name"`
			Status     string `json:"status"`
			ServerType struct {
				Cores  int     `json:"cores"`
				Memory float64 `json:"memory"`
				Disk   int     `json:"disk"`
				Name   string  `json:"name"`
			} `json:"server_type"`
			Datacenter struct {
				Name string `json:"name"`
			} `json:"datacenter"`
			PublicNet struct {
				IPv4 struct {
					IP string `json:"ip"`
				} `json:"ipv4"`
			} `json:"public_net"`
		} `json:"servers"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return err
	}
	if len(result.Servers) == 0 {
		return fmt.Errorf("server '%s' not found", mf.Name)
	}

	srv := result.Servers[0]
	icon := "⚪"
	switch srv.Status {
	case "running":
		icon = "🟢"
	case "off":
		icon = "🔴"
	case "initializing":
		icon = "🟡"
	}

	fmt.Printf("%s  Machine: %s\n", icon, srv.Name)
	fmt.Printf("   Provider: Hetzner Cloud\n")
	fmt.Printf("   Status:   %s\n", srv.Status)
	fmt.Printf("   Type:     %s\n", srv.ServerType.Name)
	fmt.Printf("   Location: %s\n", srv.Datacenter.Name)
	fmt.Printf("   IP:       %s\n", srv.PublicNet.IPv4.IP)
	fmt.Printf("   CPUs:     %d\n", srv.ServerType.Cores)
	fmt.Printf("   Memory:   %.0f GB\n", srv.ServerType.Memory)
	fmt.Printf("   Disk:     %d GB\n", srv.ServerType.Disk)
	return nil
}

// parseMemoryMB is defined in tart.go (shared across adapters)
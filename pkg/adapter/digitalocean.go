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

const doAPI = "https://api.digitalocean.com/v2"

// DigitalOceanAdapter implements Adapter for DigitalOcean droplets
type DigitalOceanAdapter struct {
	token string
}

// NewDigitalOceanAdapter creates a new DigitalOcean adapter
func NewDigitalOceanAdapter() *DigitalOceanAdapter {
	token := os.Getenv("DIGITALOCEAN_TOKEN")
	if token == "" {
		token = os.Getenv("DIGITAL_OCEAN_PAT")
	}
	return &DigitalOceanAdapter{token: token}
}

// doRequest makes an authenticated request to the DigitalOcean API
func (d *DigitalOceanAdapter) doRequest(method, path string, body interface{}) ([]byte, int, error) {
	if d.token == "" {
		return nil, 0, fmt.Errorf("DIGITALOCEAN_TOKEN not set (add it to .env or export it)")
	}

	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reqBody = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, doAPI+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+d.token)
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

// mapSize maps resources to a DO droplet size
func (d *DigitalOceanAdapter) mapSize(mf *machinefile.Machinefile) string {
	cpu := mf.Resources.CPU
	mem := parseMemoryMB(mf.Resources.Memory) / 1024

	if cpu <= 1 && mem <= 1 {
		return "s-1vcpu-1gb" // $6/mo
	} else if cpu <= 1 && mem <= 2 {
		return "s-1vcpu-2gb" // $12/mo
	} else if cpu <= 2 && mem <= 4 {
		return "s-2vcpu-4gb" // $24/mo
	} else if cpu <= 4 && mem <= 8 {
		return "s-4vcpu-8gb" // $48/mo
	} else if cpu <= 8 && mem <= 16 {
		return "s-8vcpu-16gb" // $96/mo
	}
	return "s-2vcpu-4gb"
}

// mapRegion maps a region string to a DO region slug
func (d *DigitalOceanAdapter) mapRegion(region string) string {
	switch strings.ToLower(region) {
	case "us", "us-east", "nyc", "new-york":
		return "nyc3"
	case "us-west", "sf", "san-francisco":
		return "sfo3"
	case "eu", "eu-west", "london", "uk":
		return "lon1"
	case "eu-central", "amsterdam", "nl":
		return "ams3"
	case "eu-north", "germany", "de", "frankfurt":
		return "fra1"
	case "asia", "singapore", "sg":
		return "sgp1"
	default:
		if region != "" {
			return region
		}
		return "nyc3"
	}
}

// mapImage maps an image string to a DO image slug
func (d *DigitalOceanAdapter) mapImage(image string) string {
	switch strings.ToLower(image) {
	case "", "ubuntu":
		return "ubuntu-24-04-x64"
	case "debian":
		return "debian-12-x64"
	case "fedora":
		return "fedora-41-x64"
	case "centos":
		return "centos-stream-9-x64"
	default:
		return image
	}
}

// getSSHKeyFingerprint uploads or finds the SSH key on DO
func (d *DigitalOceanAdapter) getSSHKeyFingerprint(mf *machinefile.Machinefile) (string, error) {
	if mf.SSHKey == "" {
		return "", fmt.Errorf("no SSH key found — create one with: ssh-keygen -t ed25519")
	}

	pubKeyData, err := os.ReadFile(mf.SSHKey)
	if err != nil {
		return "", fmt.Errorf("reading SSH key %s: %w", mf.SSHKey, err)
	}
	pubKey := strings.TrimSpace(string(pubKeyData))
	keyName := fmt.Sprintf("mach-%s", mf.Name)

	// Try to create (will fail with 422 if already exists, that's fine)
	createResp, status, err := d.doRequest("POST", "/account/keys", map[string]string{
		"name":       keyName,
		"public_key": pubKey,
	})
	if err != nil {
		return "", err
	}



	var keyResult struct {
		SSHKey struct {
			Fingerprint string `json:"fingerprint"`
		} `json:"ssh_key"`
	}

	if status == 201 {
		if err := json.Unmarshal(createResp, &keyResult); err != nil {
			return "", err
		}
		return keyResult.SSHKey.Fingerprint, nil
	}

	// Key might already exist — list and find by name
	listResp, _, err := d.doRequest("GET", "/account/keys?per_page=200", nil)
	if err != nil {
		return "", err
	}
	var listResult struct {
		SSHKeys []struct {
			Name        string `json:"name"`
			Fingerprint string `json:"fingerprint"`
		} `json:"ssh_keys"`
	}
	if err := json.Unmarshal(listResp, &listResult); err != nil {
		return "", err
	}
	for _, k := range listResult.SSHKeys {
		if k.Name == keyName {
			return k.Fingerprint, nil
		}
	}
	return "", fmt.Errorf("failed to create or find SSH key: HTTP %d: %s", status, string(createResp))
}

// getDroplet returns the droplet info by name
func (d *DigitalOceanAdapter) getDroplet(name string) (int64, string, string, error) {
	resp, _, err := d.doRequest("GET", "/droplets?tag_name=mach", nil)
	if err != nil {
		return 0, "", "", err
	}
	var result struct {
		Droplets []struct {
			ID       int64  `json:"id"`
			Name     string `json:"name"`
			Status   string `json:"status"`
			Networks struct {
				V4 []struct {
					IPAddress string `json:"ip_address"`
					Type      string `json:"type"`
				} `json:"v4"`
			} `json:"networks"`
		} `json:"droplets"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return 0, "", "", err
	}
	for _, dr := range result.Droplets {
		if dr.Name == name {
			ip := ""
			for _, net := range dr.Networks.V4 {
				if net.Type == "public" {
					ip = net.IPAddress
					break
				}
			}
			return dr.ID, ip, dr.Status, nil
		}
	}
	return 0, "", "", fmt.Errorf("droplet '%s' not found", name)
}

// execInVM runs a command on the droplet via SSH
func (d *DigitalOceanAdapter) execInVM(ip, command string) error {
	cmd := exec.Command("ssh", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		fmt.Sprintf("root@%s", ip), command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Create creates a DigitalOcean droplet
func (d *DigitalOceanAdapter) Create(mf *machinefile.Machinefile) error {
	// Check if droplet already exists
	id, _, dropletStatus, err := d.getDroplet(mf.Name)
	if err == nil {
		if dropletStatus == "active" {
			fmt.Printf("   Droplet '%s' already running\n", mf.Name)
			return nil
		}
		fmt.Printf("   Droplet '%s' exists (status: %s), powering on...\n", mf.Name, dropletStatus)
		_, _, err := d.doRequest("POST", fmt.Sprintf("/droplets/%d/actions", id), map[string]string{
			"type": "power_on",
		})
		return err
	}

	// Get SSH key
	fmt.Printf("   🔑 Setting up SSH key...\n")
	fingerprint, err := d.getSSHKeyFingerprint(mf)
	if err != nil {
		return err
	}

	size := d.mapSize(mf)
	region := d.mapRegion(mf.Region)
	image := d.mapImage(mf.Image)

	fmt.Printf("   🌊 Creating %s in %s (image: %s)...\n", size, region, image)

	createBody := map[string]interface{}{
		"name":     mf.Name,
		"region":   region,
		"size":     size,
		"image":    image,
		"ssh_keys": []string{fingerprint},
		"tags":     []string{"mach"},
	}

	createResp, respStatus, err := d.doRequest("POST", "/droplets", createBody)
	if err != nil {
		return err
	}
	if respStatus != 202 {
		return fmt.Errorf("creating droplet: HTTP %d: %s", respStatus, string(createResp))
	}

	var createResult struct {
		Droplet struct {
			ID int64 `json:"id"`
		} `json:"droplet"`
	}
	if err := json.Unmarshal(createResp, &createResult); err != nil {
		return err
	}

	fmt.Printf("   📡 Droplet created (ID: %d)\n", createResult.Droplet.ID)
	fmt.Printf("   ⏳ Waiting for IP and SSH...\n")

	for i := 0; i < 60; i++ {
		time.Sleep(3 * time.Second)
		_, ip, st, err := d.getDroplet(mf.Name)
		if err != nil || ip == "" || st != "active" {
			continue
		}
		check := exec.Command("ssh", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
			"-o", "ConnectTimeout=3", "-o", "BatchMode=yes", fmt.Sprintf("root@%s", ip), "echo ok")
		if output, err := check.Output(); err == nil && strings.TrimSpace(string(output)) == "ok" {
			fmt.Printf("   ✅ Droplet ready at %s\n", ip)
			return nil
		}
	}

	return fmt.Errorf("timed out waiting for droplet")
}

// Provision runs setup steps on the droplet
func (d *DigitalOceanAdapter) Provision(mf *machinefile.Machinefile) error {
	_, ip, _, err := d.getDroplet(mf.Name)
	if err != nil {
		return err
	}
	for _, step := range mf.Setup {
		if step.Install != "" {
			fmt.Printf("   📦 Installing %s...\n", step.Install)
			cmd := fmt.Sprintf("while fuser /var/lib/dpkg/lock-frontend /var/lib/apt/lists/lock /var/cache/apt/archives/lock >/dev/null 2>&1; do echo '   ⏳ Waiting for apt lock...'; sleep 3; done && apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y %s", step.Install)
			if err := d.execInVM(ip, cmd); err != nil {
				return fmt.Errorf("installing %s: %w", step.Install, err)
			}
		}
		if step.Clone != nil {
			fmt.Printf("   📥 Cloning %s...\n", step.Clone.Repo)
			cmd := fmt.Sprintf("git clone %s %s", step.Clone.Repo, step.Clone.Dest)
			if err := d.execInVM(ip, cmd); err != nil {
				return fmt.Errorf("cloning %s: %w", step.Clone.Repo, err)
			}
		}
		if step.Cmd != "" {
			fmt.Printf("   ▶️  Running: %s\n", step.Cmd)
			if err := d.execInVM(ip, step.Cmd); err != nil {
				return fmt.Errorf("running cmd: %w", err)
			}
		}
	}
	return nil
}

// Run executes run commands
func (d *DigitalOceanAdapter) Run(mf *machinefile.Machinefile) error {
	_, ip, _, err := d.getDroplet(mf.Name)
	if err != nil {
		return err
	}
	for _, step := range mf.Run {
		if step.Cmd != "" {
			fmt.Printf("   ▶️  %s\n", step.Cmd)
			if err := d.execInVM(ip, step.Cmd); err != nil {
				return err
			}
		}
	}
	return nil
}

// Stop powers off the droplet (still billed)
func (d *DigitalOceanAdapter) Stop(mf *machinefile.Machinefile) error {
	id, _, _, err := d.getDroplet(mf.Name)
	if err != nil {
		return err
	}
	_, _, err = d.doRequest("POST", fmt.Sprintf("/droplets/%d/actions", id), map[string]string{
		"type": "shutdown",
	})
	return err
}

// Destroy deletes the droplet (stops billing)
func (d *DigitalOceanAdapter) Destroy(mf *machinefile.Machinefile) error {
	id, _, _, err := d.getDroplet(mf.Name)
	if err != nil {
		return err
	}
	_, status, err := d.doRequest("DELETE", fmt.Sprintf("/droplets/%d", id), nil)
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("deleting droplet: HTTP %d", status)
	}
	return nil
}

// Connect opens an SSH session to the droplet
func (d *DigitalOceanAdapter) Connect(mf *machinefile.Machinefile) error {
	_, ip, _, err := d.getDroplet(mf.Name)
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

// Status shows the droplet status
func (d *DigitalOceanAdapter) Status(mf *machinefile.Machinefile) error {
	resp, _, err := d.doRequest("GET", "/droplets?tag_name=mach", nil)
	if err != nil {
		return err
	}
	var result struct {
		Droplets []struct {
			ID     int64  `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
			Size   struct {
				Slug   string  `json:"slug"`
				VCPUs  int     `json:"vcpus"`
				Memory int     `json:"memory"`
				Disk   int     `json:"disk"`
				PriceMonthly string `json:"price_monthly"`
			} `json:"size"`
			Region struct {
				Slug string `json:"slug"`
				Name string `json:"name"`
			} `json:"region"`
			Networks struct {
				V4 []struct {
					IPAddress string `json:"ip_address"`
					Type      string `json:"type"`
				} `json:"v4"`
			} `json:"networks"`
		} `json:"droplets"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return err
	}

	for _, dr := range result.Droplets {
		if dr.Name == mf.Name {
			icon := "⚪"
			switch dr.Status {
			case "active":
				icon = "🟢"
			case "off":
				icon = "🔴"
			case "new":
				icon = "🟡"
			}
			ip := ""
			for _, net := range dr.Networks.V4 {
				if net.Type == "public" {
					ip = net.IPAddress
				}
			}
			fmt.Printf("%s  Machine: %s\n", icon, dr.Name)
			fmt.Printf("   Provider: DigitalOcean\n")
			fmt.Printf("   Status:   %s\n", dr.Status)
			fmt.Printf("   Size:     %s\n", dr.Size.Slug)
			fmt.Printf("   Region:   %s (%s)\n", dr.Region.Name, dr.Region.Slug)
			fmt.Printf("   IP:       %s\n", ip)
			fmt.Printf("   CPUs:     %d\n", dr.Size.VCPUs)
			fmt.Printf("   Memory:   %d MB\n", dr.Size.Memory)
			fmt.Printf("   Disk:     %d GB\n", dr.Size.Disk)
			return nil
		}
	}
	return fmt.Errorf("droplet '%s' not found", mf.Name)
}
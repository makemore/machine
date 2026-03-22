package adapter

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/makemore/machine/pkg/machinefile"
)

// GCPAdapter implements Adapter for Google Cloud Compute Engine
type GCPAdapter struct{}

// NewGCPAdapter creates a new GCP adapter
func NewGCPAdapter() *GCPAdapter {
	return &GCPAdapter{}
}

// gcloud runs a gcloud command and returns the output
func (g *GCPAdapter) gcloud(args ...string) (string, error) {
	cmd := exec.Command("gcloud", args...)
	output, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(output)), err
}

// mapMachineType maps resources to a GCE machine type
func (g *GCPAdapter) mapMachineType(mf *machinefile.Machinefile) string {
	cpu := mf.Resources.CPU
	mem := parseMemoryMB(mf.Resources.Memory) / 1024

	// Use t2a (ARM) series for cost efficiency
	if cpu <= 1 && mem <= 4 {
		return "t2a-standard-1" // 1 vCPU, 4GB
	} else if cpu <= 2 && mem <= 8 {
		return "t2a-standard-2" // 2 vCPU, 8GB
	} else if cpu <= 4 && mem <= 16 {
		return "t2a-standard-4" // 4 vCPU, 16GB
	} else if cpu <= 8 && mem <= 32 {
		return "t2a-standard-8" // 8 vCPU, 32GB
	}
	return "t2a-standard-1"
}

// mapZone maps a region string to a GCE zone
func (g *GCPAdapter) mapZone(region string) string {
	switch strings.ToLower(region) {
	case "us", "us-central":
		return "us-central1-a"
	case "us-east":
		return "us-east1-b"
	case "us-west":
		return "us-west1-a"
	case "eu", "eu-west", "europe":
		return "europe-west1-b"
	case "eu-central", "germany", "de":
		return "europe-west3-a"
	case "uk", "london":
		return "europe-west2-a"
	case "asia", "asia-east":
		return "asia-east1-a"
	case "asia-southeast", "singapore", "sg":
		return "asia-southeast1-a"
	default:
		if region != "" {
			return region
		}
		return "us-central1-a"
	}
}

// mapImage maps an image string to a GCE image family
func (g *GCPAdapter) mapImage(image string) string {
	switch strings.ToLower(image) {
	case "", "ubuntu":
		return "projects/ubuntu-os-cloud/global/images/family/ubuntu-2404-lts-arm64"
	case "debian":
		return "projects/debian-cloud/global/images/family/debian-12-arm64"
	case "fedora":
		return "projects/fedora-cloud/global/images/family/fedora-cloud-41-arm64"
	default:
		return image
	}
}

// Create creates a GCE instance
func (g *GCPAdapter) Create(mf *machinefile.Machinefile) error {
	// Check if instance already exists
	output, err := g.gcloud("compute", "instances", "describe", mf.Name,
		"--zone", g.mapZone(mf.Region), "--format=json")
	if err == nil {
		var instance struct {
			Status string `json:"status"`
		}
		if json.Unmarshal([]byte(output), &instance) == nil {
			if instance.Status == "RUNNING" {
				fmt.Printf("   Instance '%s' already running\n", mf.Name)
				return nil
			}
			fmt.Printf("   Instance '%s' exists (status: %s), starting...\n", mf.Name, instance.Status)
			_, err := g.gcloud("compute", "instances", "start", mf.Name,
				"--zone", g.mapZone(mf.Region))
			return err
		}
	}

	machineType := g.mapMachineType(mf)
	zone := g.mapZone(mf.Region)
	image := g.mapImage(mf.Image)

	fmt.Printf("   ☁️  Creating %s in %s (image: %s)...\n", machineType, zone, image)

	args := []string{
		"compute", "instances", "create", mf.Name,
		"--zone", zone,
		"--machine-type", machineType,
		"--image", image,
		"--boot-disk-size", "20GB",
		"--tags", "mach",
		"--format", "json",
	}

	output, err = g.gcloud(args...)
	if err != nil {
		return fmt.Errorf("creating instance: %s", output)
	}

	fmt.Printf("   ⏳ Waiting for SSH...\n")
	for i := 0; i < 60; i++ {
		time.Sleep(3 * time.Second)
		_, err := g.gcloud("compute", "ssh", mf.Name,
			"--zone", zone, "--command", "echo ok",
			"--ssh-flag=-o", "--ssh-flag=ConnectTimeout=3")
		if err == nil {
			fmt.Printf("   ✅ Instance ready\n")
			return nil
		}
	}
	return fmt.Errorf("timed out waiting for SSH")
}



// execInVM runs a command on the instance via gcloud ssh
func (g *GCPAdapter) execInVM(name, zone, command string) error {
	cmd := exec.Command("gcloud", "compute", "ssh", name,
		"--zone", zone, "--command", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Provision runs setup steps on the instance
func (g *GCPAdapter) Provision(mf *machinefile.Machinefile) error {
	zone := g.mapZone(mf.Region)
	for _, step := range mf.Setup {
		if step.Install != "" {
			fmt.Printf("   📦 Installing %s...\n", step.Install)
			cmd := fmt.Sprintf("while sudo fuser /var/lib/dpkg/lock-frontend /var/lib/apt/lists/lock /var/cache/apt/archives/lock >/dev/null 2>&1; do sleep 3; done && sudo apt-get update -qq && sudo DEBIAN_FRONTEND=noninteractive apt-get install -y %s", step.Install)
			if err := g.execInVM(mf.Name, zone, cmd); err != nil {
				return fmt.Errorf("installing %s: %w", step.Install, err)
			}
		}
		if step.Clone != nil {
			fmt.Printf("   📥 Cloning %s...\n", step.Clone.Repo)
			cmd := fmt.Sprintf("git clone %s %s", step.Clone.Repo, step.Clone.Dest)
			if err := g.execInVM(mf.Name, zone, cmd); err != nil {
				return fmt.Errorf("cloning %s: %w", step.Clone.Repo, err)
			}
		}
		if step.Cmd != "" {
			fmt.Printf("   ▶️  Running: %s\n", step.Cmd)
			if err := g.execInVM(mf.Name, zone, step.Cmd); err != nil {
				return fmt.Errorf("running cmd: %w", err)
			}
		}
	}
	return nil
}

// Run executes run commands
func (g *GCPAdapter) Run(mf *machinefile.Machinefile) error {
	zone := g.mapZone(mf.Region)
	for _, step := range mf.Run {
		if step.Cmd != "" {
			fmt.Printf("   ▶️  %s\n", step.Cmd)
			if err := g.execInVM(mf.Name, zone, step.Cmd); err != nil {
				return err
			}
		}
	}
	return nil
}

// Stop stops the instance
func (g *GCPAdapter) Stop(mf *machinefile.Machinefile) error {
	_, err := g.gcloud("compute", "instances", "stop", mf.Name,
		"--zone", g.mapZone(mf.Region))
	return err
}

// Destroy deletes the instance
func (g *GCPAdapter) Destroy(mf *machinefile.Machinefile) error {
	_, err := g.gcloud("compute", "instances", "delete", mf.Name,
		"--zone", g.mapZone(mf.Region), "--quiet")
	return err
}

// Connect opens an SSH session
func (g *GCPAdapter) Connect(mf *machinefile.Machinefile) error {
	cmd := exec.Command("gcloud", "compute", "ssh", mf.Name,
		"--zone", g.mapZone(mf.Region))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Status shows the instance status
func (g *GCPAdapter) Status(mf *machinefile.Machinefile) error {
	zone := g.mapZone(mf.Region)
	output, err := g.gcloud("compute", "instances", "describe", mf.Name,
		"--zone", zone, "--format=json")
	if err != nil {
		return fmt.Errorf("instance '%s' not found", mf.Name)
	}

	var instance struct {
		Name              string `json:"name"`
		Status            string `json:"status"`
		MachineType       string `json:"machineType"`
		NetworkInterfaces []struct {
			AccessConfigs []struct {
				NatIP string `json:"natIP"`
			} `json:"accessConfigs"`
		} `json:"networkInterfaces"`
	}
	if err := json.Unmarshal([]byte(output), &instance); err != nil {
		return err
	}

	icon := "⚪"
	switch instance.Status {
	case "RUNNING":
		icon = "🟢"
	case "TERMINATED", "STOPPED":
		icon = "🔴"
	case "STAGING", "PROVISIONING":
		icon = "🟡"
	}

	ip := ""
	if len(instance.NetworkInterfaces) > 0 && len(instance.NetworkInterfaces[0].AccessConfigs) > 0 {
		ip = instance.NetworkInterfaces[0].AccessConfigs[0].NatIP
	}

	mtParts := strings.Split(instance.MachineType, "/")
	mt := mtParts[len(mtParts)-1]

	fmt.Printf("%s  Machine: %s\n", icon, instance.Name)
	fmt.Printf("   Provider: Google Cloud\n")
	fmt.Printf("   Status:   %s\n", instance.Status)
	fmt.Printf("   Type:     %s\n", mt)
	fmt.Printf("   Zone:     %s\n", zone)
	if ip != "" {
		fmt.Printf("   IP:       %s\n", ip)
	}
	return nil
}
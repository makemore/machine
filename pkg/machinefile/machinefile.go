package machinefile

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Machinefile represents the parsed Machinefile
type Machinefile struct {
	Name           string    `yaml:"name"`
	OS             string    `yaml:"os"`
	Image          string    `yaml:"image,omitempty"`
	PackageManager string    `yaml:"packageManager,omitempty"`
	Provider       string    `yaml:"provider,omitempty"`       // local (default), hetzner, digitalocean, gcp
	Region         string    `yaml:"region,omitempty"`         // cloud region/datacenter
	SSHKey         string            `yaml:"sshKey,omitempty"`         // path to SSH public key for cloud VMs
	SSHKeys        []SSHKeySpec      `yaml:"ssh_keys,omitempty"`       // multiple SSH keys with usernames
	CloudInit      string            `yaml:"cloud_init,omitempty"`     // path to cloud-init/user-data file
	Env            map[string]string `yaml:"env,omitempty"`            // environment variables to inject
	Resources      Resources         `yaml:"resources"`
	Setup          []Step            `yaml:"setup"`
	Run            []Step            `yaml:"run"`
	Expose         []int             `yaml:"expose"`
}

// Resources defines CPU and memory allocation
type Resources struct {
	CPU    int    `yaml:"cpu"`
	Memory string `yaml:"memory"`
}

// SSHKeySpec represents an SSH key for multi-user VMs
type SSHKeySpec struct {
	Username  string `yaml:"username"`
	PublicKey string `yaml:"public_key"`
}

// Step represents a setup or run step
// Can be: install, clone, cmd, or script
type Step struct {
	Install string      `yaml:"install,omitempty"`
	Clone   *CloneSpec  `yaml:"clone,omitempty"`
	Cmd     string      `yaml:"cmd,omitempty"`
	Script  string      `yaml:"script,omitempty"`   // path to a local script to run on VM
}

// CloneSpec represents git clone parameters
type CloneSpec struct {
	Repo string `yaml:"repo"`
	Dest string `yaml:"dest"`
}

// Load parses a Machinefile from the given path
func Load(path string) (*Machinefile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	var mf Machinefile
	if err := yaml.Unmarshal(data, &mf); err != nil {
		return nil, fmt.Errorf("parsing yaml: %w", err)
	}

	// Validation
	if mf.Name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if mf.OS == "" {
		return nil, fmt.Errorf("os is required")
	}

	// Defaults
	if mf.Resources.CPU == 0 {
		mf.Resources.CPU = 2
	}
	if mf.Resources.Memory == "" {
		mf.Resources.Memory = "4gb"
	}
	if mf.PackageManager == "" {
		mf.PackageManager = "apt"
	}
	if mf.Provider == "" {
		mf.Provider = "local"
	}
	if mf.SSHKey == "" {
		home, _ := os.UserHomeDir()
		defaultKey := home + "/.ssh/id_ed25519.pub"
		if _, err := os.Stat(defaultKey); err == nil {
			mf.SSHKey = defaultKey
		} else {
			rsaKey := home + "/.ssh/id_rsa.pub"
			if _, err := os.Stat(rsaKey); err == nil {
				mf.SSHKey = rsaKey
			}
		}
	}

	return &mf, nil
}

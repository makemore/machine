package machinefile

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Machinefile represents the parsed Machinefile
type Machinefile struct {
	Name           string            `yaml:"name"`
	OS             string            `yaml:"os"`
	Image          string            `yaml:"image,omitempty"`
	PackageManager string            `yaml:"packageManager,omitempty"`
	Provider       string            `yaml:"provider,omitempty"`
	Region         string            `yaml:"region,omitempty"`
	Harden         bool              `yaml:"harden,omitempty"`
	Swap           string            `yaml:"swap,omitempty"`
	MOTD           string            `yaml:"motd,omitempty"`
	SSHKey         string            `yaml:"sshKey,omitempty"`
	SSHKeys        []SSHKeySpec      `yaml:"ssh_keys,omitempty"`
	Users          []UserSpec        `yaml:"users,omitempty"`
	CloudInit      string            `yaml:"cloud_init,omitempty"`
	Env            map[string]string `yaml:"env,omitempty"`
	Resources      Resources         `yaml:"resources"`
	Setup          []Step            `yaml:"setup"`
	Run            []Step            `yaml:"run"`
	Services       []ServiceSpec     `yaml:"services,omitempty"`
	Conduit        *ConduitSpec      `yaml:"conduit,omitempty"`
	ReverseProxy   *ReverseProxy     `yaml:"reverse_proxy,omitempty"`
	Expose         []int             `yaml:"expose"`
}

// ConduitSpec configures the conduitd agent on the VM.
// When present, mach automatically downloads, installs, and starts conduitd
// as a systemd service that connects back to the Conduit gateway.
type ConduitSpec struct {
	DeviceID   string `yaml:"device_id"`
	GatewayURL string `yaml:"gateway_url"`
	DownloadURL string `yaml:"download_url,omitempty"` // defaults to official release
}

// Resources defines CPU and memory allocation
type Resources struct {
	CPU    int    `yaml:"cpu"`
	Memory string `yaml:"memory"`
	Disk   string `yaml:"disk,omitempty"`
}

// SSHKeySpec represents an SSH key for multi-user VMs
type SSHKeySpec struct {
	Username  string `yaml:"username"`
	PublicKey string `yaml:"public_key"`
}

// UserSpec represents a full user account
type UserSpec struct {
	Username  string `yaml:"username"`
	PublicKey string `yaml:"public_key"`
	GithubPAT string `yaml:"github_pat,omitempty"`
	Sudo      *bool  `yaml:"sudo,omitempty"`
	Shell     string `yaml:"shell,omitempty"`
}

// ServiceSpec represents a systemd service
type ServiceSpec struct {
	Name    string            `yaml:"name"`
	Cmd     string            `yaml:"cmd"`
	Install string            `yaml:"install,omitempty"`
	Workdir string            `yaml:"workdir,omitempty"`
	User    string            `yaml:"user,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	Restart string            `yaml:"restart,omitempty"`
}

// ReverseProxy configures Caddy
type ReverseProxy struct {
	Domain string       `yaml:"domain,omitempty"`
	Routes []ProxyRoute `yaml:"routes"`
}

// ProxyRoute is a single reverse proxy route
type ProxyRoute struct {
	Path   string `yaml:"path"`
	Target string `yaml:"target"`
}

// Step represents a setup or run step
type Step struct {
	Install string     `yaml:"install,omitempty"`
	Clone   *CloneSpec `yaml:"clone,omitempty"`
	Cmd     string     `yaml:"cmd,omitempty"`
	Script  string     `yaml:"script,omitempty"`
}

// CloneSpec represents git clone parameters
type CloneSpec struct {
	Repo   string `yaml:"repo"`
	Dest   string `yaml:"dest"`
	Branch string `yaml:"branch,omitempty"`
	PATEnv string `yaml:"pat_env,omitempty"`
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
	// Default to local OS-based adapter when no provider specified.
	// CLI --provider flag and MACH_PROVIDER env var can override.
	if mf.Provider == "" {
		if envProvider := os.Getenv("MACH_PROVIDER"); envProvider != "" {
			mf.Provider = envProvider
		}
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

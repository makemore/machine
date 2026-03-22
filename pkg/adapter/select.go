package adapter

import (
	"fmt"

	"github.com/makemore/machine/pkg/machinefile"
)

// Select returns the appropriate adapter based on the Machinefile config
func Select(mf *machinefile.Machinefile) (Adapter, error) {
	// Cloud providers take priority over local OS-based adapters
	switch mf.Provider {
	case "hetzner":
		return NewHetznerAdapter(), nil
	case "digitalocean", "do":
		return NewDigitalOceanAdapter(), nil
	case "gcp", "google":
		return NewGCPAdapter(), nil
	case "local", "":
		// Fall through to OS-based selection
	default:
		return nil, fmt.Errorf("unsupported provider: %s", mf.Provider)
	}

	// Local adapters based on OS
	switch mf.OS {
	case "linux":
		return NewLimaAdapter(), nil
	case "macos":
		return NewTartAdapter(), nil
	case "windows":
		return NewQemuAdapter(), nil
	default:
		return nil, fmt.Errorf("unsupported OS: %s", mf.OS)
	}
}


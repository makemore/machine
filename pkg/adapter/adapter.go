package adapter

import "github.com/makemore/machine/pkg/machinefile"

// MachineInfo is the structured data returned by Status and Create
type MachineInfo struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Provider string `json:"provider"`
	IP       string `json:"ip,omitempty"`
	Region   string `json:"region,omitempty"`
	CPUs     int    `json:"cpus,omitempty"`
	MemoryMB int    `json:"memory_mb,omitempty"`
	DiskGB   int    `json:"disk_gb,omitempty"`
	OS       string `json:"os,omitempty"`
	Type     string `json:"type,omitempty"` // instance type / server type
}

// SnapshotInfo describes an image/snapshot produced from a running VM.
// Returned by Adapter.Snapshot so callers (e.g. `mach snapshot`) can
// record the image id for later use as a Machinefile `image:` value.
type SnapshotInfo struct {
	ID          string `json:"id"`                     // provider image id (string form)
	Name        string `json:"name,omitempty"`         // human label
	Description string `json:"description,omitempty"`  // free-form
	Provider    string `json:"provider"`               // hetzner / gcp / digitalocean
	Region      string `json:"region,omitempty"`       // where the snapshot lives
	SizeGB      int    `json:"size_gb,omitempty"`      // disk size of the snapshot
	CreatedAt   string `json:"created_at,omitempty"`   // RFC3339
}

// Adapter defines the interface that all platform adapters must implement
type Adapter interface {
	// Create creates the VM
	Create(mf *machinefile.Machinefile) error

	// Provision runs setup steps (install, clone, cmd)
	Provision(mf *machinefile.Machinefile) error

	// Run executes run commands
	Run(mf *machinefile.Machinefile) error

	// Stop stops the VM
	Stop(mf *machinefile.Machinefile) error

	// Destroy deletes the VM
	Destroy(mf *machinefile.Machinefile) error

	// Connect opens interactive access (SSH/RDP/WinRM)
	Connect(mf *machinefile.Machinefile) error

	// Status returns the current status of the VM
	Status(mf *machinefile.Machinefile) error

	// Info returns structured machine info (for --json output)
	Info(mf *machinefile.Machinefile) (*MachineInfo, error)

	// Snapshot creates a provider-native image/snapshot of the VM identified
	// by mf.Name and returns its id. Adapters that do not yet support this
	// should return ErrSnapshotUnsupported.
	Snapshot(mf *machinefile.Machinefile, label string) (*SnapshotInfo, error)
}

// ErrSnapshotUnsupported is returned by adapters that have not yet
// implemented Snapshot. Callers should treat it as a soft failure.
var ErrSnapshotUnsupported = errSnapshotUnsupported{}

type errSnapshotUnsupported struct{}

func (errSnapshotUnsupported) Error() string {
	return "snapshot is not supported on this provider yet"
}


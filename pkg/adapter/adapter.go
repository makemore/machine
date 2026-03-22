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
}


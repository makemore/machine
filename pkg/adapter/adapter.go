package adapter

import "github.com/makemore/machine/pkg/machinefile"

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
}


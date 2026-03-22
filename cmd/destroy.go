package cmd

import (
	"fmt"
	"os"

	"github.com/makemore/machine/pkg/adapter"
	"github.com/makemore/machine/pkg/machinefile"
	"github.com/makemore/machine/pkg/state"
	"github.com/spf13/cobra"
)

var destroyCmd = &cobra.Command{
	Use:   "destroy [name]",
	Short: "Delete machine",
	Long: `Delete the machine and all its resources.

If a name is given, destroys that machine by looking it up across all adapters.
If no name is given, uses the Machinefile in the current directory.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 1 {
			destroyByName(args[0])
			return
		}

		mf, err := machinefile.Load(machineFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading Machinefile: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("🗑️  Destroying machine: %s\n", mf.Name)

		ad, err := adapter.Select(mf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}

		if err := ad.Destroy(mf); err != nil {
			fmt.Fprintf(os.Stderr, "Error destroying: %v\n", err)
			os.Exit(1)
		}

		// Clean up state file
		_, sfPath := state.LoadOrDefault(stateFile, mf.Name)
		_ = state.Remove(sfPath)

		fmt.Printf("✅ Machine '%s' destroyed\n", mf.Name)
	},
}

// destroyByName finds a machine across all adapters and destroys it
func destroyByName(name string) {
	fmt.Printf("🗑️  Destroying machine: %s\n", name)

	// Scan all adapters (same as `mach list`) to find which one owns this machine
	var found *listEntry
	for _, scanFn := range []func() []listEntry{listLimaVMs, listTartVMs, listQemuVMs, listHetznerVMs, listDigitalOceanVMs} {
		for _, e := range scanFn() {
			if e.Name == name {
				entry := e
				found = &entry
				break
			}
		}
		if found != nil {
			break
		}
	}

	if found == nil {
		fmt.Fprintf(os.Stderr, "Error: machine '%s' not found on any adapter\n", name)
		fmt.Fprintf(os.Stderr, "Run 'mach list' to see available machines.\n")
		os.Exit(1)
	}

	// Map the discovered OS back to an adapter
	mf := &machinefile.Machinefile{Name: name, OS: found.OS}
	switch found.OS {
	case "hetzner":
		mf.OS = "linux"
		mf.Provider = "hetzner"
	case "do":
		mf.OS = "linux"
		mf.Provider = "digitalocean"
	}

	ad, err := adapter.Select(mf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error selecting adapter: %v\n", err)
		os.Exit(1)
	}

	if err := ad.Destroy(mf); err != nil {
		fmt.Fprintf(os.Stderr, "Error destroying: %v\n", err)
		os.Exit(1)
	}

	// Clean up state file
	_, sfPath := state.LoadOrDefault(stateFile, name)
	_ = state.Remove(sfPath)

	fmt.Printf("✅ Machine '%s' destroyed\n", name)
}


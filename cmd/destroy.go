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
	Use:   "destroy",
	Short: "Delete machine",
	Long:  `Delete the machine and all its resources.`,
	Run: func(cmd *cobra.Command, args []string) {
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


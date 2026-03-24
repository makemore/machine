package cmd

import (
	"fmt"
	"os"

	"github.com/makemore/machine/pkg/adapter"
	"github.com/makemore/machine/pkg/machinefile"
	"github.com/spf13/cobra"
)

var provisionCmd = &cobra.Command{
	Use:   "provision",
	Short: "Re-provision a running machine",
	Long: `Re-run the provisioning pipeline on an already running machine.

This is useful for:
  - Adding or removing users after initial setup
  - Applying updated setup steps
  - Re-running the full provisioning without recreating the VM

The machine must already be running (use 'mach up' first).`,
	Run: func(cmd *cobra.Command, args []string) {
		mf, err := machinefile.Load(machineFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading Machinefile: %v\n", err)
			os.Exit(1)
		}

		// Select adapter
		ad, err := adapter.Select(mf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}

		// Verify machine is running
		info, err := ad.Info(mf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: machine '%s' not found. Run 'mach up' first.\n", mf.Name)
			os.Exit(1)
		}
		if info.Status != "running" {
			fmt.Fprintf(os.Stderr, "Error: machine '%s' is %s, not running. Start it first.\n", mf.Name, info.Status)
			os.Exit(1)
		}

		fmt.Printf("🔧 Re-provisioning machine: %s\n", mf.Name)
		if err := ad.Provision(mf); err != nil {
			fmt.Fprintf(os.Stderr, "Error provisioning: %v\n", err)
			os.Exit(1)
		}

		// Run commands if any
		if len(mf.Run) > 0 {
			fmt.Println("▶️  Running commands...")
			if err := ad.Run(mf); err != nil {
				fmt.Fprintf(os.Stderr, "Error running commands: %v\n", err)
				os.Exit(1)
			}
		}

		fmt.Printf("✅ Machine '%s' re-provisioned\n", mf.Name)
	},
}

func init() {
	rootCmd.AddCommand(provisionCmd)
}


package cmd

import (
	"fmt"
	"os"

	"github.com/makemore/machine/pkg/adapter"
	"github.com/makemore/machine/pkg/machinefile"
	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop machine",
	Long:  `Stop the machine defined in the Machinefile.`,
	Run: func(cmd *cobra.Command, args []string) {
		mf, err := machinefile.Load(machineFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading Machinefile: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("⏹️  Stopping machine: %s\n", mf.Name)

		ad, err := adapter.Select(mf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}

		if err := ad.Stop(mf); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✅ Machine '%s' stopped\n", mf.Name)
	},
}


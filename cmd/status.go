package cmd

import (
	"fmt"
	"os"

	"github.com/makemore/machine/pkg/adapter"
	"github.com/makemore/machine/pkg/machinefile"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show machine status",
	Long:  `Show the current status of the machine defined in the Machinefile.`,
	Run: func(cmd *cobra.Command, args []string) {
		mf, err := machinefile.Load(machineFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading Machinefile: %v\n", err)
			os.Exit(1)
		}

		ad, err := adapter.Select(mf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}

		if err := ad.Status(mf); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}


package cmd

import (
	"fmt"
	"os"

	"github.com/makemore/machine/pkg/adapter"
	"github.com/makemore/machine/pkg/machinefile"
	"github.com/spf13/cobra"
)

var sshCmd = &cobra.Command{
	Use:   "ssh",
	Short: "Connect to machine",
	Long:  `Open an SSH session to the machine.`,
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

		if err := ad.Connect(mf); err != nil {
			fmt.Fprintf(os.Stderr, "Error connecting: %v\n", err)
			os.Exit(1)
		}
	},
}


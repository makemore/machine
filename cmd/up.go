package cmd

import (
	"fmt"
	"os"

	"github.com/makemore/machine/pkg/adapter"
	"github.com/makemore/machine/pkg/machinefile"
	"github.com/spf13/cobra"
)

var upCmd = &cobra.Command{
	Use:   "up",
	Short: "Create and start machine",
	Long:  `Create and start a machine based on the Machinefile in the current directory.`,
	Run: func(cmd *cobra.Command, args []string) {
		// 1. Parse Machinefile
		mf, err := machinefile.Load(machineFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading Machinefile: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("🚀 Bringing up machine: %s (os: %s, image: %s)\n", mf.Name, mf.OS, mf.Image)

		// 2. Select adapter based on provider/OS
		ad, err := adapter.Select(mf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}

		// 3. Create VM
		fmt.Println("📦 Creating VM...")
		if err := ad.Create(mf); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating VM: %v\n", err)
			os.Exit(1)
		}

		// 4. Provision (setup steps)
		fmt.Println("🔧 Provisioning...")
		if err := ad.Provision(mf); err != nil {
			fmt.Fprintf(os.Stderr, "Error provisioning: %v\n", err)
			os.Exit(1)
		}

		// 5. Run commands
		if len(mf.Run) > 0 {
			fmt.Println("▶️  Running commands...")
			if err := ad.Run(mf); err != nil {
				fmt.Fprintf(os.Stderr, "Error running commands: %v\n", err)
				os.Exit(1)
			}
		}

		fmt.Printf("✅ Machine '%s' is up!\n", mf.Name)
		fmt.Printf("   Connect with: mach ssh\n")
	},
}

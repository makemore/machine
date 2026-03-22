package cmd

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var machineFile string

var rootCmd = &cobra.Command{
	Use:   "mach",
	Short: "Machine - declarative VM environments",
	Long:  `Machine (mach) is a CLI tool for defining and launching persistent compute environments (VMs) using a single declarative file: Machinefile.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Show help if no subcommand is given
		cmd.Help()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// Load .env file if present (won't override existing env vars)
	_ = godotenv.Load()

	rootCmd.PersistentFlags().StringVarP(&machineFile, "file", "f", "Machinefile", "Path to Machinefile")

	rootCmd.AddCommand(upCmd)
	rootCmd.AddCommand(downCmd)
	rootCmd.AddCommand(destroyCmd)
	rootCmd.AddCommand(sshCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(doctorCmd)
}

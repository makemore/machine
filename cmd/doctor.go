package cmd

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Validate environment",
	Long:  `Check that all required dependencies are installed.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("🩺 Checking environment...")

		checks := []struct {
			name    string
			command string
			args    []string
		}{
			{"lima (linux VMs)", "limactl", []string{"--version"}},
			{"tart (macOS VMs)", "tart", []string{"--version"}},
			{"qemu (Windows VMs)", "qemu-system-aarch64", []string{"--version"}},
			{"aria2 (Windows ISO download)", "aria2c", []string{"--version"}},
			{"wimlib (Windows ISO build)", "wimlib-imagex", []string{"--version"}},
			{"git", "git", []string{"--version"}},
			{"sshpass (VM SSH)", "sshpass", []string{"-V"}},
		}

		allOk := true
		for _, check := range checks {
			if _, err := exec.LookPath(check.command); err != nil {
				fmt.Printf("  ❌ %s: not found\n", check.name)
				allOk = false
			} else {
				fmt.Printf("  ✅ %s: installed\n", check.name)
			}
		}

		if allOk {
			fmt.Println("\n✅ All checks passed!")
		} else {
			fmt.Println("\n⚠️  Some dependencies are missing")
		}
	},
}


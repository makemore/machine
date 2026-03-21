package cmd

import (
	"fmt"
	"os"

	"github.com/makemore/machine/pkg/images"
	"github.com/spf13/cobra"
)

var imagesCmd = &cobra.Command{
	Use:   "images",
	Short: "Manage machine images",
	Long:  `List, update, and manage the machine images available for use.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update the image manifest from the remote repository",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("🔄 Updating image manifest...")
		if err := images.UpdateManifest(); err != nil {
			fmt.Fprintf(os.Stderr, "Error updating manifest: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Image manifest updated successfully.")
	},
}

func init() {
	imagesCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(imagesCmd)
}

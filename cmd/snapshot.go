package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/makemore/machine/pkg/adapter"
	"github.com/makemore/machine/pkg/machinefile"
	"github.com/spf13/cobra"
)

var (
	snapshotLabel    string
	snapshotManifest string
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Create a provider-native image of the running machine",
	Long: `Create a provider-native image (a.k.a. snapshot) of the machine defined
in the Machinefile. The image id is printed and, if --manifest is given,
written as JSON for downstream consumers (e.g. a Cloud Build job that
publishes the latest golden-image id).`,
	Run: func(cmd *cobra.Command, args []string) {
		mf, err := machinefile.Load(machineFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading Machinefile: %v\n", err)
			os.Exit(1)
		}
		if providerFlag != "" {
			mf.Provider = providerFlag
		}

		ad, err := adapter.Select(mf)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}

		if !jsonOutput {
			fmt.Printf("📸 Snapshotting machine: %s\n", mf.Name)
		}

		info, err := ad.Snapshot(mf, snapshotLabel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating snapshot: %v\n", err)
			os.Exit(1)
		}

		if snapshotManifest != "" {
			data, mErr := json.MarshalIndent(info, "", "  ")
			if mErr != nil {
				fmt.Fprintf(os.Stderr, "Error encoding manifest: %v\n", mErr)
				os.Exit(1)
			}
			if wErr := os.WriteFile(snapshotManifest, data, 0o644); wErr != nil {
				fmt.Fprintf(os.Stderr, "Error writing manifest %s: %v\n", snapshotManifest, wErr)
				os.Exit(1)
			}
			if !jsonOutput {
				fmt.Printf("   📝 Manifest written: %s\n", snapshotManifest)
			}
		}

		if jsonOutput {
			out, _ := json.Marshal(info)
			fmt.Println(string(out))
		} else {
			fmt.Printf("✅ Snapshot created: id=%s provider=%s size=%dGB\n",
				info.ID, info.Provider, info.SizeGB)
		}
	},
}

func init() {
	snapshotCmd.Flags().StringVar(&snapshotLabel, "label", "", "Description / label for the snapshot")
	snapshotCmd.Flags().StringVar(&snapshotManifest, "manifest", "", "Write SnapshotInfo JSON to this path")
}

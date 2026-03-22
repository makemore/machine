package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type listEntry struct {
	Icon   string
	Name   string
	Status string
	OS     string
	CPUs   string
	Memory string
	Disk   string
}

var listCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all machines",
	Long:    `List all machines and their current status across all adapters.`,
	Aliases: []string{"ls"},
	Run: func(cmd *cobra.Command, args []string) {
		var entries []listEntry

		// Lima VMs (Linux)
		entries = append(entries, listLimaVMs()...)
		// Tart VMs (macOS)
		entries = append(entries, listTartVMs()...)
		// QEMU VMs (Windows)
		entries = append(entries, listQemuVMs()...)
		// Hetzner Cloud
		entries = append(entries, listHetznerVMs()...)
		// DigitalOcean
		entries = append(entries, listDigitalOceanVMs()...)

		if len(entries) == 0 {
			fmt.Println("No machines found.")
			return
		}

		fmt.Printf("%-4s %-20s %-12s %-10s %-6s %-10s %-10s\n",
			"", "NAME", "STATUS", "OS", "CPUS", "MEMORY", "DISK")
		fmt.Println(strings.Repeat("─", 80))

		for _, e := range entries {
			fmt.Printf("%-4s %-20s %-12s %-10s %-6s %-10s %-10s\n",
				e.Icon, e.Name, e.Status, e.OS, e.CPUs, e.Memory, e.Disk)
		}
	},
}

func listLimaVMs() []listEntry {
	var entries []listEntry
	limaCmd := exec.Command("limactl", "list", "--format",
		"{{.Name}}\t{{.Status}}\t{{.Arch}}\t{{.CPUs}}\t{{.Memory}}\t{{.Disk}}")
	output, err := limaCmd.Output()
	if err != nil {
		return entries
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 6 {
			continue
		}
		icon := "⚪"
		switch parts[1] {
		case "Running":
			icon = "🟢"
		case "Stopped":
			icon = "🔴"
		}
		entries = append(entries, listEntry{
			Icon:   icon,
			Name:   parts[0],
			Status: parts[1],
			OS:     "linux",
			CPUs:   parts[3],
			Memory: humanizeListBytes(parts[4]),
			Disk:   humanizeListBytes(parts[5]),
		})
	}
	return entries
}

func listTartVMs() []listEntry {
	var entries []listEntry
	tartCmd := exec.Command("tart", "list", "--format", "json")
	output, err := tartCmd.Output()
	if err != nil {
		return entries
	}
	var vms []struct {
		Name   string `json:"Name"`
		State  string `json:"State"`
		CPU    int    `json:"CPU"`
		Memory int    `json:"Memory"`
		Disk   int    `json:"Disk"`
	}
	if err := json.Unmarshal(output, &vms); err != nil {
		return entries
	}
	for _, vm := range vms {
		// Skip OCI image references — only show actual VMs
		if strings.Contains(vm.Name, "/") || strings.Contains(vm.Name, "@") {
			continue
		}
		icon := "⚪"
		switch vm.State {
		case "running":
			icon = "🟢"
		case "stopped":
			icon = "🔴"
		case "suspended":
			icon = "🟡"
		}
		entries = append(entries, listEntry{
			Icon:   icon,
			Name:   vm.Name,
			Status: vm.State,
			OS:     "macos",
			CPUs:   strconv.Itoa(vm.CPU),
			Memory: fmt.Sprintf("%d MB", vm.Memory),
			Disk:   fmt.Sprintf("%d GB", vm.Disk),
		})
	}
	return entries
}

func listQemuVMs() []listEntry {
	var entries []listEntry
	home, err := os.UserHomeDir()
	if err != nil {
		return entries
	}
	vmsDir := filepath.Join(home, ".config", "machine", "vms")
	dirs, err := os.ReadDir(vmsDir)
	if err != nil {
		return entries
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		diskPath := filepath.Join(vmsDir, d.Name(), "disk.qcow2")
		if _, err := os.Stat(diskPath); os.IsNotExist(err) {
			continue
		}
		// Check if running
		checkCmd := exec.Command("pgrep", "-f", fmt.Sprintf("qemu-system-aarch64.*%s", diskPath))
		running := checkCmd.Run() == nil

		icon := "🔴"
		status := "stopped"
		if running {
			icon = "🟢"
			status = "running"
		}
		entries = append(entries, listEntry{
			Icon:   icon,
			Name:   d.Name(),
			Status: status,
			OS:     "windows",
			CPUs:   "-",
			Memory: "-",
			Disk:   "-",
		})
	}
	return entries
}

func humanizeListBytes(s string) string {
	bytes, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return s
	}
	const gb = 1024 * 1024 * 1024
	const mb = 1024 * 1024
	if bytes >= gb {
		return fmt.Sprintf("%.1f GiB", float64(bytes)/float64(gb))
	}
	return fmt.Sprintf("%.0f MiB", float64(bytes)/float64(mb))
}

func listHetznerVMs() []listEntry {
	var entries []listEntry
	token := os.Getenv("HETZNER_API_TOKEN")
	if token == "" {
		return entries
	}
	req, _ := http.NewRequest("GET", "https://api.hetzner.cloud/v1/servers", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return entries
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Servers []struct {
			Name       string `json:"name"`
			Status     string `json:"status"`
			ServerType struct {
				Cores  int     `json:"cores"`
				Memory float64 `json:"memory"`
				Disk   int     `json:"disk"`
			} `json:"server_type"`
		} `json:"servers"`
	}
	if json.Unmarshal(body, &result) != nil {
		return entries
	}
	for _, s := range result.Servers {
		icon := "⚪"
		switch s.Status {
		case "running":
			icon = "🟢"
		case "off":
			icon = "🔴"
		}
		entries = append(entries, listEntry{
			Icon:   icon,
			Name:   s.Name,
			Status: s.Status,
			OS:     "hetzner",
			CPUs:   strconv.Itoa(s.ServerType.Cores),
			Memory: fmt.Sprintf("%.0f GB", s.ServerType.Memory),
			Disk:   fmt.Sprintf("%d GB", s.ServerType.Disk),
		})
	}
	return entries
}

func listDigitalOceanVMs() []listEntry {
	var entries []listEntry
	token := os.Getenv("DIGITALOCEAN_TOKEN")
	if token == "" {
		token = os.Getenv("DIGITAL_OCEAN_PAT")
	}
	if token == "" {
		return entries
	}
	req, _ := http.NewRequest("GET", "https://api.digitalocean.com/v2/droplets?tag_name=mach", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return entries
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result struct {
		Droplets []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Size   struct {
				VCPUs  int `json:"vcpus"`
				Memory int `json:"memory"`
				Disk   int `json:"disk"`
			} `json:"size"`
		} `json:"droplets"`
	}
	if json.Unmarshal(body, &result) != nil {
		return entries
	}
	for _, d := range result.Droplets {
		icon := "⚪"
		switch d.Status {
		case "active":
			icon = "🟢"
		case "off":
			icon = "🔴"
		}
		entries = append(entries, listEntry{
			Icon:   icon,
			Name:   d.Name,
			Status: d.Status,
			OS:     "do",
			CPUs:   strconv.Itoa(d.Size.VCPUs),
			Memory: fmt.Sprintf("%d MB", d.Size.Memory),
			Disk:   fmt.Sprintf("%d GB", d.Size.Disk),
		})
	}
	return entries
}

func init() {
	rootCmd.AddCommand(listCmd)
}

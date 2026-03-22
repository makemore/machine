package adapter

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/makemore/machine/pkg/machinefile"
)

//go:embed autounattend.xml
var autounattendXML []byte

// QemuAdapter implements Adapter for Windows ARM VMs using QEMU with HVF
type QemuAdapter struct{}

// NewQemuAdapter creates a new QEMU adapter
func NewQemuAdapter() *QemuAdapter {
	return &QemuAdapter{}
}

// machineDir returns the directory for storing VM files
func (q *QemuAdapter) machineDir(name string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "machine", "vms", name)
}

// Create creates and starts a Windows ARM VM via QEMU
func (q *QemuAdapter) Create(mf *machinefile.Machinefile) error {
	dir := q.machineDir(mf.Name)
	diskPath := filepath.Join(dir, "disk.qcow2")

	// If disk already exists, just start the VM
	if _, err := os.Stat(diskPath); err == nil {
		fmt.Printf("   VM '%s' already exists, starting...\n", mf.Name)
		return q.startVM(mf, false)
	}

	// First-time setup
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating VM directory: %w", err)
	}

	// Prepare UEFI firmware
	fmt.Printf("   🔧 Preparing UEFI firmware...\n")
	if err := q.prepareUEFI(dir); err != nil {
		return fmt.Errorf("preparing UEFI: %w", err)
	}

	// Create disk image
	diskSizeGB := 64
	fmt.Printf("   💾 Creating %dGB disk image...\n", diskSizeGB)
	cmd := exec.Command("qemu-img", "create", "-f", "qcow2", diskPath, fmt.Sprintf("%dG", diskSizeGB))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("creating disk: %w", err)
	}

	// Check for Windows ISO
	isoPath := filepath.Join(dir, "windows.iso")
	if _, err := os.Stat(isoPath); os.IsNotExist(err) {
		fmt.Printf("   📥 No Windows ISO found. Downloading Windows 11 ARM64 via UUP dump...\n")
		if err := q.downloadWindowsISO(dir, isoPath); err != nil {
			fmt.Printf("\n   ⚠️  Auto-download failed: %v\n", err)
			fmt.Printf("   You can manually place a Windows 11 ARM ISO at:\n")
			fmt.Printf("     %s\n\n", isoPath)
			fmt.Printf("   Or download from: https://uupdump.net/?q=windows+11+arm64\n")
			return fmt.Errorf("Windows ISO required for first-time setup")
		}
	}

	// Download virtio drivers if not present
	virtioPath := filepath.Join(dir, "virtio-win.iso")
	if _, err := os.Stat(virtioPath); os.IsNotExist(err) {
		fmt.Printf("   📥 Downloading VirtIO drivers...\n")
		dlCmd := exec.Command("curl", "-fsSL", "-o", virtioPath,
			"https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/latest-virtio/virtio-win.iso")
		dlCmd.Stdout = os.Stdout
		dlCmd.Stderr = os.Stderr
		if err := dlCmd.Run(); err != nil {
			return fmt.Errorf("downloading virtio drivers: %w", err)
		}
	}

	// Start VM with unattended install
	fmt.Printf("   🖥️  Starting unattended Windows install...\n")
	return q.startVM(mf, true)
}

// prepareUEFI prepares the UEFI firmware images for QEMU
func (q *QemuAdapter) prepareUEFI(dir string) error {
	efiSrc := "/opt/homebrew/opt/qemu/share/qemu/edk2-aarch64-code.fd"
	varsSrc := "/opt/homebrew/opt/qemu/share/qemu/edk2-arm-vars.fd"

	efiDst := filepath.Join(dir, "efi.img")
	varsDst := filepath.Join(dir, "vars.img")

	// Create EFI image (64MB)
	if _, err := os.Stat(efiDst); os.IsNotExist(err) {
		if err := createPaddedFirmware(efiSrc, efiDst, 64*1024*1024); err != nil {
			return fmt.Errorf("creating EFI image: %w", err)
		}
	}

	// Create vars image (64MB)
	if _, err := os.Stat(varsDst); os.IsNotExist(err) {
		if err := createPaddedFirmware(varsSrc, varsDst, 64*1024*1024); err != nil {
			return fmt.Errorf("creating vars image: %w", err)
		}
	}

	return nil
}

// createPaddedFirmware creates a firmware image padded to the given size
func createPaddedFirmware(src, dst string, size int64) error {
	// Create file of target size
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	if err := f.Truncate(size); err != nil {
		f.Close()
		return err
	}
	f.Close()

	// Copy firmware data into it
	srcData, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("reading firmware %s: %w", src, err)
	}
	f, err = os.OpenFile(dst, os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(srcData)
	return err
}

// uupBuild represents a single build entry from UUP dump
type uupBuild struct {
	Title   string `json:"title"`
	Build   string `json:"build"`
	Arch    string `json:"arch"`
	UUID    string `json:"uuid"`
	Created int64  `json:"created"`
}

// uupListResponse represents the UUP dump list API response
type uupListResponse struct {
	Response struct {
		Builds map[string]uupBuild `json:"builds"`
	} `json:"response"`
}

// downloadWindowsISO downloads a Windows 11 ARM64 ISO using the UUP dump API
func (q *QemuAdapter) downloadWindowsISO(dir, isoPath string) error {
	// Step 1: Find the latest stable Windows 11 ARM64 build
	fmt.Printf("   🔍 Finding latest Windows 11 ARM64 build...\n")
	resp, err := http.Get("https://api.uupdump.net/listid.php?search=windows+11+arm64&sortByDate=1")
	if err != nil {
		return fmt.Errorf("querying UUP dump API: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading API response: %w", err)
	}

	var listResp uupListResponse
	if err := json.Unmarshal(body, &listResp); err != nil {
		return fmt.Errorf("parsing API response: %w", err)
	}

	// Find the newest stable (non-Insider, non-Preview) ARM64 build
	// Builds are a map keyed by string indices, so we need to sort by created date
	var buildUUID, buildTitle string
	var fallbackUUID, fallbackTitle string
	var bestCreated, fallbackCreated int64

	for _, b := range listResp.Response.Builds {
		if b.Arch != "arm64" {
			continue
		}
		if b.Created > fallbackCreated {
			fallbackUUID = b.UUID
			fallbackTitle = b.Title
			fallbackCreated = b.Created
		}
		// Skip insider preview / canary / dev builds
		lower := strings.ToLower(b.Title)
		if strings.Contains(lower, "insider preview") || strings.Contains(lower, "canary") || strings.Contains(lower, "prerelease") {
			continue
		}
		if b.Created > bestCreated {
			buildUUID = b.UUID
			buildTitle = b.Title
			bestCreated = b.Created
		}
	}

	if buildUUID == "" {
		buildUUID = fallbackUUID
		buildTitle = fallbackTitle
	}

	if buildUUID == "" {
		return fmt.Errorf("no Windows 11 ARM64 builds found on UUP dump")
	}

	fmt.Printf("   📦 Found: %s\n", buildTitle)
	fmt.Printf("   📥 Downloading UUP files and building ISO (this will take a while)...\n")

	// Step 2: Download the UUP dump conversion script
	scriptDir := filepath.Join(dir, "uup-download")
	if err := os.MkdirAll(scriptDir, 0755); err != nil {
		return fmt.Errorf("creating script dir: %w", err)
	}

	// Download the UUP dump download package
	packageURL := fmt.Sprintf("https://uupdump.net/get.php?id=%s&pack=en-us&edition=professional&autodl=2", buildUUID)
	fmt.Printf("   📋 Download package from: %s\n", packageURL)
	fmt.Printf("   📋 Alternatively, visit https://uupdump.net and search for the build.\n\n")

	// Use aria2c with the UUP dump API to download files directly
	// First, get the download links
	getURL := fmt.Sprintf("https://api.uupdump.net/get.php?id=%s&lang=en-us&edition=professional", buildUUID)
	getResp, err := http.Get(getURL)
	if err != nil {
		return fmt.Errorf("getting download links: %w", err)
	}
	defer getResp.Body.Close()

	getBody, err := io.ReadAll(getResp.Body)
	if err != nil {
		return fmt.Errorf("reading download links: %w", err)
	}

	var getResult struct {
		Response struct {
			UpdateName string `json:"updateName"`
			Files      map[string]struct {
				URL string `json:"url"`
			} `json:"files"`
		} `json:"response"`
	}
	if err := json.Unmarshal(getBody, &getResult); err != nil {
		return fmt.Errorf("parsing download links: %w", err)
	}

	if len(getResult.Response.Files) == 0 {
		return fmt.Errorf("no download files found for build %s", buildUUID)
	}

	// Create aria2 input file
	ariaInputPath := filepath.Join(scriptDir, "aria2-input.txt")
	ariaFile, err := os.Create(ariaInputPath)
	if err != nil {
		return fmt.Errorf("creating aria2 input: %w", err)
	}

	filesDir := filepath.Join(scriptDir, "files")
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		return fmt.Errorf("creating files dir: %w", err)
	}

	fileCount := 0
	skipped := 0
	for filename, info := range getResult.Response.Files {
		if info.URL == "" || info.URL == "null" {
			continue
		}
		lower := strings.ToLower(filename)
		// Only download essential files for a bootable ISO:
		// - .esd files (core Windows image)
		// - .wim files (boot/edge images)
		// - AggregatedMetadata and DesktopDeployment (needed for conversion)
		isEssential := strings.HasSuffix(lower, ".esd") ||
			strings.HasSuffix(lower, ".wim") ||
			strings.Contains(lower, "aggregatedmetadata") ||
			strings.Contains(lower, "desktopdeployment")
		if !isEssential {
			skipped++
			continue
		}
		fmt.Fprintf(ariaFile, "%s\n", info.URL)
		fmt.Fprintf(ariaFile, "  out=%s\n", filename)
		fileCount++
	}
	ariaFile.Close()

	if skipped > 0 {
		fmt.Printf("   ⏩ Skipped %d optional packages (language packs, features on demand, etc.)\n", skipped)
	}

	fmt.Printf("   📥 Downloading %d UUP files with aria2...\n", fileCount)
	ariaCmd := exec.Command("aria2c",
		"--input-file", ariaInputPath,
		"--dir", filesDir,
		"--max-connection-per-server=16",
		"--max-concurrent-downloads=8",
		"--continue=true",
		"--check-certificate=false",
		"--summary-interval=10")
	ariaCmd.Stdout = os.Stdout
	ariaCmd.Stderr = os.Stderr
	if err := ariaCmd.Run(); err != nil {
		return fmt.Errorf("downloading UUP files: %w", err)
	}

	// Step 3: Convert UUP files to ISO using wimlib
	fmt.Printf("   🔨 Converting UUP files to ISO...\n")
	if err := q.convertUUPToISO(filesDir, isoPath); err != nil {
		return fmt.Errorf("converting to ISO: %w", err)
	}

	// Clean up download files
	fmt.Printf("   🧹 Cleaning up temporary files...\n")
	os.RemoveAll(scriptDir)

	fmt.Printf("   ✅ Windows ISO created at %s\n", isoPath)
	return nil
}

// convertUUPToISO converts downloaded UUP ESD/CAB files into a bootable ISO
func (q *QemuAdapter) convertUUPToISO(filesDir, isoPath string) error {
	// Find the main ESD file (contains install.wim data)
	entries, err := os.ReadDir(filesDir)
	if err != nil {
		return fmt.Errorf("reading files dir: %w", err)
	}

	var esdFiles []string
	var cabFiles []string
	for _, e := range entries {
		name := strings.ToLower(e.Name())
		if strings.HasSuffix(name, ".esd") {
			esdFiles = append(esdFiles, filepath.Join(filesDir, e.Name()))
		} else if strings.HasSuffix(name, ".cab") {
			cabFiles = append(cabFiles, filepath.Join(filesDir, e.Name()))
		}
	}

	if len(esdFiles) == 0 {
		return fmt.Errorf("no ESD files found in download directory")
	}

	// Extract CAB files
	for _, cab := range cabFiles {
		fmt.Printf("   📦 Extracting %s...\n", filepath.Base(cab))
		cmd := exec.Command("cabextract", "-d", filesDir, cab)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		_ = cmd.Run() // Some CABs may fail, that's ok
	}

	// Find the main install ESD
	var installESD string
	for _, esd := range esdFiles {
		base := strings.ToLower(filepath.Base(esd))
		if strings.Contains(base, "professional") || strings.Contains(base, "client") || strings.Contains(base, "core") {
			installESD = esd
			break
		}
	}
	if installESD == "" {
		installESD = esdFiles[0]
	}

	// Convert ESD to install.wim
	wimPath := filepath.Join(filesDir, "install.wim")
	fmt.Printf("   🔨 Converting ESD to WIM (this takes several minutes)...\n")
	exportCmd := exec.Command("wimlib-imagex", "export", installESD, "all", wimPath, "--compress=LZX")
	exportCmd.Stdout = os.Stdout
	exportCmd.Stderr = os.Stderr
	if err := exportCmd.Run(); err != nil {
		return fmt.Errorf("converting ESD to WIM: %w", err)
	}

	// Create a minimal ISO structure
	isoDir := filepath.Join(filesDir, "iso-root")
	sourcesDir := filepath.Join(isoDir, "sources")
	if err := os.MkdirAll(sourcesDir, 0755); err != nil {
		return fmt.Errorf("creating ISO structure: %w", err)
	}

	// Move install.wim into the ISO structure
	if err := os.Rename(wimPath, filepath.Join(sourcesDir, "install.wim")); err != nil {
		return fmt.Errorf("moving install.wim: %w", err)
	}

	// Extract boot files from the ESD for a bootable ISO
	fmt.Printf("   📀 Building bootable ISO...\n")
	bootWim := filepath.Join(sourcesDir, "boot.wim")
	// Try to find and export the boot image (index 1 is usually Windows PE)
	for _, esd := range esdFiles {
		exportBoot := exec.Command("wimlib-imagex", "export", esd, "1", bootWim, "--compress=LZX", "--boot")
		if err := exportBoot.Run(); err == nil {
			break
		}
	}

	// Create ISO using mkisofs
	mkisoCmd := exec.Command("mkisofs",
		"-o", isoPath,
		"-b", "sources/boot.wim",
		"-no-emul-boot",
		"-iso-level", "3",
		"-udf",
		"-joliet",
		"-R",
		isoDir)
	mkisoCmd.Stdout = os.Stdout
	mkisoCmd.Stderr = os.Stderr
	if err := mkisoCmd.Run(); err != nil {
		// Fallback: create a non-bootable ISO with just install.wim
		// The user will need to use the EFI firmware to boot it
		mkisoFallback := exec.Command("mkisofs",
			"-o", isoPath,
			"-iso-level", "3",
			"-udf",
			"-joliet",
			"-R",
			isoDir)
		mkisoFallback.Stdout = os.Stdout
		mkisoFallback.Stderr = os.Stderr
		if err := mkisoFallback.Run(); err != nil {
			return fmt.Errorf("creating ISO: %w", err)
		}
	}

	return nil
}


// startVM launches QEMU with the appropriate flags
func (q *QemuAdapter) startVM(mf *machinefile.Machinefile, withInstallMedia bool) error {
	// Kill any stale QEMU process for this VM
	_ = q.Stop(mf)
	time.Sleep(1 * time.Second)

	dir := q.machineDir(mf.Name)
	memGB := strings.TrimSuffix(strings.ToLower(mf.Resources.Memory), "gb")

	args := []string{
		"-m", memGB + "G",
		"-smp", strconv.Itoa(mf.Resources.CPU),
		"-cpu", "host",
		"-M", "virt",
		"-accel", "hvf",
		"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s/efi.img,readonly=on", dir),
		"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s/vars.img", dir),
		"-device", "qemu-xhci",
		"-device", "usb-kbd",
		"-device", "usb-tablet",
		"-device", "virtio-net-pci,netdev=net",
		"-device", "virtio-blk,drive=system",
		"-drive", fmt.Sprintf("if=none,id=system,format=qcow2,file=%s/disk.qcow2", dir),
	}

	// Port forwarding: SSH (22→2222) + RDP (3389→3389) + any exposed ports
	hostfwd := "hostfwd=tcp::2222-:22,hostfwd=tcp::3389-:3389"
	for _, port := range mf.Expose {
		if port != 3389 {
			hostfwd += fmt.Sprintf(",hostfwd=tcp::%d-:%d", port, port)
		}
	}
	args = append(args, "-netdev", fmt.Sprintf("user,id=net,ipv6=off,%s", hostfwd))

	if withInstallMedia {
		// Unattended install: create autounattend ISO and attach everything
		autounattendISO := filepath.Join(dir, "autounattend.iso")
		if err := q.createAutounattendISO(dir, autounattendISO); err != nil {
			return fmt.Errorf("creating autounattend ISO: %w", err)
		}

		isoPath := filepath.Join(dir, "windows.iso")
		virtioPath := filepath.Join(dir, "virtio-win.iso")
		args = append(args,
			"-device", "ramfb",
			"-device", "usb-storage,drive=install",
			"-drive", fmt.Sprintf("if=none,id=install,format=raw,media=cdrom,readonly=on,file=%s", isoPath),
			"-device", "usb-storage,drive=virtio-drivers",
			"-drive", fmt.Sprintf("if=none,id=virtio-drivers,format=raw,media=cdrom,readonly=on,file=%s", virtioPath),
			"-device", "usb-storage,drive=unattend",
			"-drive", fmt.Sprintf("if=none,id=unattend,format=raw,media=cdrom,readonly=on,file=%s", autounattendISO),
		)
	}

	// Run headless unless MACH_DEBUG is set (shows QEMU display for debugging)
	if os.Getenv("MACH_DEBUG") != "" {
		fmt.Printf("   🔍 Debug mode: showing QEMU display window\n")
		args = append(args, "-device", "ramfb")
	} else {
		args = append(args, "-display", "none", "-daemonize")
	}

	cmd := exec.Command("qemu-system-aarch64", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if os.Getenv("MACH_DEBUG") != "" {
		// Debug mode: run in foreground (display window needs it)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("starting VM: %w", err)
		}
	} else {
		// Daemonized: cmd.Run() returns immediately after daemonize
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("starting VM: %w", err)
		}
	}

	if withInstallMedia {
		fmt.Printf("   🖥️  Windows is installing unattended (this takes 10-20 minutes)...\n")
		fmt.Printf("   ⏳ Waiting for install to complete and SSH to become available...\n")
		fmt.Printf("   💡 Press Ctrl+C to cancel (VM will be stopped cleanly)\n")
	} else {
		fmt.Printf("   ⏳ Waiting for VM to boot (SSH on localhost:2222)...\n")
	}

	// Catch Ctrl+C so we can clean up the QEMU process
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	// Wait longer for install (up to 30 min), shorter for normal boot (3 min)
	maxAttempts := 60
	if withInstallMedia {
		maxAttempts = 600
	}

	for i := 0; i < maxAttempts; i++ {
		select {
		case <-sigChan:
			fmt.Printf("\n   ⚠️  Interrupted — stopping VM...\n")
			_ = q.Stop(mf)
			fmt.Printf("   ✅ VM stopped cleanly. Run 'mach -f Machinefile.windows up' to retry.\n")
			if withInstallMedia {
				// Install was incomplete — remove the disk so next run starts fresh
				diskPath := filepath.Join(dir, "disk.qcow2")
				os.Remove(diskPath)
				fmt.Printf("   🧹 Removed incomplete disk image. Next run will start a fresh install.\n")
			}
			os.Exit(0)
		default:
		}

		time.Sleep(3 * time.Second)
		elapsed := (i + 1) * 3

		// Check if QEMU is still running and get CPU usage
		diskPath := filepath.Join(dir, "disk.qcow2")
		pidCmd := exec.Command("pgrep", "-f", fmt.Sprintf("qemu-system-aarch64.*%s", diskPath))
		pidOut, pidErr := pidCmd.Output()
		if pidErr != nil {
			fmt.Printf("\n   ❌ QEMU process has exited unexpectedly.\n")
			return fmt.Errorf("QEMU process died during boot")
		}
		pid := strings.TrimSpace(string(pidOut))

		// Get CPU usage
		cpuUsage := ""
		psCmd := exec.Command("ps", "-p", pid, "-o", "%cpu=")
		if psOut, err := psCmd.Output(); err == nil {
			cpuUsage = strings.TrimSpace(string(psOut))
		}

		// Detect phase by probing ports
		phase := q.detectPhase(cpuUsage)

		// Try SSH
		check := exec.Command("sshpass", "-p", "User", "ssh",
			"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
			"-o", "ConnectTimeout=3", "-p", "2222", "User@localhost", "echo ok")
		if output, err := check.Output(); err == nil && strings.TrimSpace(string(output)) == "ok" {
			fmt.Printf("\n   ✅ VM is ready (SSH on localhost:2222)\n")
			return nil
		}

		// Show progress every 6 seconds
		if elapsed%6 == 0 {
			mins := elapsed / 60
			secs := elapsed % 60
			fmt.Printf("\r   ⏳ %dm%02ds │ cpu: %5s%% │ %s", mins, secs, cpuUsage, phase)
		}
	}

	fmt.Printf("\n   ⚠️  VM started but SSH not responding after waiting.\n")
	return nil
}

// detectPhase tries to determine what the Windows VM is doing
func (q *QemuAdapter) detectPhase(cpuUsage string) string {
	// Try actual SSH banner exchange (not just port open — QEMU opens the port immediately)
	sshBanner := exec.Command("ssh", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=1", "-o", "BatchMode=yes", "-p", "2222", "User@localhost", "exit")
	sshOut, _ := sshBanner.CombinedOutput()
	sshResult := string(sshOut)

	if strings.Contains(sshResult, "Permission denied") || strings.Contains(sshResult, "password") {
		return "SSH ready — authenticating...              "
	}
	if strings.Contains(sshResult, "Connection refused") {
		// Port forward exists but nothing listening inside guest
	} else if strings.Contains(sshResult, "banner") || strings.Contains(sshResult, "SSH") {
		return "SSH service starting...                    "
	}

	// Parse CPU to guess phase
	cpu := 0.0
	fmt.Sscanf(cpuUsage, "%f", &cpu)

	if cpu > 80 {
		return "Installing (high CPU activity)...          "
	} else if cpu > 20 {
		return "Configuring (moderate activity)...          "
	} else if cpu > 5 {
		return "Booting Windows...                         "
	} else if cpu > 1 {
		return "UEFI / early boot...                       "
	}
	return "Waiting for UEFI firmware...                "
}

// createAutounattendISO creates a small ISO containing autounattend.xml
func (q *QemuAdapter) createAutounattendISO(dir, isoPath string) error {
	// Write the embedded XML to a temp directory
	tmpDir := filepath.Join(dir, "autounattend-tmp")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	xmlPath := filepath.Join(tmpDir, "autounattend.xml")
	if err := os.WriteFile(xmlPath, autounattendXML, 0644); err != nil {
		return err
	}

	// Create ISO
	cmd := exec.Command("mkisofs",
		"-o", isoPath,
		"-iso-level", "3",
		"-udf",
		"-joliet",
		"-R",
		tmpDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}


// getInstallCommand returns the install command for Windows package managers
func (q *QemuAdapter) getInstallCommand(pm, pkg string) string {
	switch pm {
	case "choco":
		return fmt.Sprintf("choco install -y %s", pkg)
	case "winget":
		return fmt.Sprintf("winget install --accept-package-agreements --accept-source-agreements -e %s", pkg)
	default:
		return fmt.Sprintf("choco install -y %s", pkg)
	}
}

// execInVM runs a command inside the Windows VM via SSH
func (q *QemuAdapter) execInVM(command string) error {
	cmd := exec.Command("sshpass", "-p", "User",
		"ssh", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		"-p", "2222", "User@localhost", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Provision runs setup steps inside the VM
func (q *QemuAdapter) Provision(mf *machinefile.Machinefile) error {
	// Ensure Chocolatey is installed if using choco
	if mf.PackageManager == "choco" {
		fmt.Printf("   🍫 Ensuring Chocolatey is installed...\n")
		chocoCheck := `powershell -Command "if (!(Get-Command choco -ErrorAction SilentlyContinue)) { Set-ExecutionPolicy Bypass -Scope Process -Force; [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072; iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1')) }"`
		if err := q.execInVM(chocoCheck); err != nil {
			return fmt.Errorf("installing Chocolatey: %w", err)
		}
	}

	for _, step := range mf.Setup {
		if step.Install != "" {
			fmt.Printf("   📦 Installing %s...\n", step.Install)
			cmd := q.getInstallCommand(mf.PackageManager, step.Install)
			if err := q.execInVM(cmd); err != nil {
				return fmt.Errorf("installing %s: %w", step.Install, err)
			}
		}
		if step.Clone != nil {
			fmt.Printf("   📥 Cloning %s...\n", step.Clone.Repo)
			cmd := fmt.Sprintf("git clone %s %s", step.Clone.Repo, step.Clone.Dest)
			if err := q.execInVM(cmd); err != nil {
				return fmt.Errorf("cloning %s: %w", step.Clone.Repo, err)
			}
		}
		if step.Cmd != "" {
			fmt.Printf("   ▶️  Running: %s\n", step.Cmd)
			if err := q.execInVM(step.Cmd); err != nil {
				return fmt.Errorf("running cmd: %w", err)
			}
		}
	}
	return nil
}

// Run executes run commands
func (q *QemuAdapter) Run(mf *machinefile.Machinefile) error {
	for _, step := range mf.Run {
		if step.Cmd != "" {
			fmt.Printf("   ▶️  %s\n", step.Cmd)
			if err := q.execInVM(step.Cmd); err != nil {
				return err
			}
		}
	}
	return nil
}

// Stop stops the VM by sending ACPI shutdown via QEMU monitor
func (q *QemuAdapter) Stop(mf *machinefile.Machinefile) error {
	// Find and kill the QEMU process for this VM
	dir := q.machineDir(mf.Name)
	diskPath := filepath.Join(dir, "disk.qcow2")
	cmd := exec.Command("pkill", "-f", fmt.Sprintf("qemu-system-aarch64.*%s", diskPath))
	_ = cmd.Run() // Ignore error if not running
	return nil
}

// Destroy deletes the VM and all its files
func (q *QemuAdapter) Destroy(mf *machinefile.Machinefile) error {
	_ = q.Stop(mf)
	dir := q.machineDir(mf.Name)
	return os.RemoveAll(dir)
}

// Connect opens an SSH session to the Windows VM
func (q *QemuAdapter) Connect(mf *machinefile.Machinefile) error {
	cmd := exec.Command("sshpass", "-p", "User",
		"ssh", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		"-p", "2222", "User@localhost")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Status shows the status of the Windows VM
func (q *QemuAdapter) Status(mf *machinefile.Machinefile) error {
	dir := q.machineDir(mf.Name)
	diskPath := filepath.Join(dir, "disk.qcow2")

	// Check if disk exists
	if _, err := os.Stat(diskPath); os.IsNotExist(err) {
		return fmt.Errorf("machine '%s' not found (has it been created?)", mf.Name)
	}

	// Check if QEMU process is running
	checkCmd := exec.Command("pgrep", "-f", fmt.Sprintf("qemu-system-aarch64.*%s", diskPath))
	running := checkCmd.Run() == nil

	icon := "🔴"
	status := "stopped"
	if running {
		icon = "🟢"
		status = "running"
	}

	fmt.Printf("%s  Machine: %s\n", icon, mf.Name)
	fmt.Printf("   Status:  %s\n", status)
	fmt.Printf("   OS:      Windows (ARM)\n")
	fmt.Printf("   CPUs:    %d\n", mf.Resources.CPU)
	fmt.Printf("   Memory:  %s\n", mf.Resources.Memory)
	fmt.Printf("   Disk:    %s\n", diskPath)
	if running {
		fmt.Printf("   SSH:     localhost:2222\n")
		fmt.Printf("   RDP:     localhost:3389\n")
	}

	return nil
}

// Info returns structured machine info
func (q *QemuAdapter) Info(mf *machinefile.Machinefile) (*MachineInfo, error) {
	dir := q.machineDir(mf.Name)
	diskPath := filepath.Join(dir, "disk.qcow2")
	status := "stopped"
	checkCmd := exec.Command("pgrep", "-f", fmt.Sprintf("qemu-system-aarch64.*%s", diskPath))
	if checkCmd.Run() == nil {
		status = "running"
	}
	return &MachineInfo{
		Name:     mf.Name,
		Status:   status,
		Provider: "local",
		OS:       "windows",
		CPUs:     mf.Resources.CPU,
	}, nil
}
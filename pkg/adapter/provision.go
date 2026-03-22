package adapter

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/makemore/machine/pkg/machinefile"
)

// sshExec runs a command on a remote host via SSH
func sshExec(user, ip, command string) error {
	cmd := exec.Command("ssh", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		fmt.Sprintf("%s@%s", user, ip), command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// scpFile copies a local file to a remote host
func scpFile(user, ip, localPath, remotePath string) error {
	cmd := exec.Command("scp", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		localPath, fmt.Sprintf("%s@%s:%s", user, ip, remotePath))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// provisionCloud runs setup steps on a cloud VM via SSH
// Handles install, clone, cmd, script steps + env injection
func provisionCloud(mf *machinefile.Machinefile, user, ip string, useSudo bool) error {
	// Inject environment variables first
	if len(mf.Env) > 0 {
		fmt.Printf("   🔧 Setting %d environment variable(s)...\n", len(mf.Env))
		var envLines []string
		for k, v := range mf.Env {
			envLines = append(envLines, fmt.Sprintf("echo 'export %s=%q' >> /etc/environment", k, v))
			// Also export for this session
			envLines = append(envLines, fmt.Sprintf("export %s=%q", k, v))
		}
		envCmd := strings.Join(envLines, " && ")
		if useSudo {
			envCmd = "sudo bash -c '" + strings.ReplaceAll(envCmd, "'", "'\\''") + "'"
		}
		if err := sshExec(user, ip, envCmd); err != nil {
			return fmt.Errorf("setting env vars: %w", err)
		}
	}

	aptPrefix := ""
	if useSudo {
		aptPrefix = "sudo "
	}

	for _, step := range mf.Setup {
		if step.Install != "" {
			fmt.Printf("   📦 Installing %s...\n", step.Install)
			cmd := fmt.Sprintf("while %sfuser /var/lib/dpkg/lock-frontend /var/lib/apt/lists/lock /var/cache/apt/archives/lock >/dev/null 2>&1; do sleep 3; done && %sapt-get update -qq && %sDEBIAN_FRONTEND=noninteractive apt-get install -y %s",
				aptPrefix, aptPrefix, aptPrefix, step.Install)
			if err := sshExec(user, ip, cmd); err != nil {
				return fmt.Errorf("installing %s: %w", step.Install, err)
			}
		}
		if step.Clone != nil {
			fmt.Printf("   📥 Cloning %s...\n", step.Clone.Repo)
			cmd := fmt.Sprintf("git clone %s %s", step.Clone.Repo, step.Clone.Dest)
			if err := sshExec(user, ip, cmd); err != nil {
				return fmt.Errorf("cloning %s: %w", step.Clone.Repo, err)
			}
		}
		if step.Cmd != "" {
			fmt.Printf("   ▶️  Running: %s\n", step.Cmd)
			if err := sshExec(user, ip, step.Cmd); err != nil {
				return fmt.Errorf("running cmd: %w", err)
			}
		}
		if step.Script != "" {
			fmt.Printf("   📜 Running script: %s\n", step.Script)
			// Check file exists locally
			if _, err := os.Stat(step.Script); os.IsNotExist(err) {
				return fmt.Errorf("script file not found: %s", step.Script)
			}
			// Copy to VM then execute
			remotePath := fmt.Sprintf("/tmp/mach-script-%d.sh", os.Getpid())
			if err := scpFile(user, ip, step.Script, remotePath); err != nil {
				return fmt.Errorf("copying script %s: %w", step.Script, err)
			}
			runCmd := fmt.Sprintf("chmod +x %s && %s", remotePath, remotePath)
			if useSudo {
				runCmd = fmt.Sprintf("chmod +x %s && sudo %s", remotePath, remotePath)
			}
			if err := sshExec(user, ip, runCmd); err != nil {
				return fmt.Errorf("running script %s: %w", step.Script, err)
			}
		}
	}
	return nil
}


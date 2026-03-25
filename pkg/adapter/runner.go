package adapter

import (
	"fmt"
	"os"
	"os/exec"
)

// CommandRunner abstracts how commands are executed on a target machine.
// SSH-based runners use ssh/scp, Lima uses limactl shell.
type CommandRunner interface {
	// Exec runs a command on the target machine
	Exec(command string) error
	// CopyFile copies a local file to the target machine
	CopyFile(localPath, remotePath string) error
}

// SSHRunner executes commands via SSH
type SSHRunner struct {
	User    string
	IP      string
	KeyPath string // optional: explicit path to private key
}

// sshKeyArgs returns -i flags if a key path is set
func (r *SSHRunner) sshKeyArgs() []string {
	if r.KeyPath != "" {
		return []string{"-i", r.KeyPath}
	}
	return nil
}

func (r *SSHRunner) Exec(command string) error {
	args := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=10",
	}
	args = append(args, r.sshKeyArgs()...)
	args = append(args, fmt.Sprintf("%s@%s", r.User, r.IP), command)
	cmd := exec.Command("ssh", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (r *SSHRunner) CopyFile(localPath, remotePath string) error {
	args := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ServerAliveInterval=30",
	}
	args = append(args, r.sshKeyArgs()...)
	args = append(args, localPath, fmt.Sprintf("%s@%s:%s", r.User, r.IP, remotePath))
	cmd := exec.Command("scp", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// LimaRunner executes commands via limactl shell
type LimaRunner struct {
	Name string
}

func (r *LimaRunner) Exec(command string) error {
	// Run as root, clearing SUDO_* vars so npm doesn't drop privileges
	wrapped := "unset SUDO_UID SUDO_GID SUDO_USER SUDO_COMMAND; export HOME=/root DEBIAN_FRONTEND=noninteractive; " + command
	cmd := exec.Command("limactl", "shell", "--workdir=/", r.Name, "sudo", "bash", "-c", wrapped)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (r *LimaRunner) CopyFile(localPath, remotePath string) error {
	cmd := exec.Command("limactl", "copy", localPath, fmt.Sprintf("%s:%s", r.Name, remotePath))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

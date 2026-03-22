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
	User string
	IP   string
}

func (r *SSHRunner) Exec(command string) error {
	cmd := exec.Command("ssh", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		fmt.Sprintf("%s@%s", r.User, r.IP), command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (r *SSHRunner) CopyFile(localPath, remotePath string) error {
	cmd := exec.Command("scp", "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null",
		localPath, fmt.Sprintf("%s@%s:%s", r.User, r.IP, remotePath))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// LimaRunner executes commands via limactl shell
type LimaRunner struct {
	Name string
}

func (r *LimaRunner) Exec(command string) error {
	cmd := exec.Command("limactl", "shell", r.Name, "bash", "-c", command)
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

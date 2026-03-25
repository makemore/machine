package adapter

import (
	"fmt"
	"os"
	"strings"

	"github.com/makemore/machine/pkg/machinefile"
)

// sshExec is kept for backward compat — wraps SSHRunner
func sshExec(user, ip, command string) error {
	return (&SSHRunner{User: user, IP: ip}).Exec(command)
}

// scpFile is kept for backward compat — wraps SSHRunner
func scpFile(user, ip, localPath, remotePath string) error {
	return (&SSHRunner{User: user, IP: ip}).CopyFile(localPath, remotePath)
}

// sshPrivateKeyPath derives the private key path from the public key path.
// e.g. "/builder/home/.ssh/id_ed25519.pub" → "/builder/home/.ssh/id_ed25519"
func sshPrivateKeyPath(pubKeyPath string) string {
	if pubKeyPath == "" {
		return ""
	}
	return strings.TrimSuffix(pubKeyPath, ".pub")
}

// waitForApt returns a command prefix that waits for apt locks.
// Uses lsof as a fallback when fuser isn't available, and also waits
// for unattended-upgrades / dpkg to finish (common on fresh VMs).
func waitForApt(sudo string) string {
	return fmt.Sprintf(
		`echo "Waiting for apt locks..." && `+
			`while %sfuser /var/lib/dpkg/lock-frontend >/dev/null 2>&1 || `+
			`%sfuser /var/lib/apt/lists/lock >/dev/null 2>&1 || `+
			`%sfuser /var/cache/apt/archives/lock >/dev/null 2>&1 || `+
			`%slsof /var/lib/dpkg/lock-frontend >/dev/null 2>&1; do `+
			`echo "  apt locked, waiting..."; sleep 5; done && `+
			`echo "apt locks clear"`,
		sudo, sudo, sudo, sudo)
}

// aptInstall returns a command to install packages via apt
func aptInstall(sudo, packages string) string {
	return fmt.Sprintf("%s && %sapt-get update -qq && %sDEBIAN_FRONTEND=noninteractive apt-get install -y %s",
		waitForApt(sudo), sudo, sudo, packages)
}

// provisionCloud runs the full provisioning pipeline on a cloud VM via SSH
func provisionCloud(mf *machinefile.Machinefile, user, ip string, useSudo bool) error {
	run := &SSHRunner{User: user, IP: ip, KeyPath: sshPrivateKeyPath(mf.SSHKey)}
	return provisionAll(mf, run, ip, useSudo)
}

// provisionAll runs the full provisioning pipeline using the given runner
func provisionAll(mf *machinefile.Machinefile, run CommandRunner, ip string, useSudo bool) error {
	sudo := ""
	if useSudo {
		sudo = "sudo "
	}

	if err := provisionEnv(mf, run, sudo); err != nil {
		return err
	}
	if err := provisionUsers(mf, run, sudo); err != nil {
		return err
	}
	if err := provisionSwap(mf, run, sudo); err != nil {
		return err
	}
	if err := provisionHarden(mf, run, sudo); err != nil {
		return err
	}
	if err := provisionSetup(mf, run, sudo); err != nil {
		return err
	}
	if err := finalizeUsers(mf, run, sudo); err != nil {
		return err
	}
	if err := provisionServices(mf, run, sudo); err != nil {
		return err
	}
	if err := provisionConduit(mf, run, sudo); err != nil {
		return err
	}
	if err := provisionReverseProxy(mf, run, sudo); err != nil {
		return err
	}
	if err := provisionMOTD(mf, run, ip, sudo); err != nil {
		return err
	}
	return nil
}

// provisionEnv injects environment variables
func provisionEnv(mf *machinefile.Machinefile, run CommandRunner, sudo string) error {
	if len(mf.Env) == 0 {
		return nil
	}
	fmt.Printf("   🔧 Setting %d environment variable(s)...\n", len(mf.Env))
	var lines []string
	for k, v := range mf.Env {
		escaped := strings.ReplaceAll(v, "'", "'\\''")
		lines = append(lines, fmt.Sprintf("echo 'export %s='\\''%s'\\''' >> /etc/environment", k, escaped))
	}
	cmd := strings.Join(lines, " && ")
	if sudo != "" {
		cmd = sudo + "bash -c '" + strings.ReplaceAll(cmd, "'", "'\\''") + "'"
	}
	return run.Exec(cmd)
}

// hasDockerInstall checks if docker is in the setup steps
func hasDockerInstall(mf *machinefile.Machinefile) bool {
	for _, step := range mf.Setup {
		if step.Install == "docker" {
			return true
		}
	}
	return false
}

// provisionUsers creates user accounts with SSH, sudo, git credentials
func provisionUsers(mf *machinefile.Machinefile, run CommandRunner, sudo string) error {
	if len(mf.Users) == 0 {
		return nil
	}
	fmt.Printf("   👤 Creating %d user(s)...\n", len(mf.Users))
	for _, u := range mf.Users {
		shell := u.Shell
		if shell == "" {
			shell = "/bin/bash"
		}
		sudoEnabled := u.Sudo == nil || *u.Sudo

		var cmds []string
		// Create user if not exists, or update shell if already exists
		cmds = append(cmds, fmt.Sprintf("id -u %s >/dev/null 2>&1 && %susermod -s %s %s || %suseradd -m -s %s %s", u.Username, sudo, shell, u.Username, sudo, shell, u.Username))
		// Ensure home directory exists (for pre-existing users like Lima default)
		cmds = append(cmds, fmt.Sprintf("%smkdir -p /home/%s", sudo, u.Username))
		if sudoEnabled {
			cmds = append(cmds, fmt.Sprintf("%susermod -aG sudo %s", sudo, u.Username))
			cmds = append(cmds, fmt.Sprintf("echo '%s ALL=(ALL) NOPASSWD:ALL' | %stee /etc/sudoers.d/%s > /dev/null", u.Username, sudo, u.Username))
		}
		cmds = append(cmds, fmt.Sprintf("%smkdir -p /home/%s/.ssh", sudo, u.Username))
		cmds = append(cmds, fmt.Sprintf("echo '%s' | %stee /home/%s/.ssh/authorized_keys > /dev/null", u.PublicKey, sudo, u.Username))
		cmds = append(cmds, fmt.Sprintf("%schmod 700 /home/%s/.ssh && %schmod 600 /home/%s/.ssh/authorized_keys", sudo, u.Username, sudo, u.Username))
		if u.GithubPAT != "" {
			cmds = append(cmds, fmt.Sprintf("echo 'https://%s:%s@github.com' | %stee /home/%s/.git-credentials > /dev/null", u.Username, u.GithubPAT, sudo, u.Username))
			cmds = append(cmds, fmt.Sprintf("%ssu - %s -c 'git config --global credential.helper store'", sudo, u.Username))
		}
		cmds = append(cmds, fmt.Sprintf("%schown -R %s:%s /home/%s", sudo, u.Username, u.Username, u.Username))

		if err := run.Exec(strings.Join(cmds, " && ")); err != nil {
			return fmt.Errorf("creating user %s: %w", u.Username, err)
		}
		fmt.Printf("   ✅ User '%s' created\n", u.Username)
	}
	return nil
}

// finalizeUsers runs after setup to copy oh-my-zsh, add docker group, etc.
// These steps depend on software installed during provisionSetup.
func finalizeUsers(mf *machinefile.Machinefile, run CommandRunner, sudo string) error {
	if len(mf.Users) == 0 {
		return nil
	}
	dockerInstalled := hasDockerInstall(mf)
	for _, u := range mf.Users {
		var cmds []string
		shell := u.Shell
		if shell == "" {
			shell = "/bin/bash"
		}

		// Add to docker group now that docker is installed
		if dockerInstalled {
			cmds = append(cmds, fmt.Sprintf("getent group docker >/dev/null 2>&1 && %susermod -aG docker %s || true", sudo, u.Username))
		}

		// Copy oh-my-zsh to user home now that setup has installed it
		if shell == "/bin/zsh" {
			cmds = append(cmds, fmt.Sprintf(
				"if [ -d /root/.oh-my-zsh ] && [ ! -d /home/%s/.oh-my-zsh ]; then "+
					"%scp -r /root/.oh-my-zsh /home/%s/.oh-my-zsh && "+
					"%scp /root/.zshrc /home/%s/.zshrc 2>/dev/null || true && "+
					"%schown -R %s:%s /home/%s/.oh-my-zsh /home/%s/.zshrc 2>/dev/null || true; "+
					"fi",
				u.Username, sudo, u.Username, sudo, u.Username,
				sudo, u.Username, u.Username, u.Username, u.Username))
		}

		if len(cmds) > 0 {
			if err := run.Exec(strings.Join(cmds, " && ")); err != nil {
				return fmt.Errorf("finalizing user %s: %w", u.Username, err)
			}
		}
	}
	return nil
}


// provisionSwap creates a swap file
func provisionSwap(mf *machinefile.Machinefile, run CommandRunner, sudo string) error {
	if mf.Swap == "" {
		return nil
	}
	fmt.Printf("   💾 Setting up %s swap...\n", mf.Swap)
	size := strings.ToUpper(strings.TrimSpace(mf.Swap))
	size = strings.ReplaceAll(size, "GB", "G")
	size = strings.ReplaceAll(size, "MB", "M")

	cmd := fmt.Sprintf(`if [ -f /swapfile ]; then echo 'Swap already exists'; else %sfallocate -l %s /swapfile && %schmod 600 /swapfile && %smkswap /swapfile && %sswapon /swapfile && echo '/swapfile none swap sw 0 0' | %stee -a /etc/fstab > /dev/null && %ssysctl vm.swappiness=10 && echo 'vm.swappiness=10' | %stee -a /etc/sysctl.conf > /dev/null && echo 'Swap created'; fi`,
		sudo, size, sudo, sudo, sudo, sudo, sudo, sudo)
	return run.Exec(cmd)
}

// provisionHarden locks down the VM
func provisionHarden(mf *machinefile.Machinefile, run CommandRunner, sudo string) error {
	if !mf.Harden {
		return nil
	}
	fmt.Printf("   🔒 Hardening VM...\n")

	fmt.Printf("   🔒 SSH hardening...\n")
	sshCmd := fmt.Sprintf(`%ssed -i 's/^#*PermitRootLogin.*/PermitRootLogin prohibit-password/' /etc/ssh/sshd_config && %ssed -i 's/^#*PasswordAuthentication.*/PasswordAuthentication no/' /etc/ssh/sshd_config && %ssed -i 's/^#*MaxAuthTries.*/MaxAuthTries 3/' /etc/ssh/sshd_config && (%ssystemctl restart sshd 2>/dev/null || %ssystemctl restart ssh)`,
		sudo, sudo, sudo, sudo, sudo)
	if err := run.Exec(sshCmd); err != nil {
		return fmt.Errorf("SSH hardening: %w", err)
	}

	fmt.Printf("   🔒 Installing fail2ban...\n")
	f2bCmd := aptInstall(sudo, "fail2ban")
	f2bCmd += fmt.Sprintf(` && echo '[sshd]
enabled = true
port = ssh
filter = sshd
logpath = /var/log/auth.log
maxretry = 3
bantime = 86400
findtime = 600' | %stee /etc/fail2ban/jail.local > /dev/null && %ssystemctl enable fail2ban && %ssystemctl restart fail2ban`, sudo, sudo, sudo)
	if err := run.Exec(f2bCmd); err != nil {
		return fmt.Errorf("fail2ban: %w", err)
	}

	fmt.Printf("   🔒 Configuring UFW...\n")
	ufwCmd := aptInstall(sudo, "ufw")
	ufwCmd += fmt.Sprintf(" && %sufw default deny incoming && %sufw default allow outgoing && %sufw allow 22/tcp", sudo, sudo, sudo)
	for _, port := range mf.Expose {
		ufwCmd += fmt.Sprintf(" && %sufw allow %d/tcp", sudo, port)
	}
	ufwCmd += fmt.Sprintf(" && echo 'y' | %sufw enable", sudo)
	if err := run.Exec(ufwCmd); err != nil {
		return fmt.Errorf("UFW: %w", err)
	}

	fmt.Printf("   🔒 Enabling unattended upgrades...\n")
	upgradeCmd := aptInstall(sudo, "unattended-upgrades")
	upgradeCmd += fmt.Sprintf(` && echo 'APT::Periodic::Update-Package-Lists "1";
APT::Periodic::Unattended-Upgrade "1";' | %stee /etc/apt/apt.conf.d/20auto-upgrades > /dev/null`, sudo)
	if err := run.Exec(upgradeCmd); err != nil {
		return fmt.Errorf("unattended upgrades: %w", err)
	}

	fmt.Printf("   ✅ VM hardened\n")
	return nil
}

// collectUsernames returns all usernames from users: and ssh_keys:
func collectUsernames(mf *machinefile.Machinefile) []string {
	var names []string
	for _, u := range mf.Users {
		names = append(names, u.Username)
	}
	for _, k := range mf.SSHKeys {
		names = append(names, k.Username)
	}
	return names
}

// installDocker installs Docker and adds users to docker group
func installDocker(mf *machinefile.Machinefile, run CommandRunner, sudo string) error {
	fmt.Printf("   🐳 Installing Docker...\n")
	cmd := fmt.Sprintf("%scurl -fsSL https://get.docker.com | %sbash", sudo, sudo)
	if err := run.Exec(cmd); err != nil {
		return fmt.Errorf("installing Docker: %w", err)
	}
	for _, u := range collectUsernames(mf) {
		run.Exec(fmt.Sprintf("%susermod -aG docker %s", sudo, u))
	}
	run.Exec(fmt.Sprintf("%ssystemctl enable docker && %ssystemctl start docker", sudo, sudo))
	return nil
}

// installPostgres installs PostgreSQL and creates a dev user/db
func installPostgres(mf *machinefile.Machinefile, run CommandRunner, sudo string) error {
	fmt.Printf("   🐘 Installing PostgreSQL...\n")
	if err := run.Exec(aptInstall(sudo, "postgresql postgresql-contrib libpq-dev")); err != nil {
		return fmt.Errorf("installing PostgreSQL: %w", err)
	}
	pgUser := "devuser"
	pgPass := "devpass"
	pgDB := "devdb"
	if v, ok := mf.Env["POSTGRES_USER"]; ok {
		pgUser = v
	}
	if v, ok := mf.Env["POSTGRES_PASSWORD"]; ok {
		pgPass = v
	}
	if v, ok := mf.Env["POSTGRES_DB"]; ok {
		pgDB = v
	}
	pgCmd := fmt.Sprintf(`%ssu - postgres -c "psql -tc \"SELECT 1 FROM pg_roles WHERE rolname='%s'\" | grep -q 1 || psql -c \"CREATE USER %s WITH PASSWORD '%s' CREATEDB;\"" && %ssu - postgres -c "psql -tc \"SELECT 1 FROM pg_database WHERE datname='%s'\" | grep -q 1 || psql -c \"CREATE DATABASE %s OWNER %s;\""`,
		sudo, pgUser, pgUser, pgPass, sudo, pgDB, pgDB, pgUser)
	if err := run.Exec(pgCmd); err != nil {
		fmt.Printf("   ⚠️  Postgres user/db setup failed (may already exist): %v\n", err)
	}
	return nil
}

// installNodeJS installs Node.js via NodeSource
func installNodeJS(version string, run CommandRunner, sudo string) error {
	ver := "lts"
	if strings.Contains(version, "-") {
		parts := strings.SplitN(version, "-", 2)
		if len(parts) == 2 && parts[1] != "" {
			ver = parts[1]
		}
	}
	fmt.Printf("   📦 Installing Node.js %s...\n", ver)

	var cmd string
	if ver == "lts" {
		cmd = fmt.Sprintf("%scurl -fsSL https://deb.nodesource.com/setup_lts.x | %sbash - && %s",
			sudo, sudo, aptInstall(sudo, "nodejs"))
	} else {
		cmd = fmt.Sprintf("%scurl -fsSL https://deb.nodesource.com/setup_%s.x | %sbash - && %s",
			sudo, ver, sudo, aptInstall(sudo, "nodejs"))
	}
	if err := run.Exec(cmd); err != nil {
		return fmt.Errorf("installing Node.js: %w", err)
	}
	run.Exec("node --version")
	return nil
}

// provisionSetup runs setup steps with special install handling
func provisionSetup(mf *machinefile.Machinefile, run CommandRunner, sudo string) error {
	for _, step := range mf.Setup {
		if step.Install != "" {
			pkg := strings.TrimSpace(step.Install)
			switch {
			case pkg == "docker":
				if err := installDocker(mf, run, sudo); err != nil {
					return err
				}
			case pkg == "postgres" || pkg == "postgresql":
				if err := installPostgres(mf, run, sudo); err != nil {
					return err
				}
			case pkg == "nodejs" || strings.HasPrefix(pkg, "nodejs-"):
				if err := installNodeJS(pkg, run, sudo); err != nil {
					return err
				}
			case pkg == "conduitd" || pkg == "conduit":
				// Skip — handled by provisionConduit via the conduit: block
				fmt.Printf("   🔌 conduitd: use the 'conduit:' block in Machinefile instead\n")
			default:
				fmt.Printf("   📦 Installing %s...\n", pkg)
				if err := run.Exec(aptInstall(sudo, pkg)); err != nil {
					return fmt.Errorf("installing %s: %w", pkg, err)
				}
			}
		}
		if step.Clone != nil {
			if err := cloneRepo(step.Clone, mf, run); err != nil {
				return err
			}
		}
		if step.Cmd != "" {
			fmt.Printf("   ▶️  Running: %s\n", step.Cmd)
			if err := run.Exec(step.Cmd); err != nil {
				return fmt.Errorf("running cmd: %w", err)
			}
		}
		if step.Script != "" {
			fmt.Printf("   📜 Running script: %s\n", step.Script)
			if _, err := os.Stat(step.Script); os.IsNotExist(err) {
				return fmt.Errorf("script file not found: %s", step.Script)
			}
			remotePath := fmt.Sprintf("/tmp/mach-script-%d.sh", os.Getpid())
			if err := run.CopyFile(step.Script, remotePath); err != nil {
				return fmt.Errorf("copying script %s: %w", step.Script, err)
			}
			runCmd := fmt.Sprintf("chmod +x %s && %s%s", remotePath, sudo, remotePath)
			if err := run.Exec(runCmd); err != nil {
				return fmt.Errorf("running script %s: %w", step.Script, err)
			}
		}
	}
	return nil
}

// cloneRepo handles git clone with branch and PAT support
func cloneRepo(c *machinefile.CloneSpec, mf *machinefile.Machinefile, run CommandRunner) error {
	repo := c.Repo
	fmt.Printf("   📥 Cloning %s...\n", repo)

	if c.PATEnv != "" {
		pat := mf.Env[c.PATEnv]
		if pat == "" {
			pat = os.Getenv(c.PATEnv)
		}
		if pat != "" {
			repo = strings.Replace(repo, "https://", fmt.Sprintf("https://pat:%s@", pat), 1)
		}
	}

	branch := ""
	if c.Branch != "" {
		branch = fmt.Sprintf(" -b %s", c.Branch)
	}

	cmd := fmt.Sprintf("git clone%s %s %s", branch, repo, c.Dest)
	if err := run.Exec(cmd); err != nil {
		return fmt.Errorf("cloning %s: %w", c.Repo, err)
	}

	if c.PATEnv != "" && repo != c.Repo {
		run.Exec(fmt.Sprintf("cd %s && git remote set-url origin %s", c.Dest, c.Repo))
	}
	return nil
}

// provisionServices creates systemd units for declared services
func provisionServices(mf *machinefile.Machinefile, run CommandRunner, sudo string) error {
	if len(mf.Services) == 0 {
		return nil
	}
	for _, svc := range mf.Services {
		fmt.Printf("   ⚙️  Setting up service: %s\n", svc.Name)

		if svc.Install != "" {
			fmt.Printf("   📥 Downloading %s...\n", svc.Install)
			dlCmd := fmt.Sprintf("%scurl -fsSL -o /usr/local/bin/%s '%s' && %schmod +x /usr/local/bin/%s",
				sudo, svc.Name, svc.Install, sudo, svc.Name)
			if err := run.Exec(dlCmd); err != nil {
				return fmt.Errorf("downloading %s: %w", svc.Name, err)
			}
		}

		workdir := svc.Workdir
		if workdir == "" {
			workdir = "/opt/project"
		}
		svcUser := svc.User
		if svcUser == "" {
			svcUser = "root"
		}
		restart := svc.Restart
		if restart == "" {
			restart = "always"
		}

		var envLines string
		for k, v := range svc.Env {
			envLines += fmt.Sprintf("Environment=%s=%s\n", k, v)
		}

		unit := fmt.Sprintf(`[Unit]
Description=%s
After=network.target

[Service]
Type=simple
User=%s
WorkingDirectory=%s
ExecStart=%s
Restart=%s
RestartSec=5
%s
[Install]
WantedBy=multi-user.target`, svc.Name, svcUser, workdir, svc.Cmd, restart, envLines)

		writeCmd := fmt.Sprintf("echo '%s' | %stee /etc/systemd/system/%s.service > /dev/null",
			strings.ReplaceAll(unit, "'", "'\\''"), sudo, svc.Name)
		if err := run.Exec(writeCmd); err != nil {
			return fmt.Errorf("writing systemd unit for %s: %w", svc.Name, err)
		}

		startCmd := fmt.Sprintf("%ssystemctl daemon-reload && %ssystemctl enable %s && %ssystemctl start %s",
			sudo, sudo, svc.Name, sudo, svc.Name)
		if err := run.Exec(startCmd); err != nil {
			return fmt.Errorf("starting service %s: %w", svc.Name, err)
		}
		fmt.Printf("   ✅ Service '%s' running\n", svc.Name)
	}
	return nil
}

const defaultConduitdURLBase = "https://downloads.myconduit.io/agent/latest/conduitd-linux-"

// provisionConduit downloads and installs the conduitd agent as a systemd service
func provisionConduit(mf *machinefile.Machinefile, run CommandRunner, sudo string) error {
	if mf.Conduit == nil {
		return nil
	}
	c := mf.Conduit
	fmt.Printf("   🔌 Installing conduitd agent...\n")

	dlURL := c.DownloadURL
	if dlURL == "" {
		// Auto-detect target arch by querying the remote host
		dlURL = defaultConduitdURLBase + "amd64" // default
	}

	// Stop existing conduitd if running (can't overwrite a running binary — ETXTBSY)
	run.Exec(fmt.Sprintf("%ssystemctl stop conduitd 2>/dev/null || true; %srm -f /usr/local/bin/conduitd", sudo, sudo))

	// Download the binary — detect arch on the target and pick the right URL
	dlCmd := fmt.Sprintf(
		`ARCH=$(%suname -m); `+
			`case "$ARCH" in aarch64|arm64) DL_ARCH=arm64;; *) DL_ARCH=amd64;; esac; `+
			`DL_URL='%s'; `+
			`if echo "$DL_URL" | grep -q 'amd64'; then DL_URL=$(echo "$DL_URL" | sed "s/amd64/$DL_ARCH/"); fi; `+
			`echo "   Downloading conduitd ($DL_ARCH)..." && `+
			`%scurl -fsSL -o /usr/local/bin/conduitd "$DL_URL" && %schmod +x /usr/local/bin/conduitd`,
		sudo, dlURL, sudo, sudo)
	if err := run.Exec(dlCmd); err != nil {
		return fmt.Errorf("downloading conduitd: %w", err)
	}

	gwURL := c.GatewayURL
	if gwURL == "" {
		gwURL = "wss://gateway.myconduit.io"
	}

	// Create systemd service
	unit := fmt.Sprintf(`[Unit]
Description=Conduit Agent
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/conduitd run
Restart=always
RestartSec=5
Environment=CONDUIT_DEVICE_ID=%s
Environment=CONDUIT_GATEWAY_URL=%s

[Install]
WantedBy=multi-user.target`, c.DeviceID, gwURL)

	writeCmd := fmt.Sprintf("echo '%s' | %stee /etc/systemd/system/conduitd.service > /dev/null",
		strings.ReplaceAll(unit, "'", "'\\''"), sudo)
	if err := run.Exec(writeCmd); err != nil {
		return fmt.Errorf("writing conduitd unit: %w", err)
	}

	startCmd := fmt.Sprintf("%ssystemctl daemon-reload && %ssystemctl enable conduitd && %ssystemctl start conduitd",
		sudo, sudo, sudo)
	if err := run.Exec(startCmd); err != nil {
		return fmt.Errorf("starting conduitd: %w", err)
	}

	fmt.Printf("   ✅ conduitd agent running (device: %s)\n", c.DeviceID)
	return nil
}


// provisionReverseProxy installs Caddy and configures routes
func provisionReverseProxy(mf *machinefile.Machinefile, run CommandRunner, sudo string) error {
	if mf.ReverseProxy == nil || len(mf.ReverseProxy.Routes) == 0 {
		return nil
	}
	fmt.Printf("   🌐 Setting up reverse proxy (Caddy)...\n")

	installCmd := fmt.Sprintf(`%sapt-get install -y debian-keyring debian-archive-keyring apt-transport-https curl && %scurl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | %sgpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg 2>/dev/null && %scurl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | %stee /etc/apt/sources.list.d/caddy-stable.list > /dev/null && %s`,
		sudo, sudo, sudo, sudo, sudo, aptInstall(sudo, "caddy"))
	if err := run.Exec(installCmd); err != nil {
		return fmt.Errorf("installing Caddy: %w", err)
	}

	host := ":443"
	tlsLine := "	tls internal"
	if mf.ReverseProxy.Domain != "" {
		host = mf.ReverseProxy.Domain
		tlsLine = ""
	}

	var caddyfile strings.Builder
	caddyfile.WriteString(host + " {\n")
	if tlsLine != "" {
		caddyfile.WriteString(tlsLine + "\n")
	}
	for _, route := range mf.ReverseProxy.Routes {
		if route.Path == "/" {
			caddyfile.WriteString(fmt.Sprintf("	reverse_proxy %s\n", route.Target))
		} else {
			caddyfile.WriteString(fmt.Sprintf("	handle_path %s {\n		reverse_proxy %s\n	}\n", route.Path, route.Target))
		}
	}
	caddyfile.WriteString("}\n")

	writeCmd := fmt.Sprintf("echo '%s' | %stee /etc/caddy/Caddyfile > /dev/null",
		strings.ReplaceAll(caddyfile.String(), "'", "'\\''"), sudo)
	if err := run.Exec(writeCmd); err != nil {
		return fmt.Errorf("writing Caddyfile: %w", err)
	}

	startCmd := fmt.Sprintf("%ssystemctl enable caddy && %ssystemctl restart caddy", sudo, sudo)
	if err := run.Exec(startCmd); err != nil {
		return fmt.Errorf("starting Caddy: %w", err)
	}
	fmt.Printf("   ✅ Reverse proxy configured\n")
	return nil
}

// provisionMOTD sets the message of the day
func provisionMOTD(mf *machinefile.Machinefile, run CommandRunner, ip, sudo string) error {
	if mf.MOTD == "" {
		return nil
	}
	fmt.Printf("   📝 Setting MOTD...\n")

	motd := mf.MOTD
	motd = strings.ReplaceAll(motd, "{{ .Name }}", mf.Name)
	motd = strings.ReplaceAll(motd, "{{.Name}}", mf.Name)
	motd = strings.ReplaceAll(motd, "{{ .Provider }}", mf.Provider)
	motd = strings.ReplaceAll(motd, "{{.Provider}}", mf.Provider)
	motd = strings.ReplaceAll(motd, "{{ .IP }}", ip)
	motd = strings.ReplaceAll(motd, "{{.IP}}", ip)

	cmd := fmt.Sprintf("echo '%s' | %stee /etc/motd > /dev/null && %schmod -x /etc/update-motd.d/* 2>/dev/null; true",
		strings.ReplaceAll(motd, "'", "'\\''"), sudo, sudo)
	return run.Exec(cmd)
}

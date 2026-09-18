package adapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/makemore/machine/pkg/machinefile"
)

const hostingerAPI = "https://developers.hostinger.com"

// HostingerAdapter implements Adapter for Hostinger VPS virtual machines.
type HostingerAdapter struct{ token string }

// NewHostingerAdapter creates a new Hostinger adapter.
func NewHostingerAdapter() *HostingerAdapter {
	return &HostingerAdapter{token: os.Getenv("HOSTINGER_API_TOKEN")}
}

type hostingerIP struct {
	Address string `json:"address"`
}

type hostingerVM struct {
	ID           int64         `json:"id"`
	Hostname     string        `json:"hostname"`
	State        string        `json:"state"`
	Plan         string        `json:"plan"`
	DataCenterID int64         `json:"data_center_id"`
	CPUs         int           `json:"cpus"`
	Memory       int           `json:"memory"` // MB
	Disk         int           `json:"disk"`   // MB
	IPv4         []hostingerIP `json:"ipv4"`
	Template     *struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"template"`
}

type hostingerNamedID struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Location    string `json:"location"`
	City        string `json:"city"`
	Continent   string `json:"continent"`
}

func (h *HostingerAdapter) request(method, path string, body interface{}) ([]byte, int, error) {
	if h.token == "" {
		return nil, 0, fmt.Errorf("HOSTINGER_API_TOKEN not set (export it or add it to your shell environment)")
	}
	var reqBody io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reqBody = bytes.NewReader(jsonBody)
	}
	req, err := http.NewRequest(method, hostingerAPI+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+h.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return respBody, resp.StatusCode, nil
}

func (h *HostingerAdapter) mapItemID(mf *machinefile.Machinefile) string {
	if itemID := os.Getenv("HOSTINGER_VPS_ITEM_ID"); itemID != "" {
		return itemID
	}
	memGB := parseMemoryMB(mf.Resources.Memory) / 1024
	plan := "kvm1"
	switch {
	case memGB <= 4:
		plan = "kvm1"
	case memGB <= 8:
		plan = "kvm2"
	case memGB <= 16:
		plan = "kvm4"
	default:
		plan = "kvm8"
	}
	currency := strings.ToLower(envDefault("HOSTINGER_CURRENCY", "usd"))
	period := strings.ToLower(envDefault("HOSTINGER_BILLING_PERIOD", "1m"))
	return fmt.Sprintf("hostingercom-vps-%s-%s-%s", plan, currency, period)
}

func (h *HostingerAdapter) resolveTemplateID(image string) (int64, error) {
	if id, ok, err := envOrNumeric("HOSTINGER_TEMPLATE_ID", image); ok || err != nil {
		return id, err
	}
	resp, status, err := h.request("GET", "/api/vps/v1/templates", nil)
	if err != nil {
		return 0, err
	}
	if status >= 300 {
		return 0, fmt.Errorf("listing Hostinger templates: HTTP %d: %s", status, string(resp))
	}
	var templates []hostingerNamedID
	if err := json.Unmarshal(resp, &templates); err != nil {
		return 0, err
	}
	desired := strings.ToLower(strings.TrimSpace(image))
	if desired == "" || desired == "ubuntu" {
		desired = "ubuntu 24"
	}
	return firstMatchingID(templates, desired, "template", "HOSTINGER_TEMPLATE_ID")
}

func (h *HostingerAdapter) resolveDataCenterID(region string) (int64, error) {
	if id, ok, err := envOrNumeric("HOSTINGER_DATA_CENTER_ID", region); ok || err != nil {
		return id, err
	}
	resp, status, err := h.request("GET", "/api/vps/v1/data-centers", nil)
	if err != nil {
		return 0, err
	}
	if status >= 300 {
		return 0, fmt.Errorf("listing Hostinger data centers: HTTP %d: %s", status, string(resp))
	}
	var dataCenters []hostingerNamedID
	if err := json.Unmarshal(resp, &dataCenters); err != nil {
		return 0, err
	}
	desired := strings.ToLower(strings.TrimSpace(region))
	if desired == "" {
		desired = "us"
	}
	desired = mapHostingerRegionAlias(desired)
	return firstMatchingID(dataCenters, desired, "data center", "HOSTINGER_DATA_CENTER_ID")
}

func (h *HostingerAdapter) buildSetup(mf *machinefile.Machinefile) (map[string]interface{}, error) {
	if mf.CloudInit != "" {
		return nil, fmt.Errorf("cloud_init is not supported by the Hostinger adapter; use setup/script steps instead")
	}
	templateID, err := h.resolveTemplateID(mf.Image)
	if err != nil {
		return nil, err
	}
	dataCenterID, err := h.resolveDataCenterID(mf.Region)
	if err != nil {
		return nil, err
	}
	setup := map[string]interface{}{
		"template_id":    templateID,
		"data_center_id": dataCenterID,
		"hostname":       mf.Name,
		"install_monarx": false,
		"enable_backups": envBool("HOSTINGER_ENABLE_BACKUPS", false),
	}
	if mf.SSHKey == "" {
		return nil, fmt.Errorf("no SSH key found — create one with: ssh-keygen -t ed25519")
	}
	pubKeyData, err := os.ReadFile(mf.SSHKey)
	if err != nil {
		return nil, fmt.Errorf("reading SSH key %s: %w", mf.SSHKey, err)
	}
	setup["public_key"] = map[string]string{
		"name": fmt.Sprintf("mach-%s-provisioner", mf.Name),
		"key":  strings.TrimSpace(string(pubKeyData)),
	}
	if scriptID := os.Getenv("HOSTINGER_POST_INSTALL_SCRIPT_ID"); scriptID != "" {
		id, err := strconv.ParseInt(scriptID, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("HOSTINGER_POST_INSTALL_SCRIPT_ID must be numeric")
		}
		setup["post_install_script_id"] = id
	}
	return setup, nil
}

func (h *HostingerAdapter) getVM(name string) (*hostingerVM, error) {
	resp, status, err := h.request("GET", "/api/vps/v1/virtual-machines", nil)
	if err != nil {
		return nil, err
	}
	if status >= 300 {
		return nil, fmt.Errorf("listing Hostinger VMs: HTTP %d: %s", status, string(resp))
	}
	var vms []hostingerVM
	if err := json.Unmarshal(resp, &vms); err != nil {
		return nil, err
	}
	for _, vm := range vms {
		if vm.Hostname == name || strings.TrimSuffix(vm.Hostname, ".") == name {
			return &vm, nil
		}
	}
	return nil, nil
}

func (h *HostingerAdapter) getVMByID(id int64) (*hostingerVM, error) {
	resp, status, err := h.request("GET", fmt.Sprintf("/api/vps/v1/virtual-machines/%d", id), nil)
	if err != nil {
		return nil, err
	}
	if status >= 300 {
		return nil, fmt.Errorf("getting Hostinger VM %d: HTTP %d: %s", id, status, string(resp))
	}
	var vm hostingerVM
	if err := json.Unmarshal(resp, &vm); err != nil {
		return nil, err
	}
	return &vm, nil
}

func (h *HostingerAdapter) vmByNameOrErr(name string) (*hostingerVM, error) {
	vm, err := h.getVM(name)
	if err != nil {
		return nil, err
	}
	if vm == nil {
		return nil, fmt.Errorf("Hostinger VM '%s' not found", name)
	}
	return vm, nil
}

func hostingerIPOf(vm *hostingerVM) string {
	if vm == nil || len(vm.IPv4) == 0 {
		return ""
	}
	return vm.IPv4[0].Address
}

func (h *HostingerAdapter) waitForSSH(mf *machinefile.Machinefile, id int64) (*hostingerVM, error) {
	keyPath := sshPrivateKeyPath(mf.SSHKey)
	for i := 0; i < 72; i++ {
		time.Sleep(5 * time.Second)
		vm, err := h.getVMByID(id)
		if err != nil {
			continue
		}
		ip := hostingerIPOf(vm)
		if vm.State != "running" || ip == "" {
			continue
		}
		sshArgs := []string{"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null", "-o", "ConnectTimeout=4", "-o", "BatchMode=yes"}
		if keyPath != "" {
			sshArgs = append(sshArgs, "-i", keyPath)
		}
		sshArgs = append(sshArgs, fmt.Sprintf("root@%s", ip), "echo ok")
		if output, err := exec.Command("ssh", sshArgs...).Output(); err == nil && strings.TrimSpace(string(output)) == "ok" {
			fmt.Printf("   ✅ Hostinger VM ready at %s\n", ip)
			return vm, nil
		}
	}
	return nil, fmt.Errorf("timed out waiting for Hostinger VM SSH")
}

// Create purchases/configures a Hostinger VPS when missing, or starts it when stopped.
func (h *HostingerAdapter) Create(mf *machinefile.Machinefile) error {
	vm, err := h.getVM(mf.Name)
	if err != nil {
		return err
	}
	if vm != nil {
		if vm.State == "running" {
			fmt.Printf("   Hostinger VM '%s' already running\n", mf.Name)
			return nil
		}
		fmt.Printf("   Hostinger VM '%s' exists (%s), starting...\n", mf.Name, vm.State)
		resp, status, err := h.request("POST", fmt.Sprintf("/api/vps/v1/virtual-machines/%d/start", vm.ID), nil)
		if err != nil {
			return err
		}
		if status >= 300 {
			return fmt.Errorf("starting Hostinger VM: HTTP %d: %s", status, string(resp))
		}
		_, err = h.waitForSSH(mf, vm.ID)
		return err
	}

	setup, err := h.buildSetup(mf)
	if err != nil {
		return err
	}
	body := map[string]interface{}{"item_id": h.mapItemID(mf), "setup": setup}
	if paymentMethod := os.Getenv("HOSTINGER_PAYMENT_METHOD_ID"); paymentMethod != "" {
		id, err := strconv.ParseInt(paymentMethod, 10, 64)
		if err != nil {
			return fmt.Errorf("HOSTINGER_PAYMENT_METHOD_ID must be numeric")
		}
		body["payment_method_id"] = id
	}
	fmt.Printf("   🟣 Purchasing Hostinger VPS %s in data center %v...\n", body["item_id"], setup["data_center_id"])
	resp, status, err := h.request("POST", "/api/vps/v1/virtual-machines", body)
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("purchasing Hostinger VPS: HTTP %d: %s", status, string(resp))
	}
	var result struct {
		VirtualMachine hostingerVM `json:"virtual_machine"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return err
	}
	vmID := result.VirtualMachine.ID
	if vmID == 0 {
		created, err := h.vmByNameOrErr(mf.Name)
		if err != nil {
			return err
		}
		vmID = created.ID
	}
	fmt.Printf("   📡 Hostinger VM created (ID: %d)\n", vmID)
	readyVM, err := h.waitForSSH(mf, vmID)
	if err != nil {
		return err
	}
	if len(mf.Expose) > 0 {
		if err := h.setupFirewall(mf, readyVM.ID); err != nil {
			fmt.Printf("   ⚠️  Firewall setup failed: %v\n", err)
		}
	}
	return nil
}

func (h *HostingerAdapter) execInVM(ip, command string, mf *machinefile.Machinefile) error {
	args := []string{"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null"}
	if keyPath := sshPrivateKeyPath(mf.SSHKey); keyPath != "" {
		args = append(args, "-i", keyPath)
	}
	args = append(args, fmt.Sprintf("root@%s", ip), command)
	cmd := exec.Command("ssh", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (h *HostingerAdapter) Provision(mf *machinefile.Machinefile) error {
	vm, err := h.vmByNameOrErr(mf.Name)
	if err != nil {
		return err
	}
	return provisionCloud(mf, "root", hostingerIPOf(vm), false)
}

func (h *HostingerAdapter) Run(mf *machinefile.Machinefile) error {
	vm, err := h.vmByNameOrErr(mf.Name)
	if err != nil {
		return err
	}
	for _, step := range mf.Run {
		if step.Cmd != "" {
			fmt.Printf("   ▶️  %s\n", step.Cmd)
			if err := h.execInVM(hostingerIPOf(vm), step.Cmd, mf); err != nil {
				return err
			}
		}
	}
	return nil
}

func (h *HostingerAdapter) Stop(mf *machinefile.Machinefile) error {
	vm, err := h.vmByNameOrErr(mf.Name)
	if err != nil {
		return err
	}
	resp, status, err := h.request("POST", fmt.Sprintf("/api/vps/v1/virtual-machines/%d/stop", vm.ID), nil)
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("stopping Hostinger VM: HTTP %d: %s", status, string(resp))
	}
	return nil
}

func (h *HostingerAdapter) Destroy(mf *machinefile.Machinefile) error {
	return fmt.Errorf("Hostinger API does not expose VM deletion in the public VPS endpoints; use 'mach down' to stop billing compute time if applicable, then cancel/delete in hPanel if needed")
}

func (h *HostingerAdapter) Connect(mf *machinefile.Machinefile) error {
	vm, err := h.vmByNameOrErr(mf.Name)
	if err != nil {
		return err
	}
	args := []string{"-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null"}
	if keyPath := sshPrivateKeyPath(mf.SSHKey); keyPath != "" {
		args = append(args, "-i", keyPath)
	}
	args = append(args, fmt.Sprintf("root@%s", hostingerIPOf(vm)))
	cmd := exec.Command("ssh", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (h *HostingerAdapter) Status(mf *machinefile.Machinefile) error {
	vm, err := h.vmByNameOrErr(mf.Name)
	if err != nil {
		return err
	}
	icon := "⚪"
	if vm.State == "running" {
		icon = "🟢"
	} else if vm.State == "stopped" {
		icon = "🔴"
	}
	fmt.Printf("%s  Machine: %s\n", icon, vm.Hostname)
	fmt.Printf("   Provider: Hostinger VPS\n")
	fmt.Printf("   Status:   %s\n", vm.State)
	fmt.Printf("   Plan:     %s\n", vm.Plan)
	fmt.Printf("   IP:       %s\n", hostingerIPOf(vm))
	fmt.Printf("   CPUs:     %d\n", vm.CPUs)
	fmt.Printf("   Memory:   %d MB\n", vm.Memory)
	fmt.Printf("   Disk:     %d MB\n", vm.Disk)
	return nil
}

func (h *HostingerAdapter) Info(mf *machinefile.Machinefile) (*MachineInfo, error) {
	vm, err := h.vmByNameOrErr(mf.Name)
	if err != nil {
		return nil, err
	}
	return &MachineInfo{Name: vm.Hostname, Status: vm.State, Provider: "hostinger", IP: hostingerIPOf(vm), Region: mf.Region, CPUs: vm.CPUs, MemoryMB: vm.Memory, DiskGB: vm.Disk / 1024, OS: mf.OS, Type: vm.Plan}, nil
}

func (h *HostingerAdapter) setupFirewall(mf *machinefile.Machinefile, vmID int64) error {
	fwName := fmt.Sprintf("mach-%s", mf.Name)
	fwID, existed, err := h.getFirewallID(fwName)
	if err != nil {
		return err
	}
	if !existed {
		resp, status, err := h.request("POST", "/api/vps/v1/firewall", map[string]string{"name": fwName})
		if err != nil {
			return err
		}
		if status >= 300 {
			return fmt.Errorf("creating Hostinger firewall: HTTP %d: %s", status, string(resp))
		}
		var fw struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(resp, &fw); err != nil {
			return err
		}
		fwID = fw.ID
		for _, port := range append([]int{22}, mf.Expose...) {
			if err := h.addFirewallRule(fwID, port); err != nil {
				return err
			}
		}
	}
	resp, status, err := h.request("POST", fmt.Sprintf("/api/vps/v1/firewall/%d/activate/%d", fwID, vmID), nil)
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("activating Hostinger firewall: HTTP %d: %s", status, string(resp))
	}
	fmt.Printf("   🔥 Firewall active (SSH + %d exposed port(s))\n", len(mf.Expose))
	return nil
}

func (h *HostingerAdapter) getFirewallID(name string) (int64, bool, error) {
	resp, status, err := h.request("GET", "/api/vps/v1/firewall", nil)
	if err != nil {
		return 0, false, err
	}
	if status >= 300 {
		return 0, false, fmt.Errorf("listing Hostinger firewalls: HTTP %d: %s", status, string(resp))
	}
	var firewalls []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(resp, &firewalls); err != nil {
		var wrapped struct {
			Data []struct {
				ID   int64  `json:"id"`
				Name string `json:"name"`
			} `json:"data"`
		}
		if err2 := json.Unmarshal(resp, &wrapped); err2 != nil {
			return 0, false, err
		}
		firewalls = wrapped.Data
	}
	for _, fw := range firewalls {
		if fw.Name == name {
			return fw.ID, true, nil
		}
	}
	return 0, false, nil
}

func (h *HostingerAdapter) addFirewallRule(firewallID int64, port int) error {
	body := map[string]string{"protocol": "TCP", "port": fmt.Sprintf("%d", port), "source": "any", "source_detail": "any"}
	resp, status, err := h.request("POST", fmt.Sprintf("/api/vps/v1/firewall/%d/rules", firewallID), body)
	if err != nil {
		return err
	}
	if status >= 300 {
		return fmt.Errorf("adding Hostinger firewall rule for port %d: HTTP %d: %s", port, status, string(resp))
	}
	return nil
}

func envDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		return strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
	}
	return fallback
}

func envOrNumeric(envKey, raw string) (int64, bool, error) {
	if v := os.Getenv(envKey); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		return id, true, err
	}
	if raw != "" {
		if id, err := strconv.ParseInt(raw, 10, 64); err == nil {
			return id, true, nil
		}
	}
	return 0, false, nil
}

func mapHostingerRegionAlias(region string) string {
	switch region {
	case "eu", "europe", "eu-west", "eu-central", "uk", "germany", "france", "netherlands":
		return "europe"
	case "us", "usa", "us-east", "us-west", "north-america":
		return "us"
	case "asia", "singapore", "india":
		return "asia"
	default:
		return region
	}
}

func firstMatchingID(items []hostingerNamedID, desired, kind, envKey string) (int64, error) {
	var names []string
	for _, item := range items {
		haystack := strings.ToLower(strings.Join([]string{item.Name, item.Description, item.Location, item.City, item.Continent}, " "))
		names = append(names, fmt.Sprintf("%d:%s", item.ID, strings.TrimSpace(strings.Join([]string{item.Name, item.City, item.Location}, " "))))
		if strings.Contains(haystack, desired) {
			return item.ID, nil
		}
	}
	return 0, fmt.Errorf("could not resolve Hostinger %s for %q; set %s explicitly. Available: %s", kind, desired, envKey, strings.Join(names, ", "))
}

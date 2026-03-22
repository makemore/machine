package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// MachineState persists machine info for idempotent operations
type MachineState struct {
	Name       string `json:"name"`
	Provider   string `json:"provider"`
	ServerID   string `json:"server_id,omitempty"` // provider-specific ID
	IP         string `json:"ip,omitempty"`
	Status     string `json:"status"`
	Region     string `json:"region,omitempty"`
	SSHUser    string `json:"ssh_user,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
}

// DefaultStateFile returns the default state file path for a machine name
func DefaultStateFile(name string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "machine", "state", name+".json")
}

// Load reads state from a file
func Load(path string) (*MachineState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // no state file = fresh machine
		}
		return nil, err
	}
	var s MachineState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Save writes state to a file
func Save(path string, s *MachineState) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Remove deletes a state file
func Remove(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// LoadOrDefault loads state from the given path, or uses the default path
func LoadOrDefault(stateFile, name string) (*MachineState, string) {
	if stateFile == "" {
		stateFile = DefaultStateFile(name)
	}
	s, err := Load(stateFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not read state file: %v\n", err)
	}
	return s, stateFile
}


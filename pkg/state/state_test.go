package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	s := &MachineState{
		Name:     "test-vm",
		Provider: "hetzner",
		ServerID: "12345",
		IP:       "1.2.3.4",
		Status:   "running",
		Region:   "eu-central",
		SSHUser:  "root",
	}

	if err := Save(path, s); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded == nil {
		t.Fatal("loaded state is nil")
	}
	if loaded.Name != "test-vm" {
		t.Errorf("Name = %q, want %q", loaded.Name, "test-vm")
	}
	if loaded.Provider != "hetzner" {
		t.Errorf("Provider = %q", loaded.Provider)
	}
	if loaded.ServerID != "12345" {
		t.Errorf("ServerID = %q", loaded.ServerID)
	}
	if loaded.IP != "1.2.3.4" {
		t.Errorf("IP = %q", loaded.IP)
	}
	if loaded.Status != "running" {
		t.Errorf("Status = %q", loaded.Status)
	}
}

func TestLoad_NotFound(t *testing.T) {
	s, err := Load("/nonexistent/file.json")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if s != nil {
		t.Fatalf("expected nil state for missing file, got: %+v", s)
	}
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	// Save, then remove
	Save(path, &MachineState{Name: "test", Status: "running"})
	if err := Remove(path); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	// File should be gone
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should be removed")
	}
}

func TestRemove_NotFound(t *testing.T) {
	err := Remove("/nonexistent/file.json")
	if err != nil {
		t.Fatalf("Remove nonexistent should not error, got: %v", err)
	}
}

func TestDefaultStateFile(t *testing.T) {
	path := DefaultStateFile("my-machine")
	if filepath.Base(path) != "my-machine.json" {
		t.Errorf("DefaultStateFile = %q, want **/my-machine.json", path)
	}
	if !filepath.IsAbs(path) {
		t.Errorf("DefaultStateFile should be absolute, got %q", path)
	}
}

func TestLoadOrDefault_NoFile(t *testing.T) {
	s, path := LoadOrDefault("", "nonexistent-machine")
	if s != nil {
		t.Errorf("expected nil state, got %+v", s)
	}
	if filepath.Base(path) != "nonexistent-machine.json" {
		t.Errorf("path = %q", path)
	}
}

func TestLoadOrDefault_CustomPath(t *testing.T) {
	dir := t.TempDir()
	customPath := filepath.Join(dir, "custom.json")
	Save(customPath, &MachineState{Name: "custom", Status: "stopped"})

	s, path := LoadOrDefault(customPath, "ignored")
	if path != customPath {
		t.Errorf("path = %q, want %q", path, customPath)
	}
	if s == nil || s.Name != "custom" {
		t.Errorf("expected state with name 'custom', got %+v", s)
	}
}

func TestSave_CreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c", "state.json")

	err := Save(nested, &MachineState{Name: "nested"})
	if err != nil {
		t.Fatalf("Save to nested path: %v", err)
	}

	loaded, err := Load(nested)
	if err != nil {
		t.Fatalf("Load from nested path: %v", err)
	}
	if loaded.Name != "nested" {
		t.Errorf("Name = %q", loaded.Name)
	}
}

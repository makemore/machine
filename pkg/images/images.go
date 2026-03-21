package images

import (
	"embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"
)

//go:embed all:data
var embeddedData embed.FS

const ManifestURL = "https://raw.githubusercontent.com/makemore/machine/main/data/images.yaml"

type ImageManifest struct {
	Images map[string]ImageSet `yaml:"images"`
}

type ImageSet struct {
	X86_64 ImageLocation `yaml:"x86_64"`
	Arm64  ImageLocation `yaml:"arm64"`
}

type ImageLocation struct {
	URL string `yaml:"url"`
}

// LoadManifest loads the image manifest, preferring the user's local copy
// and falling back to the embedded one.
func LoadManifest() (*ImageManifest, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return nil, fmt.Errorf("getting config path: %w", err)
	}

	manifestPath := filepath.Join(configPath, "images.yaml")

	var data []byte
	if _, err := os.Stat(manifestPath); err == nil {
		// User manifest exists, load it
		data, err = os.ReadFile(manifestPath)
		if err != nil {
			return nil, fmt.Errorf("reading user manifest: %w", err)
		}
	} else {
		// Fallback to embedded manifest
		file, err := embeddedData.Open("data/images.yaml")
		if err != nil {
			return nil, fmt.Errorf("opening embedded manifest: %w", err)
		}
		defer file.Close()
		data, err = io.ReadAll(file)
		if err != nil {
			return nil, fmt.Errorf("reading embedded manifest: %w", err)
		}
	}

	var manifest ImageManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	return &manifest, nil
}

// UpdateManifest downloads the latest manifest from the repo.
func UpdateManifest() error {
	resp, err := http.Get(ManifestURL)
	if err != nil {
		return fmt.Errorf("downloading manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status from manifest url: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading manifest body: %w", err)
	}

	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(configPath, 0755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	manifestPath := filepath.Join(configPath, "images.yaml")
	if err := os.WriteFile(manifestPath, body, 0644); err != nil {
		return fmt.Errorf("writing manifest: %w", err)
	}

	return nil
}

// ResolveImage finds the correct image URL for the current architecture.
func (m *ImageManifest) ResolveImage(name string) (string, error) {
	set, ok := m.Images[name]
	if !ok {
		// If not a friendly name, assume it's a URL
		if strings.HasPrefix(name, "http://") || strings.HasPrefix(name, "https://") {
			return name, nil
		}
		return "", fmt.Errorf("image '%s' not found in manifest", name)
	}

	arch := runtime.GOARCH
	var url string
	switch arch {
	case "x86_64":
		url = set.X86_64.URL
	case "arm64":
		url = set.Arm64.URL
	default:
		return "", fmt.Errorf("unsupported architecture: %s", arch)
	}

	if url == "" {
		return "", fmt.Errorf("no URL found for image '%s' on architecture '%s'", name, arch)
	}

	return url, nil
}

// GetConfigPath returns the path to the user's config directory.
func GetConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "machine"), nil
}

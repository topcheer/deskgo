package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigCreatesExplicitPath(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "nested", "autostart.json")

	config, loadedPath, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	if loadedPath != configPath {
		t.Fatalf("expected loaded path %q, got %q", configPath, loadedPath)
	}

	if config != DefaultConfig() {
		t.Fatalf("expected default config, got %+v", config)
	}

	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected config file to be created: %v", err)
	}
}

func TestLoadConfigUsesExplicitPathContents(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "service.json")

	custom := []byte(`{
  "server": "wss://relay.example.com/api/desktop",
  "session": "Office-Mac",
  "codec": "jpeg",
  "fps": 12
}`)
	if err := os.WriteFile(configPath, custom, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	config, loadedPath, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	if loadedPath != configPath {
		t.Fatalf("expected loaded path %q, got %q", configPath, loadedPath)
	}
	if config.Server != "wss://relay.example.com/api/desktop" {
		t.Fatalf("unexpected server: %q", config.Server)
	}
	if config.Session != "office-mac" {
		t.Fatalf("unexpected session: %q", config.Session)
	}
	if config.Codec != "jpeg" {
		t.Fatalf("unexpected codec: %q", config.Codec)
	}
	if config.FPS != 12 {
		t.Fatalf("unexpected fps: %d", config.FPS)
	}
	if config.Quality == 0 {
		t.Fatalf("expected normalization to fill quality")
	}
}

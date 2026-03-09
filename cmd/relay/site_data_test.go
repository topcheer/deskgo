package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCollectSiteDownloadsUsesGitHubReleaseFallbackForMissingDesktopGroup(t *testing.T) {
	tempDir := t.TempDir()
	for _, name := range []string{
		"deskgo-desktop-linux-amd64",
		"deskgo-desktop-windows-amd64.exe",
		"deskgo-relay-linux-amd64",
	} {
		if err := os.WriteFile(filepath.Join(tempDir, name), []byte("artifact"), 0o644); err != nil {
			t.Fatalf("write temp artifact %s: %v", name, err)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/topcheer/deskgo/releases/tags/v0.1.0" {
			t.Fatalf("unexpected release path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"assets": [
				{"name":"deskgo-desktop-darwin-amd64","browser_download_url":"https://example.com/deskgo-desktop-darwin-amd64"},
				{"name":"deskgo-desktop-darwin-arm64","browser_download_url":"https://example.com/deskgo-desktop-darwin-arm64"},
				{"name":"deskgo-desktop-linux-amd64","browser_download_url":"https://example.com/deskgo-desktop-linux-amd64"},
				{"name":"SHA256SUMS.txt","browser_download_url":"https://example.com/SHA256SUMS.txt"}
			]
		}`))
	}))
	defer server.Close()

	originalBaseURL := githubReleaseAPIBaseURL
	originalHTTPClient := githubReleaseHTTPClient
	defer func() {
		githubReleaseAPIBaseURL = originalBaseURL
		githubReleaseHTTPClient = originalHTTPClient
		resetGitHubReleaseCache()
	}()

	githubReleaseAPIBaseURL = server.URL
	githubReleaseHTTPClient = server.Client()
	resetGitHubReleaseCache()

	t.Setenv("DESKGO_RELEASE_REPOSITORY", "topcheer/deskgo")
	t.Setenv("DESKGO_RELEASE_TAG", "v0.1.0")

	data := collectSiteDownloads(tempDir)

	if !data.HasChecksums {
		t.Fatalf("expected checksum fallback to be available")
	}
	if data.ChecksumURL != "https://example.com/SHA256SUMS.txt" {
		t.Fatalf("unexpected checksum URL: %s", data.ChecksumURL)
	}

	if len(data.DesktopDownloads) != 3 {
		t.Fatalf("expected three desktop OS groups, got %d", len(data.DesktopDownloads))
	}

	darwinGroup := findDownloadGroup(data.DesktopDownloads, "darwin")
	if darwinGroup == nil {
		t.Fatalf("expected darwin desktop group from GitHub release fallback")
	}
	if len(darwinGroup.Artifacts) != 2 {
		t.Fatalf("expected two darwin artifacts, got %d", len(darwinGroup.Artifacts))
	}

	linuxGroup := findDownloadGroup(data.DesktopDownloads, "linux")
	if linuxGroup == nil {
		t.Fatalf("expected local linux desktop group")
	}
	if len(linuxGroup.Artifacts) != 1 {
		t.Fatalf("expected duplicate linux artifact to be de-duplicated, got %d", len(linuxGroup.Artifacts))
	}
}

func findDownloadGroup(groups []downloadGroup, osName string) *downloadGroup {
	for i := range groups {
		if groups[i].OS == osName {
			return &groups[i]
		}
	}
	return nil
}

func resetGitHubReleaseCache() {
	githubReleaseCache.mu.Lock()
	defer githubReleaseCache.mu.Unlock()

	githubReleaseCache.key = ""
	githubReleaseCache.fetchedAt = time.Time{}
	githubReleaseCache.artifacts = nil
	githubReleaseCache.checksumURL = ""
}

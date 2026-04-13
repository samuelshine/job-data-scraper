package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig_LoadsDotEnvFile(t *testing.T) {
	tempDir := t.TempDir()
	envPath := filepath.Join(tempDir, ".env")
	content := "JSEARCH_API_KEY=test-jsearch\nADZUNA_APP_ID=test-adzuna-id\nADZUNA_APP_KEY=test-adzuna-key\nSOURCE_FETCH_WORKERS=6\nRATE_LIMIT_ENABLED=true\nRATE_LIMIT_REQUESTS=240\nRATE_LIMIT_WINDOW=2m\nAUTH_RATE_LIMIT_REQUESTS=15\nAUTH_RATE_LIMIT_WINDOW=90s\nADMIN_RATE_LIMIT_REQUESTS=5\nADMIN_RATE_LIMIT_WINDOW=3m\nSCRAPE_BRIDGE_URL=https://scraper.example.com/search\nSCRAPE_BRIDGE_TOKEN=test-bridge-token\nSCRAPE_BRIDGE_SOURCES=linkedin|indeed\nLIVE_SYNC_QUERIES=golang developer|python developer\nLIVE_SYNC_LOCATIONS=Remote|San Francisco, CA\nLIVE_SYNC_INTERVAL=45m\nLIVE_SYNC_ON_START=false\n"

	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	defer os.Chdir(cwd)

	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}

	t.Setenv("JSEARCH_API_KEY", "")
	t.Setenv("ADZUNA_APP_ID", "")
	t.Setenv("ADZUNA_APP_KEY", "")
	t.Setenv("SOURCE_FETCH_WORKERS", "")
	t.Setenv("RATE_LIMIT_ENABLED", "")
	t.Setenv("RATE_LIMIT_REQUESTS", "")
	t.Setenv("RATE_LIMIT_WINDOW", "")
	t.Setenv("AUTH_RATE_LIMIT_REQUESTS", "")
	t.Setenv("AUTH_RATE_LIMIT_WINDOW", "")
	t.Setenv("ADMIN_RATE_LIMIT_REQUESTS", "")
	t.Setenv("ADMIN_RATE_LIMIT_WINDOW", "")
	t.Setenv("SCRAPE_BRIDGE_URL", "")
	t.Setenv("SCRAPE_BRIDGE_TOKEN", "")
	t.Setenv("SCRAPE_BRIDGE_SOURCES", "")
	t.Setenv("LIVE_SYNC_QUERIES", "")
	t.Setenv("LIVE_SYNC_LOCATIONS", "")
	t.Setenv("LIVE_SYNC_INTERVAL", "")
	t.Setenv("LIVE_SYNC_ON_START", "")

	cfg := LoadConfig()

	if cfg.JSearchAPIKey != "test-jsearch" {
		t.Fatalf("JSearchAPIKey = %q, want test-jsearch", cfg.JSearchAPIKey)
	}
	if cfg.AdzunaAppID != "test-adzuna-id" {
		t.Fatalf("AdzunaAppID = %q, want test-adzuna-id", cfg.AdzunaAppID)
	}
	if cfg.AdzunaAppKey != "test-adzuna-key" {
		t.Fatalf("AdzunaAppKey = %q, want test-adzuna-key", cfg.AdzunaAppKey)
	}
	if cfg.SourceFetchWorkers != 6 {
		t.Fatalf("SourceFetchWorkers = %d, want 6", cfg.SourceFetchWorkers)
	}
	if !cfg.RateLimitEnabled {
		t.Fatal("RateLimitEnabled = false, want true")
	}
	if cfg.RateLimitRequests != 240 {
		t.Fatalf("RateLimitRequests = %d, want 240", cfg.RateLimitRequests)
	}
	if cfg.RateLimitWindow != 2*time.Minute {
		t.Fatalf("RateLimitWindow = %s, want 2m", cfg.RateLimitWindow)
	}
	if cfg.AuthRateLimitRequests != 15 {
		t.Fatalf("AuthRateLimitRequests = %d, want 15", cfg.AuthRateLimitRequests)
	}
	if cfg.AuthRateLimitWindow != 90*time.Second {
		t.Fatalf("AuthRateLimitWindow = %s, want 90s", cfg.AuthRateLimitWindow)
	}
	if cfg.AdminRateLimitRequests != 5 {
		t.Fatalf("AdminRateLimitRequests = %d, want 5", cfg.AdminRateLimitRequests)
	}
	if cfg.AdminRateLimitWindow != 3*time.Minute {
		t.Fatalf("AdminRateLimitWindow = %s, want 3m", cfg.AdminRateLimitWindow)
	}
	if cfg.ScrapeBridgeURL != "https://scraper.example.com/search" {
		t.Fatalf("ScrapeBridgeURL = %q, want bridge URL", cfg.ScrapeBridgeURL)
	}
	if cfg.ScrapeBridgeToken != "test-bridge-token" {
		t.Fatalf("ScrapeBridgeToken = %q, want test-bridge-token", cfg.ScrapeBridgeToken)
	}
	if len(cfg.ScrapeBridgeSources) != 2 {
		t.Fatalf("ScrapeBridgeSources length = %d, want 2", len(cfg.ScrapeBridgeSources))
	}
	if len(cfg.LiveSyncQueries) != 2 {
		t.Fatalf("LiveSyncQueries length = %d, want 2", len(cfg.LiveSyncQueries))
	}
	if len(cfg.LiveSyncLocations) != 2 {
		t.Fatalf("LiveSyncLocations length = %d, want 2", len(cfg.LiveSyncLocations))
	}
	if cfg.LiveSyncInterval != 45*time.Minute {
		t.Fatalf("LiveSyncInterval = %s, want 45m", cfg.LiveSyncInterval)
	}
	if cfg.LiveSyncOnStart {
		t.Fatal("LiveSyncOnStart = true, want false")
	}
}

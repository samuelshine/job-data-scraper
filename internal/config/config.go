package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds application configuration.
type Config struct {
	Port               string
	DatabasePath       string
	JWTSecret          string
	CORSOrigins        []string
	FrontendServerURL  string
	SourceFetchWorkers int

	RateLimitEnabled       bool
	RateLimitRequests      int
	RateLimitWindow        time.Duration
	AuthRateLimitRequests  int
	AuthRateLimitWindow    time.Duration
	AdminRateLimitRequests int
	AdminRateLimitWindow   time.Duration

	// API keys for job sources
	JSearchAPIKey string
	AdzunaAppID   string
	AdzunaAppKey  string

	// Optional external scraper bridge for sources like LinkedIn/Indeed.
	ScrapeBridgeURL     string
	ScrapeBridgeToken   string
	ScrapeBridgeSources []string

	// Experimental built-in HTML scraping for public job search pages.
	BuiltInScrapersEnabled bool
	BuiltInScraperSources  []string

	// Cache settings
	CacheTTL time.Duration

	// Optional background sync settings for live ingestion
	LiveSyncQueries   []string
	LiveSyncLocations []string
	LiveSyncInterval  time.Duration
	LiveSyncOnStart   bool

	// Scraper toggles (enabled by default)
	DisableHNScraper       bool
	DisableRemoteOKScraper bool
	DisableWWRScraper      bool
	DisableJobicyScraper   bool
}

// LoadConfig reads configuration from environment variables with defaults.
func LoadConfig() *Config {
	loadDotEnv(".env")

	return &Config{
		Port:                   getEnv("PORT", "8080"),
		DatabasePath:           getEnv("DATABASE_PATH", "jobpulse.db"),
		JWTSecret:              getEnv("JWT_SECRET", "dev-secret-change-in-production"),
		CORSOrigins:            splitCSV(getEnv("CORS_ORIGINS", "http://localhost:4321,http://127.0.0.1:4321,http://localhost:8080,http://127.0.0.1:8080")),
		FrontendServerURL:      getEnv("FRONTEND_SERVER_URL", ""),
		SourceFetchWorkers:     parseIntEnv("SOURCE_FETCH_WORKERS", 4),
		RateLimitEnabled:       parseBoolEnv("RATE_LIMIT_ENABLED", true),
		RateLimitRequests:      parseIntEnv("RATE_LIMIT_REQUESTS", 120),
		RateLimitWindow:        parseDurationEnv("RATE_LIMIT_WINDOW", time.Minute),
		AuthRateLimitRequests:  parseIntEnv("AUTH_RATE_LIMIT_REQUESTS", 20),
		AuthRateLimitWindow:    parseDurationEnv("AUTH_RATE_LIMIT_WINDOW", time.Minute),
		AdminRateLimitRequests: parseIntEnv("ADMIN_RATE_LIMIT_REQUESTS", 10),
		AdminRateLimitWindow:   parseDurationEnv("ADMIN_RATE_LIMIT_WINDOW", time.Minute),
		JSearchAPIKey:          os.Getenv("JSEARCH_API_KEY"),
		AdzunaAppID:            os.Getenv("ADZUNA_APP_ID"),
		AdzunaAppKey:           os.Getenv("ADZUNA_APP_KEY"),
		ScrapeBridgeURL:        strings.TrimSpace(os.Getenv("SCRAPE_BRIDGE_URL")),
		ScrapeBridgeToken:      strings.TrimSpace(os.Getenv("SCRAPE_BRIDGE_TOKEN")),
		ScrapeBridgeSources: splitCSV(
			getEnv("SCRAPE_BRIDGE_SOURCES", "linkedin,indeed"),
		),
		BuiltInScrapersEnabled: parseBoolEnv("ENABLE_BUILTIN_SCRAPERS", false),
		BuiltInScraperSources: splitCSV(
			getEnv("BUILTIN_SCRAPER_SOURCES", "linkedin,indeed"),
		),
		CacheTTL:          24 * time.Hour,
		LiveSyncQueries:   splitCSVEnv("LIVE_SYNC_QUERIES"),
		LiveSyncLocations: splitCSVEnv("LIVE_SYNC_LOCATIONS"),
		LiveSyncInterval:  parseDurationEnv("LIVE_SYNC_INTERVAL", 30*time.Minute),
		LiveSyncOnStart:   parseBoolEnv("LIVE_SYNC_ON_START", true),

		DisableHNScraper:       parseBoolEnv("DISABLE_HN_SCRAPER", false),
		DisableRemoteOKScraper: parseBoolEnv("DISABLE_REMOTEOK_SCRAPER", false),
		DisableWWRScraper:      parseBoolEnv("DISABLE_WWR_SCRAPER", false),
		DisableJobicyScraper:   parseBoolEnv("DISABLE_JOBICY_SCRAPER", false),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitCSVEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}

	return splitCSV(raw)
}

func splitCSV(raw string) []string {
	delimiter := ","
	if strings.Contains(raw, "|") {
		delimiter = "|"
	}

	parts := strings.Split(raw, delimiter)
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}

	if len(values) == 0 {
		return nil
	}

	return values
}

func parseDurationEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	if duration, err := time.ParseDuration(raw); err == nil && duration > 0 {
		return duration
	}

	if minutes, err := strconv.Atoi(raw); err == nil && minutes > 0 {
		return time.Duration(minutes) * time.Minute
	}

	return fallback
}

func parseBoolEnv(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}

	return parsed
}

func parseIntEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}

	return parsed
}

func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		if key == "" || os.Getenv(key) != "" {
			continue
		}

		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		_ = os.Setenv(key, value)
	}
}

package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/samuelshine/job-data-scraper/internal/api"
	"github.com/samuelshine/job-data-scraper/internal/api/handlers"
	"github.com/samuelshine/job-data-scraper/internal/config"
	"github.com/samuelshine/job-data-scraper/internal/database"
	"github.com/samuelshine/job-data-scraper/internal/repository"
	"github.com/samuelshine/job-data-scraper/internal/scraper"
	"github.com/samuelshine/job-data-scraper/internal/service"
	"github.com/samuelshine/job-data-scraper/internal/sources"
	"github.com/samuelshine/job-data-scraper/internal/sources/adzuna"
	"github.com/samuelshine/job-data-scraper/internal/sources/jsearch"
	"github.com/samuelshine/job-data-scraper/internal/sources/scrapebridge"
	"github.com/samuelshine/job-data-scraper/internal/sources/webscrape"
)

func main() {
	// Load configuration
	cfg := config.LoadConfig()
	ctx := context.Background()

	// Initialize database
	db, err := database.NewDatabase(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("❌ Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Seed database with initial data (idempotent)
	if err := repository.SeedDatabase(ctx, db); err != nil {
		log.Fatalf("❌ Failed to seed database: %v", err)
	}

	// Initialize repositories
	jobRepo := repository.NewJobRepo(db)
	userRepo := repository.NewUserRepo(db)
	cacheRepo := repository.NewCacheRepo(db)
	trendsRepo := repository.NewTrendsRepo(db)
	appRepo := repository.NewApplicationRepo(db)

	// Build job source clients (conditional on API keys)
	var srcs []sources.JobSource

	if cfg.JSearchAPIKey != "" {
		srcs = append(srcs, jsearch.New(cfg.JSearchAPIKey))
		log.Printf("✅ JSearch source enabled")
	} else {
		log.Printf("⚠️  JSEARCH_API_KEY not set — JSearch source disabled")
	}

	if cfg.AdzunaAppID != "" && cfg.AdzunaAppKey != "" {
		srcs = append(srcs, adzuna.New(cfg.AdzunaAppID, cfg.AdzunaAppKey))
		log.Printf("✅ Adzuna source enabled")
	} else {
		log.Printf("⚠️  ADZUNA_APP_ID/KEY not set — Adzuna source disabled")
	}

	if cfg.ScrapeBridgeURL != "" {
		srcs = append(srcs, scrapebridge.New(cfg.ScrapeBridgeURL, cfg.ScrapeBridgeToken, cfg.ScrapeBridgeSources))
		log.Printf("✅ Scrape bridge source enabled for %v", cfg.ScrapeBridgeSources)
	} else {
		log.Printf("⚠️  SCRAPE_BRIDGE_URL not set — scrape bridge source disabled")
	}

	if cfg.BuiltInScrapersEnabled {
		for _, provider := range cfg.BuiltInScraperSources {
			source, err := webscrape.New(provider)
			if err != nil {
				log.Printf("⚠️  Built-in scraper %q disabled: %v", provider, err)
				continue
			}
			srcs = append(srcs, source)
			log.Printf("✅ Built-in scraper enabled for %s", provider)
		}
	} else {
		log.Printf("⚠️  ENABLE_BUILTIN_SCRAPERS not set — built-in HTML scraping disabled")
	}

	// Add new web scrapers
	if !cfg.DisableHNScraper {
		srcs = append(srcs, scraper.NewHNScraper())
		log.Printf("✅ HN Scraper enabled")
	}
	if !cfg.DisableRemoteOKScraper {
		srcs = append(srcs, scraper.NewRemoteOKScraper())
		log.Printf("✅ RemoteOK Scraper enabled")
	}
	if !cfg.DisableWWRScraper {
		srcs = append(srcs, scraper.NewWWRScraper())
		log.Printf("✅ WWR Scraper enabled")
	}
	if !cfg.DisableJobicyScraper {
		srcs = append(srcs, scraper.NewJobicyScraper())
		log.Printf("✅ Jobicy Scraper enabled")
	}

	// Build aggregator (nil if no sources)
	var aggregator *service.Aggregator
	if len(srcs) > 0 {
		aggregator = service.NewAggregator(srcs, jobRepo, cacheRepo, cfg.CacheTTL, cfg.SourceFetchWorkers)
		log.Printf("🔍 Aggregator enabled with %d source(s)", len(srcs))
	} else {
		log.Printf("📦 No API keys configured — using seed data only")
	}

	// Initialize services
	jobService := service.NewJobService(jobRepo, userRepo, cacheRepo, trendsRepo, aggregator)
	authService := service.NewAuthService(userRepo, cfg.JWTSecret)
	appService := service.NewApplicationService(appRepo)

	// Optionally keep the local database warm with periodic live syncs.
	liveSync := service.NewLiveSyncWorker(jobService, cfg.LiveSyncQueries, cfg.LiveSyncLocations, cfg.LiveSyncInterval)
	liveSync.Start(ctx, cfg.LiveSyncOnStart)

	// Initialize handlers
	jobHandler := handlers.NewJobHandler(jobService)
	companyHandler := handlers.NewCompanyHandler(jobService)
	analyticsHandler := handlers.NewAnalyticsHandler(jobService)
	authHandler := handlers.NewAuthHandler(authService, userRepo)
	appHandler := handlers.NewApplicationHandler(appService)

	// Create router
	router := api.NewRouter(api.RouterConfig{
		JobHandler:             jobHandler,
		CompanyHandler:         companyHandler,
		AnalyticsHandler:       analyticsHandler,
		AuthHandler:            authHandler,
		ApplicationHandler:     appHandler,
		JWTSecret:              cfg.JWTSecret,
		CORSOrigins:            cfg.CORSOrigins,
		FrontendServerURL:      cfg.FrontendServerURL,
		RateLimitEnabled:       cfg.RateLimitEnabled,
		RateLimitRequests:      cfg.RateLimitRequests,
		RateLimitWindow:        cfg.RateLimitWindow,
		AuthRateLimitRequests:  cfg.AuthRateLimitRequests,
		AuthRateLimitWindow:    cfg.AuthRateLimitWindow,
		AdminRateLimitRequests: cfg.AdminRateLimitRequests,
		AdminRateLimitWindow:   cfg.AdminRateLimitWindow,
	})

	// Start server
	addr := fmt.Sprintf(":%s", cfg.Port)
	log.Printf("🚀 JobPulse API server starting on http://localhost%s", addr)
	log.Printf("📚 API: http://localhost%s/api/v1/jobs", addr)
	log.Printf("🔐 Auth: http://localhost%s/api/v1/auth/register", addr)

	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("❌ Server failed: %v", err)
	}
}

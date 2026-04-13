package api

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/samuelshine/job-data-scraper/internal/api/handlers"
	"github.com/samuelshine/job-data-scraper/internal/api/middleware"
)

// RouterConfig holds all dependencies needed for the router.
type RouterConfig struct {
	JobHandler             *handlers.JobHandler
	CompanyHandler         *handlers.CompanyHandler
	AnalyticsHandler       *handlers.AnalyticsHandler
	AuthHandler            *handlers.AuthHandler
	ApplicationHandler     *handlers.ApplicationHandler
	JWTSecret              string
	CORSOrigins            []string
	FrontendServerURL      string
	RateLimitEnabled       bool
	RateLimitRequests      int
	RateLimitWindow        time.Duration
	AuthRateLimitRequests  int
	AuthRateLimitWindow    time.Duration
	AdminRateLimitRequests int
	AdminRateLimitWindow   time.Duration
}

// NewRouter creates the HTTP router with all routes.
func NewRouter(cfg RouterConfig) *chi.Mux {
	r := chi.NewRouter()
	apiLimiter := buildRateLimiter(cfg.RateLimitEnabled, cfg.RateLimitRequests, cfg.RateLimitWindow)
	authLimiter := buildRateLimiter(cfg.RateLimitEnabled, cfg.AuthRateLimitRequests, cfg.AuthRateLimitWindow)
	adminLimiter := buildRateLimiter(cfg.RateLimitEnabled, cfg.AdminRateLimitRequests, cfg.AdminRateLimitWindow)

	// Middleware
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Compress(5))
	r.Use(chimiddleware.StripSlashes)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORSOrigins,
		AllowedMethods:   []string{"GET", "POST", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// API docs (Swagger UI)
	r.Get("/docs", DocsHandler())
	r.Get("/openapi.json", OpenAPIHandler())

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// API v1
	r.Route("/api/v1", func(r chi.Router) {
		if apiLimiter != nil {
			r.Use(apiLimiter.Middleware)
		}

		// Public auth routes
		r.Route("/auth", func(r chi.Router) {
			if authLimiter != nil {
				r.Use(authLimiter.Middleware)
			}
			r.Post("/register", cfg.AuthHandler.Register)
			r.Post("/login", cfg.AuthHandler.Login)
			r.Post("/logout", cfg.AuthHandler.Logout)
			r.With(middleware.OptionalAuthMiddleware(cfg.JWTSecret)).Get("/session", cfg.AuthHandler.GetSession)
		})

		// Jobs (public, with optional auth for saved status)
		r.Route("/jobs", func(r chi.Router) {
			r.Use(middleware.OptionalAuthMiddleware(cfg.JWTSecret))
			r.Get("/", cfg.JobHandler.ListJobs)
			r.Get("/{id}", cfg.JobHandler.GetJob)
		})

		// Companies (public)
		r.Route("/companies", func(r chi.Router) {
			r.Get("/", cfg.CompanyHandler.ListCompanies)
			r.Get("/{slug}", cfg.CompanyHandler.GetCompany)
		})

		// Analytics (public)
		r.Route("/analytics", func(r chi.Router) {
			r.Get("/skills", cfg.AnalyticsHandler.GetTopSkills)
			r.Get("/summary", cfg.AnalyticsHandler.GetSummary)
			r.Get("/trends", cfg.AnalyticsHandler.GetMarketTrends)
			r.Get("/sources", cfg.AnalyticsHandler.GetSourceDistribution)
			r.Get("/source-health", cfg.AnalyticsHandler.GetSourceHealth)
			r.Get("/salary", cfg.AnalyticsHandler.GetSalaryStats)
			r.Post("/refresh", cfg.AnalyticsHandler.RefreshTrends)
		})

		// Filters (public)
		r.Get("/filters", cfg.JobHandler.GetFilters)

		// Protected user routes
		r.Route("/me", func(r chi.Router) {
			r.Use(middleware.AuthMiddleware(cfg.JWTSecret))
			r.Get("/", cfg.AuthHandler.GetProfile)
			r.Get("/saved-jobs", cfg.AuthHandler.GetSavedJobs)
			r.Post("/saved-jobs/{id}", cfg.AuthHandler.SaveJob)
			r.Delete("/saved-jobs/{id}", cfg.AuthHandler.UnsaveJob)

			// Application Tracker
			r.Route("/applications", func(r chi.Router) {
				r.Get("/", cfg.ApplicationHandler.ListApplications)
				r.Post("/", cfg.ApplicationHandler.CreateApplication)
				r.Patch("/{id}", cfg.ApplicationHandler.UpdateApplication)
				r.Delete("/{id}", cfg.ApplicationHandler.DeleteApplication)
			})
		})

		// Admin/Scraper management
		r.Route("/admin", func(r chi.Router) {
			if adminLimiter != nil {
				r.Use(adminLimiter.Middleware)
			}
			r.Post("/scrape/{source}", cfg.AnalyticsHandler.ScrapeSource)
		})
	})

	if cfg.FrontendServerURL != "" {
		if proxy := buildFrontendProxy(cfg.FrontendServerURL); proxy != nil {
			r.NotFound(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasPrefix(r.URL.Path, "/api/") {
					http.NotFound(w, r)
					return
				}
				proxy.ServeHTTP(w, r)
			})
		}
	}

	return r
}

func buildFrontendProxy(rawURL string) *httputil.ReverseProxy {
	target, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "frontend is unavailable", http.StatusBadGateway)
	}
	return proxy
}

func buildRateLimiter(enabled bool, requests int, window time.Duration) *middleware.RateLimiter {
	if !enabled {
		return nil
	}

	return middleware.NewRateLimiter(requests, window)
}

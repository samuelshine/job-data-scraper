package api

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/samuelshine/job-data-scraper/internal/api/handlers"
	"github.com/samuelshine/job-data-scraper/internal/api/middleware"
)

// RouterConfig holds all dependencies needed for the router.
type RouterConfig struct {
	JobHandler         *handlers.JobHandler
	CompanyHandler     *handlers.CompanyHandler
	AnalyticsHandler   *handlers.AnalyticsHandler
	AuthHandler        *handlers.AuthHandler
	ApplicationHandler *handlers.ApplicationHandler
	JWTSecret          string
	CORSOrigins        []string
	FrontendServerURL  string
}

// NewRouter creates the HTTP router with all routes.
func NewRouter(cfg RouterConfig) *chi.Mux {
	r := chi.NewRouter()

	// Middleware
<<<<<<< HEAD
	r.Use(chimiddleware.Logger)
=======
	r.Use(middleware.SlogLogger)
>>>>>>> dev-deepu
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

<<<<<<< HEAD
	// API v1
	r.Route("/api/v1", func(r chi.Router) {
		// Public auth routes
		r.Route("/auth", func(r chi.Router) {
=======
	// Rate limiters
	// Auth: 5 requests/minute, burst of 10 — prevents brute-force login attempts
	authLimiter := middleware.NewRateLimitMiddleware(5.0/60.0, 10)
	// Admin: 2 requests/minute, burst of 3 — prevents abuse of expensive scrape triggers
	adminLimiter := middleware.NewRateLimitMiddleware(2.0/60.0, 3)

	// API v1
	r.Route("/api/v1", func(r chi.Router) {
		// Public auth routes — rate limited against brute-force
		r.Route("/auth", func(r chi.Router) {
			r.Use(authLimiter)
>>>>>>> dev-deepu
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

<<<<<<< HEAD
		// Admin/Scraper management
		r.Route("/admin", func(r chi.Router) {
=======
		// Admin/Scraper management — rate limited to prevent scrape abuse
		r.Route("/admin", func(r chi.Router) {
			r.Use(adminLimiter)
>>>>>>> dev-deepu
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

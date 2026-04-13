package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/samuelshine/job-data-scraper/internal/api/handlers"
	"github.com/samuelshine/job-data-scraper/internal/database"
	"github.com/samuelshine/job-data-scraper/internal/domain"
	"github.com/samuelshine/job-data-scraper/internal/repository"
	"github.com/samuelshine/job-data-scraper/internal/service"
	"github.com/samuelshine/job-data-scraper/internal/sources"
)

const testJWTSecret = "test-secret-key-for-jwt"

type fakeSource struct {
	name string
	jobs []domain.Job
}

func (f fakeSource) Name() string { return f.name }

func (f fakeSource) Search(_ context.Context, _, _ string, _ int) ([]domain.Job, error) {
	return append([]domain.Job(nil), f.jobs...), nil
}

// testServer creates a full HTTP test server with seeded data.
func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	db, err := database.NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Create repos and services
	jobRepo := repository.NewJobRepo(db)
	userRepo := repository.NewUserRepo(db)
	cacheRepo := repository.NewCacheRepo(db)
	trendsRepo := repository.NewTrendsRepo(db)

	jobSvc := service.NewJobService(jobRepo, userRepo, cacheRepo, trendsRepo, nil)
	authSvc := service.NewAuthService(userRepo, testJWTSecret)

	// Create handlers
	jobHandler := handlers.NewJobHandler(jobSvc)
	companyHandler := handlers.NewCompanyHandler(jobSvc)
	analyticsHandler := handlers.NewAnalyticsHandler(jobSvc)
	authHandler := handlers.NewAuthHandler(authSvc, userRepo)

	// Create router
	router := NewRouter(RouterConfig{
		JobHandler:       jobHandler,
		CompanyHandler:   companyHandler,
		AnalyticsHandler: analyticsHandler,
		AuthHandler:      authHandler,
		JWTSecret:        testJWTSecret,
		CORSOrigins:      []string{"*"},
	})

	// Seed test data
	ctx := context.Background()
	intPtr := func(v int) *int { return &v }
	now := time.Now()

	jobs := []domain.Job{
		{
			ID: "j1", Title: "Go Developer", Description: "Build Go services",
			Company: "TechCo", CompanySlug: "techco", Location: "San Francisco, CA",
			SalaryMin: intPtr(150000), SalaryMax: intPtr(200000), SalaryCurrency: "USD",
			PostedAt: now.Add(-24 * time.Hour), Source: "linkedin", SourceURL: "https://linkedin.com/j1",
			Skills: domain.StringSlice{"Go", "Docker"}, IsRemote: true,
			EmploymentType: "full-time", ExperienceLevel: "senior",
		},
		{
			ID: "j2", Title: "React Developer", Description: "Frontend work",
			Company: "WebCo", CompanySlug: "webco", Location: "New York, NY",
			SalaryMin: intPtr(100000), SalaryMax: intPtr(140000), SalaryCurrency: "USD",
			PostedAt: now.Add(-48 * time.Hour), Source: "indeed", SourceURL: "https://indeed.com/j2",
			Skills: domain.StringSlice{"React", "TypeScript"}, IsRemote: false,
			EmploymentType: "full-time", ExperienceLevel: "mid",
		},
	}
	for i := range jobs {
		jobRepo.UpsertJob(ctx, &jobs[i])
	}
	jobRepo.UpsertCompany(ctx, &domain.Company{
		Slug: "techco", Name: "TechCo", Industry: "Tech",
		Description: "Test company", Website: "https://techco.com", JobCount: 1,
	})

	return httptest.NewServer(router)
}

func testServerWithSources(t *testing.T, srcs []sources.JobSource) *httptest.Server {
	t.Helper()
	db, err := database.NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	jobRepo := repository.NewJobRepo(db)
	userRepo := repository.NewUserRepo(db)
	cacheRepo := repository.NewCacheRepo(db)
	trendsRepo := repository.NewTrendsRepo(db)
	aggregator := service.NewAggregator(srcs, jobRepo, cacheRepo, time.Hour, 2)

	jobSvc := service.NewJobService(jobRepo, userRepo, cacheRepo, trendsRepo, aggregator)
	authSvc := service.NewAuthService(userRepo, testJWTSecret)

	router := NewRouter(RouterConfig{
		JobHandler:       handlers.NewJobHandler(jobSvc),
		CompanyHandler:   handlers.NewCompanyHandler(jobSvc),
		AnalyticsHandler: handlers.NewAnalyticsHandler(jobSvc),
		AuthHandler:      handlers.NewAuthHandler(authSvc, userRepo),
		JWTSecret:        testJWTSecret,
		CORSOrigins:      []string{"*"},
	})

	return httptest.NewServer(router)
}

// registerAndLogin registers a user and returns the JWT token.
func registerAndLogin(t *testing.T, server *httptest.Server) string {
	t.Helper()
	body, _ := json.Marshal(domain.RegisterRequest{
		Email: "test@example.com", Password: "password123", Name: "Test User",
	})
	resp, err := http.Post(server.URL+"/api/v1/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("register request failed: %v", err)
	}
	defer resp.Body.Close()

	var result domain.AuthResponse
	json.NewDecoder(resp.Body).Decode(&result)
	if result.Token == "" {
		t.Fatal("no token returned from register")
	}
	return result.Token
}

func TestAPI_Health(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestAPI_ListJobs(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/jobs/")
	if err != nil {
		t.Fatalf("list jobs failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatal("expected 'data' array in response")
	}
	if len(data) != 2 {
		t.Errorf("data len = %d, want 2", len(data))
	}

	pagination, ok := result["pagination"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'pagination' object in response")
	}
	if pagination["totalItems"].(float64) != 2 {
		t.Errorf("totalItems = %v, want 2", pagination["totalItems"])
	}
}

func TestAPI_ListJobs_WithFilters(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/jobs/?source=linkedin")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	data := result["data"].([]interface{})
	if len(data) != 1 {
		t.Errorf("filtered data len = %d, want 1", len(data))
	}
}

func TestAPI_ListJobs_RefreshesFromSourcesOnSearch(t *testing.T) {
	intPtr := func(v int) *int { return &v }
	now := time.Now()
	ts := testServerWithSources(t, []sources.JobSource{
		fakeSource{
			name: "bridge",
			jobs: []domain.Job{
				{
					ID: "live-1", Title: "ML Engineer", Description: "Models and inference",
					Company: "AI India", CompanySlug: "ai-india", Location: "Bengaluru, India",
					SalaryMin: intPtr(120000), SalaryMax: intPtr(180000), SalaryCurrency: "USD",
					PostedAt: now, Source: "linkedin", SourceURL: "https://example.com/jobs/live-1",
					Skills: domain.StringSlice{"Python", "PyTorch"}, IsRemote: true,
					EmploymentType: "full-time", ExperienceLevel: "mid",
				},
			},
		},
	})
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/jobs/?q=ML+Engineer")
	if err != nil {
		t.Fatalf("list jobs failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	data := result["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("data len = %d, want 1", len(data))
	}

	first := data[0].(map[string]interface{})
	if first["title"] != "ML Engineer" {
		t.Fatalf("title = %v, want ML Engineer", first["title"])
	}
}

func TestAPI_Analytics_SourceHealth(t *testing.T) {
	now := time.Now()
	ts := testServerWithSources(t, []sources.JobSource{
		fakeSource{
			name: "bridge",
			jobs: []domain.Job{
				{
					ID: "live-1", Title: "Data Engineer", Description: "Pipelines",
					Company: "DataCo", CompanySlug: "dataco", Location: "Remote",
					PostedAt: now, Source: "bridge", SourceURL: "https://example.com/jobs/live-1",
					EmploymentType: "full-time", ExperienceLevel: "mid",
				},
			},
		},
	})
	defer ts.Close()

	_, err := http.Get(ts.URL + "/api/v1/jobs/?q=Data+Engineer")
	if err != nil {
		t.Fatalf("live search failed: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/v1/analytics/source-health")
	if err != nil {
		t.Fatalf("source health request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result map[string][]map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if len(result["data"]) != 1 {
		t.Fatalf("data len = %d, want 1", len(result["data"]))
	}

	if result["data"][0]["name"] != "bridge" {
		t.Fatalf("source name = %v, want bridge", result["data"][0]["name"])
	}
}

func TestAPI_AuthRateLimit(t *testing.T) {
	db, err := database.NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	defer db.Close()

	jobRepo := repository.NewJobRepo(db)
	userRepo := repository.NewUserRepo(db)
	cacheRepo := repository.NewCacheRepo(db)
	trendsRepo := repository.NewTrendsRepo(db)

	jobSvc := service.NewJobService(jobRepo, userRepo, cacheRepo, trendsRepo, nil)
	authSvc := service.NewAuthService(userRepo, testJWTSecret)

	router := NewRouter(RouterConfig{
		JobHandler:             handlers.NewJobHandler(jobSvc),
		CompanyHandler:         handlers.NewCompanyHandler(jobSvc),
		AnalyticsHandler:       handlers.NewAnalyticsHandler(jobSvc),
		AuthHandler:            handlers.NewAuthHandler(authSvc, userRepo),
		JWTSecret:              testJWTSecret,
		CORSOrigins:            []string{"*"},
		RateLimitEnabled:       true,
		RateLimitRequests:      100,
		RateLimitWindow:        time.Minute,
		AuthRateLimitRequests:  1,
		AuthRateLimitWindow:    time.Minute,
		AdminRateLimitRequests: 1,
		AdminRateLimitWindow:   time.Minute,
	})

	ts := httptest.NewServer(router)
	defer ts.Close()

	client := &http.Client{}
	for attempt := range 2 {
		req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/auth/session", nil)
		if err != nil {
			t.Fatalf("NewRequest failed: %v", err)
		}
		req.Header.Set("X-Forwarded-For", "203.0.113.10")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request %d failed: %v", attempt+1, err)
		}
		resp.Body.Close()

		if attempt == 0 && resp.StatusCode != http.StatusOK {
			t.Fatalf("first status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		if attempt == 1 && resp.StatusCode != http.StatusTooManyRequests {
			t.Fatalf("second status = %d, want %d", resp.StatusCode, http.StatusTooManyRequests)
		}
	}
}

func TestAPI_GetJob(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/jobs/j1")
	if err != nil {
		t.Fatalf("get job failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var job domain.Job
	json.NewDecoder(resp.Body).Decode(&job)
	if job.Title != "Go Developer" {
		t.Errorf("Title = %q, want %q", job.Title, "Go Developer")
	}
}

func TestAPI_GetJob_NotFound(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/jobs/nonexistent")
	if err != nil {
		t.Fatalf("get job failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNotFound)
	}
}

func TestAPI_ListCompanies(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/companies/")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestAPI_GetCompany(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/companies/techco")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	company := result["company"].(map[string]interface{})
	if company["name"] != "TechCo" {
		t.Errorf("company name = %v, want TechCo", company["name"])
	}
}

func TestAPI_Analytics_Summary(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/analytics/summary")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestAPI_Analytics_Skills(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/analytics/skills")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestAPI_Filters(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/filters")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestAPI_Auth_RegisterAndLogin(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Register
	regBody, _ := json.Marshal(domain.RegisterRequest{
		Email: "new@example.com", Password: "password123", Name: "New User",
	})
	regResp, err := http.Post(ts.URL+"/api/v1/auth/register", "application/json", bytes.NewReader(regBody))
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	defer regResp.Body.Close()

	if regResp.StatusCode != http.StatusCreated {
		t.Errorf("register status = %d, want %d", regResp.StatusCode, http.StatusCreated)
	}

	// Login
	loginBody, _ := json.Marshal(domain.LoginRequest{
		Email: "new@example.com", Password: "password123",
	})
	loginResp, err := http.Post(ts.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	defer loginResp.Body.Close()

	if loginResp.StatusCode != http.StatusOK {
		t.Errorf("login status = %d, want %d", loginResp.StatusCode, http.StatusOK)
	}

	var result domain.AuthResponse
	json.NewDecoder(loginResp.Body).Decode(&result)
	if result.Token == "" {
		t.Error("expected token in login response")
	}
}

func TestAPI_Auth_DuplicateRegister(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	body, _ := json.Marshal(domain.RegisterRequest{
		Email: "dup@example.com", Password: "password123", Name: "Dup",
	})

	http.Post(ts.URL+"/api/v1/auth/register", "application/json", bytes.NewReader(body))
	resp, err := http.Post(ts.URL+"/api/v1/auth/register", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("duplicate register request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate register status = %d, want %d", resp.StatusCode, http.StatusConflict)
	}
}

func TestAPI_Auth_LoginWrongPassword(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	// Register first
	regBody, _ := json.Marshal(domain.RegisterRequest{
		Email: "user@example.com", Password: "correct123", Name: "User",
	})
	http.Post(ts.URL+"/api/v1/auth/register", "application/json", bytes.NewReader(regBody))

	// Login with wrong password
	body, _ := json.Marshal(domain.LoginRequest{
		Email: "user@example.com", Password: "wrong",
	})
	resp, err := http.Post(ts.URL+"/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong password status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestAPI_ProtectedEndpoint_NoAuth(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/v1/me/")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestAPI_ProtectedEndpoint_WithAuth(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	token := registerAndLogin(t, ts)

	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/me/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("profile request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestAPI_SaveAndUnsaveJob(t *testing.T) {
	ts := testServer(t)
	defer ts.Close()

	token := registerAndLogin(t, ts)

	// Save job
	req, _ := http.NewRequest("POST", ts.URL+"/api/v1/me/saved-jobs/j1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("save job failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("save status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// List saved jobs
	req2, _ := http.NewRequest("GET", ts.URL+"/api/v1/me/saved-jobs", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	resp2, err2 := http.DefaultClient.Do(req2)
	if err2 != nil {
		t.Fatalf("list saved jobs failed: %v", err2)
	}
	defer resp2.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&result)
	data := result["data"].([]interface{})
	if len(data) != 1 {
		t.Errorf("saved jobs len = %d, want 1", len(data))
	}

	// Unsave job
	req3, _ := http.NewRequest("DELETE", ts.URL+"/api/v1/me/saved-jobs/j1", nil)
	req3.Header.Set("Authorization", "Bearer "+token)
	resp3, err3 := http.DefaultClient.Do(req3)
	if err3 != nil {
		t.Fatalf("unsave job failed: %v", err3)
	}
	defer resp3.Body.Close()

	if resp3.StatusCode != http.StatusOK {
		t.Errorf("unsave status = %d, want %d", resp3.StatusCode, http.StatusOK)
	}
}

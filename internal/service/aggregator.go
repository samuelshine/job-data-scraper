package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/samuelshine/job-data-scraper/internal/domain"
	"github.com/samuelshine/job-data-scraper/internal/repository"
	"github.com/samuelshine/job-data-scraper/internal/sources"
)

// Aggregator fans out search requests to multiple job sources,
// deduplicates results, and persists them via the repository layer.
type Aggregator struct {
	sources   []sources.JobSource
	jobRepo   *repository.JobRepo
	cacheRepo *repository.CacheRepo
	cacheTTL  time.Duration
	workers   int
	mu        sync.RWMutex
	statuses  map[string]domain.SourceHealth
}

// NewAggregator creates a new aggregator.
func NewAggregator(srcs []sources.JobSource, jobRepo *repository.JobRepo, cacheRepo *repository.CacheRepo, cacheTTL time.Duration, workers int) *Aggregator {
	if workers <= 0 {
		workers = 1
	}

	return &Aggregator{
		sources:   srcs,
		jobRepo:   jobRepo,
		cacheRepo: cacheRepo,
		cacheTTL:  cacheTTL,
		workers:   workers,
		statuses:  make(map[string]domain.SourceHealth, len(srcs)),
	}
}

// sourceResult holds results from a single source goroutine.
type sourceResult struct {
	source   string
	jobs     []domain.Job
	err      error
	query    string
	started  time.Time
	finished time.Time
}

// SourceHealth returns the latest status for each configured source.
func (a *Aggregator) SourceHealth(ctx context.Context) []domain.SourceHealth {
	a.mu.RLock()
	defer a.mu.RUnlock()

	statuses := make([]domain.SourceHealth, 0, len(a.sources))
	for _, src := range a.sources {
		status := a.statuses[src.Name()]
		status.Name = src.Name()
		status.Enabled = true

		// Fetch total yield from database for this source
		if count, err := a.jobRepo.GetJobCountBySource(ctx, src.Name()); err == nil {
			status.ResultCount = count
		}

		statuses = append(statuses, status)
	}

	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].Name < statuses[j].Name
	})

	return statuses
}

// SearchAndStore fans out to all sources, deduplicates, and persists results.
// If forceRefresh is false and the cache is fresh, it returns stored data without calling APIs.
func (a *Aggregator) SearchAndStore(ctx context.Context, query, location string, page int, forceRefresh bool) ([]domain.Job, error) {
	cacheKey := buildCacheKey(query, location, page)

	// Check cache freshness
	fresh := false
	if !forceRefresh {
		var err error
		fresh, err = a.cacheRepo.IsCacheFresh(ctx, cacheKey, a.cacheTTL)
		if err != nil {
			log.Printf("⚠️  Cache check failed: %v", err)
		}
	}
	if fresh {
		log.Printf("📦 Cache hit for %q (fresh)", cacheKey)
		// Return from database
		params := domain.JobQueryParams{Query: query, Location: location}
		pag := domain.Pagination{Page: 1, Limit: 20}
		jobs, _, err := a.jobRepo.ListJobs(ctx, params, pag)
		if err != nil {
			return nil, fmt.Errorf("aggregator: failed to read cached jobs: %w", err)
		}
		// Convert summaries back to full jobs for consistency
		fullJobs := make([]domain.Job, 0, len(jobs))
		for _, s := range jobs {
			j, err := a.jobRepo.GetJob(ctx, s.ID)
			if err == nil && j != nil {
				fullJobs = append(fullJobs, *j)
			}
		}
		return fullJobs, nil
	}

	if forceRefresh {
		log.Printf("🔄 Force refresh for %q, fetching from %d sources", cacheKey, len(a.sources))
	} else {
		log.Printf("🔍 Cache miss for %q, fetching from %d sources", cacheKey, len(a.sources))
	}

	if len(a.sources) == 0 {
		return nil, fmt.Errorf("aggregator: no sources configured")
	}

	fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	pool := NewWorkerPool[sources.JobSource, sourceResult](a.workers)
	results := pool.Run(fetchCtx, a.sources, func(ctx context.Context, s sources.JobSource) sourceResult {
		started := time.Now()
		jobs, err := s.Search(ctx, query, location, page)
		return sourceResult{
			source:   s.Name(),
			jobs:     jobs,
			err:      err,
			query:    query,
			started:  started,
			finished: time.Now(),
		}
	})

	// Fan-in: collect results
	var allJobs []domain.Job
	var errors []string
	for _, res := range results {
		a.updateSourceHealth(res)
		if res.err != nil {
			log.Printf("⚠️  Source %q failed: %v", res.source, res.err)
			errors = append(errors, fmt.Sprintf("%s: %v", res.source, res.err))
			continue
		}
		log.Printf("✅ Source %q returned %d jobs", res.source, len(res.jobs))
		allJobs = append(allJobs, res.jobs...)
	}

	// If all sources failed, try returning cached data
	if len(allJobs) == 0 && len(errors) > 0 {
		log.Printf("⚠️  All sources failed, attempting cached fallback")
		params := domain.JobQueryParams{Query: query, Location: location}
		pag := domain.Pagination{Page: 1, Limit: 20}
		cached, _, err := a.jobRepo.ListJobs(ctx, params, pag)
		if err == nil && len(cached) > 0 {
			log.Printf("📦 Returning %d cached results as fallback", len(cached))
			fullJobs := make([]domain.Job, 0, len(cached))
			for _, s := range cached {
				j, err := a.jobRepo.GetJob(ctx, s.ID)
				if err == nil && j != nil {
					fullJobs = append(fullJobs, *j)
				}
			}
			return fullJobs, nil
		}
		return nil, fmt.Errorf("aggregator: all sources failed: %s", strings.Join(errors, "; "))
	}

	// Deduplicate
	deduped := dedup(allJobs)
	log.Printf("📊 %d total → %d unique after dedup", len(allJobs), len(deduped))

	// Persist to database
	if err := a.jobRepo.UpsertJobs(ctx, deduped); err != nil {
		log.Printf("⚠️  Failed to persist jobs: %v", err)
		// Still return results even if persistence fails
	} else {
		// Upsert companies
		companies := extractCompanies(deduped)
		for _, co := range companies {
			if err := a.jobRepo.UpsertCompany(ctx, &co); err != nil {
				log.Printf("⚠️  Failed to upsert company %s: %v", co.Slug, err)
			}
		}
	}

	// Update cache entry
	cacheEntry := &domain.SearchCacheEntry{
		QueryHash:   cacheKey,
		QueryText:   query,
		ResultCount: len(deduped),
	}
	if err := a.cacheRepo.SetCacheEntry(ctx, cacheEntry); err != nil {
		log.Printf("⚠️  Failed to update cache entry: %v", err)
	}

	return deduped, nil
}

// ScrapeSource triggers a focused fetch for a specific named source.
func (a *Aggregator) ScrapeSource(ctx context.Context, sourceName string) ([]domain.Job, error) {
	a.mu.RLock()
	var targeted sources.JobSource
	for _, s := range a.sources {
		if s.Name() == sourceName {
			targeted = s
			break
		}
	}
	a.mu.RUnlock()

	if targeted == nil {
		return nil, fmt.Errorf("source %q not found or disabled", sourceName)
	}

	started := time.Now()
	// Scraper-type sources return broad results without a query filter.
	// API-based sources need a query or they return errors.
	query := ""
	location := ""
	switch sourceName {
	case "jsearch", "adzuna":
		query = "developer"
	}
	jobs, err := targeted.Search(ctx, query, location, 1)

	a.updateSourceHealth(sourceResult{
		source:   targeted.Name(),
		jobs:     jobs,
		err:      err,
		query:    "manual_refresh",
		started:  started,
		finished: time.Now(),
	})

	if err != nil {
		return nil, err
	}

	// Persist results
	deduped := dedup(jobs)
	if err := a.jobRepo.UpsertJobs(ctx, deduped); err != nil {
		log.Printf("⚠️  ScrapeSource: failed to persist jobs: %v", err)
	}

	return deduped, nil
}

// dedup removes duplicate jobs by canonical URL or normalized job identity and
// prefers the richer record when duplicates overlap.
func dedup(jobs []domain.Job) []domain.Job {
	seen := make(map[string]int)
	result := make([]domain.Job, 0, len(jobs))
	for _, j := range jobs {
		keys := dedupeKeys(j)
		matchedIndex := -1
		for _, key := range keys {
			if index, ok := seen[key]; ok {
				matchedIndex = index
				break
			}
		}

		if matchedIndex >= 0 {
			result[matchedIndex] = mergeJobs(result[matchedIndex], j)
			for _, key := range dedupeKeys(result[matchedIndex]) {
				seen[key] = matchedIndex
			}
			continue
		}

		result = append(result, j)
		index := len(result) - 1
		for _, key := range keys {
			seen[key] = index
		}
	}
	return result
}

// extractCompanies extracts unique companies from a slice of jobs.
func extractCompanies(jobs []domain.Job) []domain.Company {
	seen := make(map[string]bool)
	companies := []domain.Company{}
	for _, j := range jobs {
		if j.CompanySlug == "" || seen[j.CompanySlug] {
			continue
		}
		seen[j.CompanySlug] = true
		companies = append(companies, domain.Company{
			Slug:     j.CompanySlug,
			Name:     j.Company,
			JobCount: 1,
		})
	}
	return companies
}

// buildCacheKey creates a deterministic cache key from search parameters.
func buildCacheKey(query, location string, page int) string {
	raw := fmt.Sprintf("search:%s:%s:%d", strings.ToLower(query), strings.ToLower(location), page)
	h := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", h[:8])
}

func (a *Aggregator) updateSourceHealth(res sourceResult) {
	a.mu.Lock()
	defer a.mu.Unlock()

	status := a.statuses[res.source]
	status.Name = res.source
	status.LastQuery = res.query
	status.ResultCount = len(res.jobs)
	status.LastDuration = res.finished.Sub(res.started).Round(time.Millisecond).String()
	status.LastAttemptAt = timePtr(res.finished)

	if res.err != nil {
		status.Healthy = false
		status.LastError = res.err.Error()
	} else {
		status.Healthy = true
		status.LastError = ""
		status.LastSuccessAt = timePtr(res.finished)
	}

	a.statuses[res.source] = status
}

func timePtr(value time.Time) *time.Time {
	return &value
}

func dedupeKeys(job domain.Job) []string {
	keys := make([]string, 0, 3)

	if sourceURL := normalizeSourceURL(job.SourceURL); sourceURL != "" {
		keys = append(keys, "url:"+sourceURL)
	}

	title := normalizeJobIdentityPart(job.Title)
	company := normalizeJobIdentityPart(job.Company)
	location := normalizeJobIdentityPart(job.Location)
	kind := normalizeJobIdentityPart(job.EmploymentType)
	postedDay := ""
	if !job.PostedAt.IsZero() {
		postedDay = job.PostedAt.UTC().Format("2006-01-02")
	}

	if title != "" && company != "" && location != "" {
		keys = append(keys, "identity:"+strings.Join([]string{title, company, location, kind}, "|"))
	}

	if title != "" && company != "" && postedDay != "" {
		keys = append(keys, "posting:"+strings.Join([]string{title, company, postedDay}, "|"))
	}

	return keys
}

func mergeJobs(current, candidate domain.Job) domain.Job {
	if jobQualityScore(candidate) > jobQualityScore(current) {
		current, candidate = candidate, current
	}

	if current.ID == "" {
		current.ID = candidate.ID
	}
	if current.CompanySlug == "" {
		current.CompanySlug = candidate.CompanySlug
	}
	if current.SourceURL == "" {
		current.SourceURL = candidate.SourceURL
	}
	if current.Description == "" || len(candidate.Description) > len(current.Description) {
		current.Description = candidate.Description
	}
	if current.Location == "" {
		current.Location = candidate.Location
	}
	if current.EmploymentType == "" {
		current.EmploymentType = candidate.EmploymentType
	}
	if current.ExperienceLevel == "" {
		current.ExperienceLevel = candidate.ExperienceLevel
	}
	if current.SalaryMin == nil {
		current.SalaryMin = candidate.SalaryMin
	}
	if current.SalaryMax == nil {
		current.SalaryMax = candidate.SalaryMax
	}
	if current.SalaryCurrency == "" {
		current.SalaryCurrency = candidate.SalaryCurrency
	}
	if current.PostedAt.IsZero() || (!candidate.PostedAt.IsZero() && candidate.PostedAt.Before(current.PostedAt)) {
		current.PostedAt = candidate.PostedAt
	}
	if current.ExpiresAt == nil {
		current.ExpiresAt = candidate.ExpiresAt
	}
	current.IsRemote = current.IsRemote || candidate.IsRemote
	current.Skills = mergeSkills(current.Skills, candidate.Skills)

	return current
}

func mergeSkills(left, right domain.StringSlice) domain.StringSlice {
	if len(left) == 0 {
		return append(domain.StringSlice(nil), right...)
	}
	if len(right) == 0 {
		return left
	}

	seen := make(map[string]struct{}, len(left)+len(right))
	merged := make(domain.StringSlice, 0, len(left)+len(right))
	for _, skill := range append(append(domain.StringSlice(nil), left...), right...) {
		normalized := normalizeJobIdentityPart(skill)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		merged = append(merged, skill)
	}

	return merged
}

func jobQualityScore(job domain.Job) int {
	score := 0
	if strings.TrimSpace(job.ID) != "" {
		score += 1
	}
	if strings.TrimSpace(job.SourceURL) != "" {
		score += 3
	}
	if strings.TrimSpace(job.Description) != "" {
		score += min(len(strings.TrimSpace(job.Description))/120, 5)
	}
	if job.SalaryMin != nil {
		score += 2
	}
	if job.SalaryMax != nil {
		score += 2
	}
	if len(job.Skills) > 0 {
		score += min(len(job.Skills), 4)
	}
	if !job.PostedAt.IsZero() {
		score += 1
	}
	if strings.TrimSpace(job.Location) != "" {
		score += 1
	}
	if strings.TrimSpace(job.ExperienceLevel) != "" {
		score += 1
	}

	return score
}

func normalizeSourceURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return strings.TrimRight(strings.ToLower(raw), "/")
	}

	parsed.Scheme = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	host := strings.ToLower(parsed.Host)
	path := strings.TrimRight(strings.ToLower(parsed.EscapedPath()), "/")
	if path == "" {
		path = "/"
	}

	return host + path
}

func normalizeJobIdentityPart(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return ""
	}

	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}

	return strings.Trim(b.String(), "-")
}

package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/samuelshine/job-data-scraper/internal/domain"
	"github.com/samuelshine/job-data-scraper/internal/sources"
)

type RemoteOKScraper struct {
	client *http.Client
}

func NewRemoteOKScraper() *RemoteOKScraper {
	return &RemoteOKScraper{
		client: &http.Client{Timeout: 25 * time.Second},
	}
}

func (s *RemoteOKScraper) Name() string {
	return "remoteok"
}

// remoteOKJob represents the JSON structure from RemoteOK API
type remoteOKJob struct {
	Slug        string   `json:"slug"`
	ID          string   `json:"id"`
	Date        string   `json:"date"`
	Company     string   `json:"company"`
	Position    string   `json:"position"`
	Tags        []string `json:"tags"`
	Description string   `json:"description"`
	Location    string   `json:"location"`
	SalaryMin   int      `json:"salary_min"`
	SalaryMax   int      `json:"salary_max"`
	URL         string   `json:"url"`
}

func (s *RemoteOKScraper) Search(ctx context.Context, query, location string, page int) ([]domain.Job, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://remoteok.com/api", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remoteok api returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	rawBody := string(body)
	if len(rawBody) > 500 {
		rawBody = rawBody[:500] + "..."
	}
	log.Printf("[RemoteOK] Raw Response (truncated): %s", rawBody)

	var rawJobs []interface{}
	if err := json.Unmarshal(body, &rawJobs); err != nil {
		return nil, err
	}

	log.Printf("[RemoteOK] Received %d raw items", len(rawJobs))

	// Skip the first element (legal notice)
	if len(rawJobs) <= 1 {
		return nil, nil
	}

	var jobs []domain.Job
	for i := 1; i < len(rawJobs); i++ {
		jobData, err := json.Marshal(rawJobs[i])
		if err != nil {
			continue
		}

		var rj remoteOKJob
		if err := json.Unmarshal(jobData, &rj); err != nil {
			continue
		}

		// Basic filtering based on query
		if query != "" {
			match := strings.Contains(strings.ToLower(rj.Position), strings.ToLower(query)) ||
				strings.Contains(strings.ToLower(rj.Company), strings.ToLower(query)) ||
				strings.Join(rj.Tags, " ") != "" && strings.Contains(strings.ToLower(strings.Join(rj.Tags, " ")), strings.ToLower(query))
			if !match {
				continue
			}
		}

		postedAt := time.Now()
		if rj.Date != "" {
			if t, err := time.Parse(time.RFC3339, rj.Date); err == nil {
				postedAt = t
			}
		}

		extURL := rj.URL
		if !strings.HasPrefix(extURL, "http") && rj.URL != "" {
			extURL = "https://remoteok.com" + rj.URL
		}

		if !IsValidJobURL(extURL) || rj.URL == "" {
			extURL = "https://remoteok.com/jobs"
		}

		// Use the pointers for salary
		var sMin, sMax *int
		if rj.SalaryMin > 0 {
			min := rj.SalaryMin
			sMin = &min
		}
		if rj.SalaryMax > 0 {
			max := rj.SalaryMax
			sMax = &max
		}

		job := domain.Job{
			ID:             sources.StableJobID("remoteok", rj.ID, rj.Position, rj.Company),
			ExternalID:     rj.ID,
			Title:          rj.Position,
			Company:        rj.Company,
			CompanySlug:    Slugify(rj.Company),
			Location:       NormalizeLocation(rj.Location),
			Description:    StripHTML(rj.Description),
			SalaryMin:      sMin,
			SalaryMax:      sMax,
			SalaryCurrency: "USD",
			PostedAt:       postedAt,
			Source:         "remoteok",
			SourceURL:      extURL,
			Skills:         rj.Tags,
			IsRemote:       true,
			EmploymentType: "Full-time", // Most are full time or contract, 
		}

		jobs = append(jobs, job)
	}

	// Simple pagination
	limit := 50
	start := (page - 1) * limit
	if start >= len(jobs) {
		return nil, nil
	}
	end := start + limit
	if end > len(jobs) {
		end = len(jobs)
	}

	return jobs[start:end], nil
}

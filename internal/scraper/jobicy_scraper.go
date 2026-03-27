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

type JobicyScraper struct {
	client *http.Client
	urls   []string
}

func NewJobicyScraper() *JobicyScraper {
	return &JobicyScraper{
		client: &http.Client{Timeout: 25 * time.Second},
		urls: []string{
			"https://jobicy.com/api/v2/remote-jobs?count=50&tag=engineer",
		},
	}
}

func (s *JobicyScraper) Name() string {
	return "jobicy"
}

type jobicyResponse struct {
	Success bool        `json:"success"`
	Jobs    []jobicyJob `json:"jobs"`
}

type jobicyJob struct {
	ID             int      `json:"id"`
	JobTitle       string   `json:"jobTitle"`
	CompanyName    string   `json:"companyName"`
	JobGeo         string   `json:"jobGeo"`
	JobType        []string `json:"jobType"`
	JobSalary      string   `json:"jobSalary"`
	JobExcerpt     string   `json:"jobExcerpt"`
	URL            string   `json:"url"`
	PubDate        string   `json:"pubDate"`
	JobIndustry    []string `json:"jobIndustry"`
	SalaryMin      int      `json:"salaryMin"`
	SalaryMax      int      `json:"salaryMax"`
	SalaryCurrency string   `json:"salaryCurrency"`
}

func (s *JobicyScraper) Search(ctx context.Context, query, location string, page int) ([]domain.Job, error) {
	var allJobs []domain.Job

	for _, apiURL := range s.urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
		if err != nil {
			continue
		}

		req.Header.Set("User-Agent", "JobPulse/1.0 (+https://github.com/samuelshine/jobpulse; job aggregator)")

		resp, err := s.client.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			continue
		}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		continue
	}

	rawBody := string(body)
	if len(rawBody) > 500 {
		rawBody = rawBody[:500] + "..."
	}
	log.Printf("[Jobicy] Raw Response (truncated) from %s: %s", apiURL, rawBody)

	var res jobicyResponse
	if err := json.Unmarshal(body, &res); err != nil {
		log.Printf("[Jobicy] Failed to unmarshal: %v", err)
		continue
	}

	log.Printf("[Jobicy] Successfully unmarshaled %d jobs", len(res.Jobs))

		for _, j := range res.Jobs {
			// Filtering
			if query != "" {
				if !strings.Contains(strings.ToLower(j.JobTitle), strings.ToLower(query)) &&
					!strings.Contains(strings.ToLower(j.CompanyName), strings.ToLower(query)) &&
					!strings.Contains(strings.ToLower(j.JobExcerpt), strings.ToLower(query)) {
					continue
				}
			}

			postedAt, _ := time.Parse(time.RFC3339, j.PubDate)
			if postedAt.IsZero() {
				postedAt, _ = time.Parse(time.RFC1123Z, j.PubDate)
			}
			if postedAt.IsZero() {
				postedAt, _ = time.Parse("2006-01-02 15:04:05", j.PubDate)
			}

			var minPtr, maxPtr *int
			if j.SalaryMin > 0 {
				m := j.SalaryMin
				minPtr = &m
			}
			if j.SalaryMax > 0 {
				m := j.SalaryMax
				maxPtr = &m
			}

			jobType := "Full-time"
			if len(j.JobType) > 0 {
				jobType = j.JobType[0]
			}

			job := domain.Job{
                ID:             sources.StableJobID("jobicy", fmt.Sprintf("%d", j.ID), j.JobTitle, j.CompanyName),
				ExternalID:     fmt.Sprintf("%d", j.ID),
				Title:          j.JobTitle,
				Company:        j.CompanyName,
				CompanySlug:    Slugify(j.CompanyName),
				Location:       NormalizeLocation(j.JobGeo),
				Description:    StripHTML(j.JobExcerpt),
				SalaryMin:      minPtr,
				SalaryMax:      maxPtr,
				SalaryCurrency: j.SalaryCurrency,
				PostedAt:       postedAt,
				Source:         "jobicy",
				SourceURL:      j.URL,
				Skills:         ExtractSkills(j.JobTitle + " " + j.JobExcerpt),
				IsRemote:       true,
				EmploymentType: jobType,
			}

			if !strings.HasPrefix(job.SourceURL, "https://jobicy.com") || !IsValidJobURL(job.SourceURL) {
				job.SourceURL = "https://jobicy.com/jobs"
			}
			allJobs = append(allJobs, job)
		}

		// Throttle
		time.Sleep(1 * time.Second)
	}

	// Deduplicate
	seen := make(map[string]bool)
	var uniqueJobs []domain.Job
	for _, j := range allJobs {
		if !seen[j.ID] {
			seen[j.ID] = true
			uniqueJobs = append(uniqueJobs, j)
		}
	}

	// Pagination
	limit := 50
	start := (page - 1) * limit
	if start >= len(uniqueJobs) {
		return nil, nil
	}
	end := start + limit
	if end > len(uniqueJobs) {
		end = len(uniqueJobs)
	}

	return uniqueJobs[start:end], nil
}

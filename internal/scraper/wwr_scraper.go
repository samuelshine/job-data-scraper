package scraper

import (
	"context"
	"crypto/tls"
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/samuelshine/job-data-scraper/internal/domain"
	"github.com/samuelshine/job-data-scraper/internal/sources"
)

type WWRScraper struct {
	client *http.Client
	feeds  []string
}

func NewWWRScraper() *WWRScraper {
	return &WWRScraper{
		client: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		feeds: []string{
			"https://weworkremotely.com/remote-jobs.rss",
			"https://weworkremotely.com/categories/remote-programming-jobs.rss",
			"https://weworkremotely.com/categories/remote-devops-sysadmin-jobs.rss",
		},
	}
}

func (s *WWRScraper) Name() string {
	return "weworkremotely"
}

type rssRoot struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	Region      string `xml:"region"`
}

func (s *WWRScraper) Search(ctx context.Context, query, location string, page int) ([]domain.Job, error) {
	var allJobs []domain.Job

	for _, feedURL := range s.feeds {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
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

		var rss rssRoot
		if err := xml.Unmarshal(body, &rss); err != nil {
			continue
		}

		for _, item := range rss.Channel.Items {
			// Title format is usually "Company Name: Job Title"
			parts := strings.SplitN(item.Title, ": ", 2)
			company := "Unknown"
			title := item.Title
			if len(parts) == 2 {
				company = parts[0]
				title = parts[1]
			}

			// Filtering
			if query != "" {
				if !strings.Contains(strings.ToLower(title), strings.ToLower(query)) &&
					!strings.Contains(strings.ToLower(company), strings.ToLower(query)) &&
					!strings.Contains(strings.ToLower(item.Description), strings.ToLower(query)) {
					continue
				}
			}

			postedAt, _ := time.Parse(time.RFC1123Z, item.PubDate)
			if postedAt.IsZero() {
				postedAt, _ = time.Parse(time.RFC1123, item.PubDate)
			}

			cleanDesc := StripHTML(item.Description)
			skills := ExtractSkills(item.Title + " " + cleanDesc)

			job := domain.Job{
                ID:             sources.StableJobID("weworkremotely", "", title, company, item.Link),
				Title:          title,
				Company:        company,
				CompanySlug:    Slugify(company),
				Location:       NormalizeLocation(item.Region),
				Description:    cleanDesc,
				PostedAt:       postedAt,
				Source:         "weworkremotely",
				SourceURL:      item.Link,
				Skills:         skills,
				IsRemote:       true,
				EmploymentType: "Full-time",
			}

			if !strings.Contains(job.SourceURL, "weworkremotely.com") || !IsValidJobURL(job.SourceURL) {
				job.SourceURL = "https://weworkremotely.com"
			}
			allJobs = append(allJobs, job)
		}

		// Throttle between feeds
		time.Sleep(500 * time.Millisecond)
	}

	// Deduplicate across feeds
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

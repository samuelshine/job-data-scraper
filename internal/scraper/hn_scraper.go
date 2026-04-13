package scraper

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"
	"github.com/samuelshine/job-data-scraper/internal/domain"
	"github.com/samuelshine/job-data-scraper/internal/sources"
)

type HNScraper struct {
	threadID string
}

func NewHNScraper() *HNScraper {
	id := resolveLatestHNThread()
	return &HNScraper{threadID: id}
}

// resolveLatestHNThread queries the HN Algolia API for the latest
// "Ask HN: Who is hiring?" thread and returns its object ID.
func resolveLatestHNThread() string {
	const fallback = "47219668"
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Get("https://hn.algolia.com/api/v1/search_by_date?query=%22Ask+HN:+Who+is+hiring%22&tags=story,ask_hn&hitsPerPage=1")
	if err != nil {
		log.Printf("⚠️  HN thread lookup failed, using fallback: %v", err)
		return fallback
	}
	defer resp.Body.Close()

	var result struct {
		Hits []struct {
			ObjectID string `json:"objectID"`
			Title    string `json:"title"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.Hits) == 0 {
		log.Printf("⚠️  HN thread lookup returned no results, using fallback")
		return fallback
	}

	log.Printf("✅ Resolved HN thread: %s (%s)", result.Hits[0].ObjectID, result.Hits[0].Title)
	return result.Hits[0].ObjectID
}

func (s *HNScraper) Name() string {
	return "hn_hiring"
}

func (s *HNScraper) Search(ctx context.Context, query, location string, page int) ([]domain.Job, error) {
	var jobs []domain.Job

	c := colly.NewCollector(
		colly.AllowedDomains("news.ycombinator.com"),
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"),
		colly.Async(true),
	)

	// Configure TLS skip verify for Docker Alpine compatibility
	c.WithTransport(&http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	})

	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: 1,
		Delay:       3 * time.Second,
	})

	c.OnHTML("tr.athing", func(e *colly.HTMLElement) {
		commHTML, _ := e.DOM.Find(".commtext").Html()
		if commHTML == "" {
			return
		}

		cleanText := StripHTML(commHTML)

		lines := strings.Split(cleanText, "\n")
		if len(lines) == 0 {
			return
		}
		firstLine := strings.TrimSpace(lines[0])

		// Logic for splitting first line
		var parts []string
		if strings.Contains(firstLine, "|") {
			parts = strings.Split(firstLine, "|")
		} else if strings.Contains(firstLine, " - ") {
			parts = strings.Split(firstLine, " - ")
		} else if strings.Contains(firstLine, " – ") {
			parts = strings.Split(firstLine, " – ")
		}

		company := "Unknown Company"
		title := firstLine
		loc := "Remote"
		isRemote := false

		if len(parts) >= 1 {
			company = strings.TrimSpace(parts[0])
			if len(company) > 60 {
				company = company[:60]
			}
		}
		if len(parts) >= 2 {
			title = strings.TrimSpace(parts[1])
			if len(title) > 100 {
				title = title[:100]
			}
		}
		if len(parts) >= 3 {
			loc = strings.TrimSpace(parts[2])
		}

		// Validation for company name
		invalidPrefixes := []string{"Join", "We", "The", "A ", "An ", "Our", "This", "Looking", "Hiring", "Remote", "Full", "Part", "Senior", "Junior"}
		isInvalid := false
		for _, prefix := range invalidPrefixes {
			if strings.HasPrefix(strings.ToLower(company), strings.ToLower(prefix)) {
				isInvalid = true
				break
			}
		}
		if isInvalid || company == "" {
			company = "Unknown Company"
		}

		// Swap if title looks like company and company is unknown
		if company == "Unknown Company" && !strings.Contains(title, " ") && title == strings.ToUpper(title) {
			company = title
			title = "Software Engineer"
		}

		lowerText := strings.ToLower(cleanText)
		if strings.Contains(lowerText, "remote") {
			isRemote = true
		}

		// The canonical apply URL for an HN job is always the comment itself
		extURL := fmt.Sprintf("https://news.ycombinator.com/item?id=%s", e.Attr("id"))

		// Filtering
		if query != "" {
			if !strings.Contains(strings.ToLower(title), strings.ToLower(query)) &&
				!strings.Contains(strings.ToLower(company), strings.ToLower(query)) &&
				!strings.Contains(strings.ToLower(cleanText), strings.ToLower(query)) {
				return
			}
		}

		skills := ExtractSkills(cleanText)

		job := domain.Job{
			ID:             sources.StableJobID("hn_hiring", e.Attr("id"), title, company),
			ExternalID:     e.Attr("id"),
			Title:          title,
			Company:        company,
			CompanySlug:    Slugify(company),
			Location:       NormalizeLocation(loc),
			Description:    cleanText,
			PostedAt:       time.Now(), // Thread is usually current month
			Source:         "hn_hiring",
			SourceURL:      extURL,
			Skills:         skills,
			IsRemote:       isRemote,
			EmploymentType: "Full-time",
		}
		jobs = append(jobs, job)
	})

	err := c.Visit(fmt.Sprintf("https://news.ycombinator.com/item?id=%s", s.threadID))
	if err != nil {
		return nil, err
	}
	c.Wait()

	// HN "Who's Hiring" is one huge page, so we just return the full set or a slice
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

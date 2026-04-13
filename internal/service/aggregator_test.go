package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/samuelshine/job-data-scraper/internal/domain"
)

func TestDedup_MergesRicherDuplicate(t *testing.T) {
	intPtr := func(v int) *int { return &v }

	jobs := []domain.Job{
		{
			ID:             "job-1",
			Title:          "Senior Go Engineer",
			Company:        "Acme",
			CompanySlug:    "acme",
			Location:       "Remote",
			EmploymentType: "full-time",
			PostedAt:       time.Date(2026, 3, 27, 10, 0, 0, 0, time.UTC),
			Skills:         domain.StringSlice{"Go"},
		},
		{
			ID:             "job-2",
			Title:          "Senior Go Engineer",
			Company:        "Acme",
			CompanySlug:    "acme-inc",
			Location:       "Remote",
			EmploymentType: "full-time",
			PostedAt:       time.Date(2026, 3, 27, 11, 0, 0, 0, time.UTC),
			SourceURL:      "https://jobs.example.com/roles/go-engineer?utm_source=test",
			Description:    "Build distributed services in Go.",
			SalaryMin:      intPtr(140000),
			SalaryMax:      intPtr(180000),
			Skills:         domain.StringSlice{"Distributed Systems", "Go"},
			IsRemote:       true,
		},
	}

	deduped := dedup(jobs)
	if len(deduped) != 1 {
		t.Fatalf("len(deduped) = %d, want 1", len(deduped))
	}

	job := deduped[0]
	if job.SourceURL == "" {
		t.Fatal("SourceURL not merged from richer duplicate")
	}
	if job.Description == "" {
		t.Fatal("Description not merged from richer duplicate")
	}
	if job.SalaryMin == nil || *job.SalaryMin != 140000 {
		t.Fatalf("SalaryMin = %v, want 140000", job.SalaryMin)
	}
	if len(job.Skills) != 2 {
		t.Fatalf("len(Skills) = %d, want 2", len(job.Skills))
	}
	if !job.IsRemote {
		t.Fatal("IsRemote = false, want true")
	}
}

func TestWorkerPool_RunBoundsConcurrency(t *testing.T) {
	pool := NewWorkerPool[int, int](2)
	tasks := []int{1, 2, 3, 4, 5, 6}

	var current int32
	var maxSeen int32

	results := pool.Run(context.Background(), tasks, func(_ context.Context, task int) int {
		active := atomic.AddInt32(&current, 1)
		for {
			prev := atomic.LoadInt32(&maxSeen)
			if active <= prev || atomic.CompareAndSwapInt32(&maxSeen, prev, active) {
				break
			}
		}

		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&current, -1)
		return task * 2
	})

	if got := len(results); got != len(tasks) {
		t.Fatalf("len(results) = %d, want %d", got, len(tasks))
	}
	if maxSeen > 2 {
		t.Fatalf("max concurrent workers = %d, want <= 2", maxSeen)
	}
	for index, result := range results {
		want := tasks[index] * 2
		if result != want {
			t.Fatalf("results[%d] = %d, want %d", index, result, want)
		}
	}
}

package service

import (
	"context"
	"sync"
)

// WorkerPool runs tasks with bounded concurrency.
type WorkerPool[T any, R any] struct {
	workers int
}

// NewWorkerPool creates a pool that executes up to workers tasks at a time.
func NewWorkerPool[T any, R any](workers int) *WorkerPool[T, R] {
	if workers <= 0 {
		workers = 1
	}

	return &WorkerPool[T, R]{workers: workers}
}

// Run processes tasks and returns one result per submitted task.
func (p *WorkerPool[T, R]) Run(ctx context.Context, tasks []T, fn func(context.Context, T) R) []R {
	if len(tasks) == 0 {
		return nil
	}

	type taskWithIndex struct {
		index int
		task  T
	}
	type resultWithIndex struct {
		index  int
		result R
	}

	taskCh := make(chan taskWithIndex)
	resultCh := make(chan resultWithIndex, len(tasks))

	var wg sync.WaitGroup
	for range p.workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range taskCh {
				resultCh <- resultWithIndex{
					index:  task.index,
					result: fn(ctx, task.task),
				}
			}
		}()
	}

	go func() {
		defer close(taskCh)
		for index, task := range tasks {
			select {
			case <-ctx.Done():
				return
			case taskCh <- taskWithIndex{index: index, task: task}:
			}
		}
	}()

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	results := make([]R, len(tasks))
	for item := range resultCh {
		results[item.index] = item.result
	}

	return results
}

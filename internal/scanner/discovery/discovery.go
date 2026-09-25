package discovery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Walk discovers Git marker parents under root. Work is bounded by the worker
// count and job buffer. Callbacks may run concurrently.
func Walk(ctx context.Context, root string, maxDepth int, excluded func(string, string) bool, found func(string) error) error {
	if maxDepth < 0 {
		return fmt.Errorf("negative max depth: %d", maxDepth)
	}
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("root %s is not a directory", root)
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	type item struct {
		path  string
		depth int
	}
	jobs := make(chan item, 64)
	var tasks, workers sync.WaitGroup
	var errorMu sync.Mutex
	var firstErr error
	var partial []error
	partialCount := 0
	recordPartial := func(err error) {
		errorMu.Lock()
		partialCount++
		if len(partial) < 5 {
			partial = append(partial, err)
		}
		errorMu.Unlock()
	}
	fail := func(err error) {
		errorMu.Lock()
		if firstErr == nil {
			firstErr = err
			cancel()
		}
		errorMu.Unlock()
	}
	var process func(item)
	process = func(current item) {
		defer tasks.Done()
		if ctx.Err() != nil {
			return
		}
		dir, err := os.Open(current.path)
		if err != nil {
			if current.depth > 0 && os.IsNotExist(err) {
				return
			}
			wrapped := fmt.Errorf("read %s: %w", current.path, err)
			if current.depth == 0 {
				fail(wrapped)
			} else {
				recordPartial(wrapped)
			}
			return
		}
		defer dir.Close()
		for {
			entries, readErr := dir.ReadDir(128)
			for _, entry := range entries {
				if ctx.Err() != nil {
					return
				}
				if entry.Name() == ".git" {
					if err := found(current.path); err != nil {
						fail(err)
						return
					}
					continue
				}
				if current.depth >= maxDepth || !entry.IsDir() {
					continue
				}
				child := filepath.Join(current.path, entry.Name())
				if excluded != nil && excluded(entry.Name(), child) {
					continue
				}
				tasks.Add(1)
				next := item{child, current.depth + 1}
				select {
				case jobs <- next:
				default:
					// Keep making progress when the bounded queue is full.
					process(next)
				}
			}
			if readErr == io.EOF {
				return
			}
			if readErr != nil {
				recordPartial(fmt.Errorf("read %s: %w", current.path, readErr))
				return
			}
		}
	}
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for current := range jobs {
				process(current)
			}
		}()
	}
	tasks.Add(1)
	jobs <- item{root, 0}
	tasks.Wait()
	close(jobs)
	workers.Wait()
	errorMu.Lock()
	err = firstErr
	count := partialCount
	examples := append([]error(nil), partial...)
	errorMu.Unlock()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if count > 0 {
		return &PartialError{Count: count, Examples: examples}
	}
	return nil
}

// PartialError reports unreadable subdirectories without discarding results
// found in the remainder of the root.
type PartialError struct {
	Count    int
	Examples []error
}

func (e *PartialError) Error() string {
	if len(e.Examples) == 0 {
		return fmt.Sprintf("%d directories could not be read", e.Count)
	}
	return fmt.Sprintf("%d directories could not be read: %v", e.Count, errors.Join(e.Examples...))
}

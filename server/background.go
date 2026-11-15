// SPDX-License-Identifier: AGPL-3.0-or-later

package server

import (
	"sync"

	"github.com/rs/zerolog/log"
)

const backgroundQueueSize = 1000

// backgroundQueue runs keyed jobs on a pool of workers. A key that is
// already queued or running is not queued again.
type backgroundQueue struct {
	name    string
	once    sync.Once
	jobs    chan backgroundJob
	mu      sync.Mutex
	workers int
	pending map[string]struct{}
}

type backgroundJob struct {
	key string
	run func()
}

func newBackgroundQueue(name string) *backgroundQueue {
	return &backgroundQueue{name: name}
}

// enqueue schedules run unless key is pending. The pool grows to workers.
func (q *backgroundQueue) enqueue(key string, workers int, run func()) {
	q.once.Do(func() {
		q.jobs = make(chan backgroundJob, backgroundQueueSize)
		q.pending = make(map[string]struct{})
	})
	q.mu.Lock()
	defer q.mu.Unlock()
	if _, queued := q.pending[key]; queued {
		return
	}
	for q.workers < max(1, workers) {
		q.workers++
		go q.work()
	}
	select {
	case q.jobs <- backgroundJob{key: key, run: run}:
		q.pending[key] = struct{}{}
	default:
		log.Warn().Str("queue", q.name).Str("key", key).Msg("background queue is full; skipping job")
	}
}

func (q *backgroundQueue) work() {
	for job := range q.jobs {
		job.run()
		q.mu.Lock()
		delete(q.pending, job.key)
		q.mu.Unlock()
	}
}

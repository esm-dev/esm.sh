package server

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

// BuildQueue runs builds in FIFO order, sharing results for the same requested path.
type BuildQueue struct {
	lock        sync.Mutex
	tasks       map[string]*buildTask
	pending     list.List
	resumes     []chan struct{} // released builds waiting to take a slot back
	running     int
	concurrency int
	timeout     time.Duration
}

type buildTask struct {
	ctx         *BuildContext
	path        string // the queue key stays fixed when installation changes ctx.Path()
	done        chan struct{}
	meta        *BuildMeta
	err         error
	waitClients int
	createdAt   time.Time
	startedAt   time.Time
	elem        *list.Element
}

// buildQueueKey is the context key of the queue running a build.
type buildQueueKey struct{}

func NewBuildQueue(concurrency int, timeout time.Duration) *BuildQueue {
	if concurrency <= 0 {
		concurrency = 1
	}
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	return &BuildQueue{
		tasks:       map[string]*buildTask{},
		concurrency: concurrency,
		timeout:     timeout,
	}
}

// Build waits for a shared build. Canceling the wait leaves an active build running.
func (q *BuildQueue) Build(ctx context.Context, build *BuildContext) (*BuildMeta, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if message := build.npmrc.getCachedPackageNotFound(build.esmPath.Package()); message != "" {
		return nil, errors.New(message)
	}
	path := build.Path()
	if message := negativeCache.get("404-path:" + build.esmPath.PackageId() + "/build:" + path); message != "" {
		return nil, errors.New(message)
	}
	q.lock.Lock()
	task := q.tasks[path]
	if task == nil {
		task = &buildTask{
			ctx:       build,
			path:      path,
			done:      make(chan struct{}),
			createdAt: time.Now(),
		}
		build.status.Store("pending")
		q.tasks[path] = task
		task.elem = q.pending.PushBack(task)
	}
	task.waitClients++
	q.scheduleLocked()
	q.lock.Unlock()

	defer func() {
		q.lock.Lock()
		task.waitClients--
		if task.waitClients == 0 && task.startedAt.IsZero() {
			delete(q.tasks, task.path)
			q.pending.Remove(task.elem)
		}
		q.lock.Unlock()
	}()

	select {
	case <-task.done:
		if build != task.ctx {
			build.path = task.ctx.path
			build.esmPath = task.ctx.esmPath
		}
		return task.meta, task.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (q *BuildQueue) Snapshot() []map[string]any {
	q.lock.Lock()
	defer q.lock.Unlock()

	tasks := make([]*buildTask, 0, len(q.tasks))
	for _, task := range q.tasks {
		tasks = append(tasks, task)
	}
	slices.SortFunc(tasks, func(a, b *buildTask) int {
		return a.createdAt.Compare(b.createdAt)
	})
	items := make([]map[string]any, 0, len(tasks))
	for _, task := range tasks {
		items = append(items, map[string]any{
			"waitClients": task.waitClients,
			"createdAt":   task.createdAt.Format(time.RFC1123),
			"path":        task.path,
			"status":      task.ctx.status.Load(),
		})
	}
	return items
}

// release frees the slot of a running build that waits for another build.
func (q *BuildQueue) release() {
	q.lock.Lock()
	q.running--
	q.scheduleLocked()
	q.lock.Unlock()
}

// acquire takes a slot back for a released build, before the pending tasks.
func (q *BuildQueue) acquire() {
	resume := make(chan struct{})
	q.lock.Lock()
	q.resumes = append(q.resumes, resume)
	q.scheduleLocked()
	q.lock.Unlock()
	<-resume
}

// scheduleLocked resumes released builds, then starts pending tasks while capacity is available.
func (q *BuildQueue) scheduleLocked() {
	for q.running < q.concurrency && len(q.resumes) > 0 {
		close(q.resumes[0])
		q.resumes = slices.Delete(q.resumes, 0, 1)
		q.running++
	}
	for q.running < q.concurrency && q.pending.Len() > 0 {
		task := q.pending.Remove(q.pending.Front()).(*buildTask)
		task.startedAt = time.Now()
		task.ctx.status.Store("build")
		q.running++
		go q.run(task)
	}
}

func (q *BuildQueue) run(task *buildTask) {
	defer func() {
		if r := recover(); r != nil {
			task.err = fmt.Errorf("build panic: %v", r)
		}
		if logger := task.ctx.logger; logger != nil {
			if task.err != nil {
				logger.Errorf("build '%s': %v", task.path, task.err)
			} else {
				logger.Infof("build '%s'(%s) done in %v", task.ctx.Path(), task.ctx.target, time.Since(task.startedAt))
			}
		}

		q.lock.Lock()
		delete(q.tasks, task.path)
		q.running--
		close(task.done)
		q.scheduleLocked()
		q.lock.Unlock()
	}()

	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), buildQueueKey{}, q), q.timeout)
	defer cancel()
	task.meta, task.err = task.ctx.Build(ctx)
	purgeOnNoSpace(task.err)
	if errors.Is(task.err, context.DeadlineExceeded) {
		task.err = fmt.Errorf("build timeout after %d seconds", q.timeout/time.Second)
	}
}

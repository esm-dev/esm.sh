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
	resumes     []*buildTask // released builds waiting to take a slot back
	running     int
	concurrency int
	timeout     time.Duration
}

type buildTask struct {
	queue       *BuildQueue
	ctx         *BuildContext
	path        string // the queue key stays fixed when installation changes ctx.Path()
	done        chan struct{}
	resume      chan struct{}
	meta        *BuildMeta
	err         error
	waitClients int
	slot        bool // holds a build slot
	createdAt   time.Time
	startedAt   time.Time
	elem        *list.Element
}

// buildTaskKey is the context key of the queue task running a build.
type buildTaskKey struct{}

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
			queue:     q,
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
		// [warning] a timed out build may still run, its context is only safe to read after a success
		if task.err == nil && build != task.ctx {
			build.path = task.ctx.path
			build.esmPath = task.ctx.esmPath
		}
		return task.meta, task.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Has reports whether a build of the path is pending or running.
func (q *BuildQueue) Has(path string) bool {
	q.lock.Lock()
	defer q.lock.Unlock()
	return q.tasks[path] != nil
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
func (task *buildTask) release() {
	q := task.queue
	q.lock.Lock()
	if task.slot {
		task.slot = false
		q.running--
		q.scheduleLocked()
	}
	q.lock.Unlock()
}

// acquire takes a slot back for a released build, before the pending tasks.
func (task *buildTask) acquire() {
	q := task.queue
	q.lock.Lock()
	select {
	case <-task.done:
		// a finished (timed out) task never takes a slot back, its build only exits
		q.lock.Unlock()
		return
	default:
	}
	// [warning] parallel resolver callbacks of one build may wait at the same time, queue the task only once
	if task.slot {
		q.lock.Unlock()
		return
	}
	resume := task.resume
	if resume == nil {
		resume = make(chan struct{})
		task.resume = resume
		q.resumes = append(q.resumes, task)
		q.scheduleLocked()
	}
	q.lock.Unlock()
	select {
	case <-resume:
	case <-task.done:
	}
}

// scheduleLocked resumes released builds, then starts pending tasks while capacity is available.
func (q *BuildQueue) scheduleLocked() {
	for q.running < q.concurrency && len(q.resumes) > 0 {
		task := q.resumes[0]
		q.resumes = slices.Delete(q.resumes, 0, 1)
		task.slot = true
		q.running++
		close(task.resume)
		task.resume = nil
	}
	for q.running < q.concurrency && q.pending.Len() > 0 {
		task := q.pending.Remove(q.pending.Front()).(*buildTask)
		task.startedAt = time.Now()
		task.slot = true
		task.ctx.status.Store("build")
		q.running++
		go q.run(task)
	}
}

func (q *BuildQueue) run(task *buildTask) {
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), buildTaskKey{}, task), q.timeout)
	defer cancel()
	timeoutErr := fmt.Errorf("build timeout after %d seconds", q.timeout/time.Second)
	// [warning] a build may block in a call that ignores ctx, the timeout must not wait for it to exit
	key := "404-path:" + task.ctx.esmPath.PackageId() + "/build:" + task.path
	stop := context.AfterFunc(ctx, func() {
		negativeCache.put(key, timeoutErr.Error(), buildTimeoutTTL)
		q.finish(task, nil, timeoutErr)
	})
	var meta *BuildMeta
	var err error
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("build panic: %v", r)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			err = timeoutErr
		}
		if stop() {
			q.finish(task, meta, err)
		}
	}()
	meta, err = task.ctx.Build(ctx)
}

// finish removes a task from the queue, frees its slot and wakes its waiters.
func (q *BuildQueue) finish(task *buildTask, meta *BuildMeta, err error) {
	if logger := task.ctx.logger; logger != nil {
		if err != nil {
			logger.Errorf("build '%s': %v", task.path, err)
		} else {
			logger.Infof("build '%s'(%s) done in %v", task.ctx.Path(), task.ctx.target, time.Since(task.startedAt))
		}
	}
	q.lock.Lock()
	delete(q.tasks, task.path)
	if task.slot {
		task.slot = false
		q.running--
	}
	q.resumes = slices.DeleteFunc(q.resumes, func(t *buildTask) bool { return t == task })
	task.meta, task.err = meta, err
	close(task.done)
	q.scheduleLocked()
	q.lock.Unlock()
}

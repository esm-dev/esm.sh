package server

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// BuildQueue schedules build tasks of esm.sh
type BuildQueue struct {
	lock      sync.Mutex
	tasks     map[string]*BuildTask
	canonical map[string]*BuildTask
	queue     map[*BuildTask]struct{}
	pending   *list.List
	chann     int
	scheduler bool
}

type BuildTask struct {
	ctx       *BuildContext
	key       string
	canonical string
	waitChans []chan BuildOutput
	createdAt time.Time
	startedAt time.Time
}

type BuildOutput struct {
	meta *BuildMeta
	ctx  *BuildContext
	err  error
}

func NewBuildQueue(concurrency int) *BuildQueue {
	if concurrency <= 0 {
		concurrency = 1 // ensure at least 1 concurrent task
	}
	return &BuildQueue{
		queue:     map[*BuildTask]struct{}{},
		pending:   list.New(),
		tasks:     map[string]*BuildTask{},
		canonical: map[string]*BuildTask{},
		chann:     concurrency,
	}
}

// Add adds a new build task to the queue.
func (q *BuildQueue) Add(ctx *BuildContext) chan BuildOutput {
	q.lock.Lock()
	defer q.lock.Unlock()

	ch := make(chan BuildOutput, 1)

	key := ctx.Path()
	task, ok := q.canonical[key]
	if !ok {
		task, ok = q.tasks[key]
	}
	if ok {
		task.waitChans = append(task.waitChans, ch)
		return ch
	}

	task = &BuildTask{
		ctx:       ctx,
		key:       key,
		createdAt: time.Now(),
		waitChans: []chan BuildOutput{ch},
	}
	ctx.status = "pending"

	q.pending.PushBack(task)
	q.tasks[key] = task

	q.startSchedulerLocked()

	return ch
}

func (q *BuildQueue) Snapshot() []map[string]any {
	q.lock.Lock()
	defer q.lock.Unlock()

	items := make([]map[string]any, 0, len(q.queue)+q.pending.Len())
	for task := range q.queue {
		items = append(items, map[string]any{
			"waitClients": len(task.waitChans),
			"createdAt":   task.createdAt.Format(time.RFC1123),
			"path":        task.ctx.Path(),
			"status":      task.ctx.status,
		})
	}
	for el := q.pending.Front(); el != nil; el = el.Next() {
		task, ok := el.Value.(*BuildTask)
		if !ok {
			continue
		}
		items = append(items, map[string]any{
			"waitClients": len(task.waitChans),
			"createdAt":   task.createdAt.Format(time.RFC1123),
			"path":        task.ctx.Path(),
			"status":      task.ctx.status,
		})
	}
	return items
}

func (q *BuildQueue) startSchedulerLocked() {
	if q.scheduler || q.chann == 0 || q.pending.Len() == 0 {
		return
	}
	q.scheduler = true
	go q.schedule()
}

func (q *BuildQueue) schedule() {
	for {
		q.lock.Lock()
		if q.chann == 0 || q.pending.Len() == 0 {
			q.scheduler = false
			q.lock.Unlock()
			return
		}

		task, _ := q.pending.Remove(q.pending.Front()).(*BuildTask)
		q.queue[task] = struct{}{}
		task.startedAt = time.Now()
		q.chann--
		q.lock.Unlock()

		go q.run(task)
	}
}

func (q *BuildQueue) run(task *BuildTask) {
	var output BuildOutput

	defer func() {
		if r := recover(); r != nil {
			output.err = fmt.Errorf("build panic: %v", r)
			task.ctx.status = "error"
			task.ctx.logger.Errorf("build '%s' panicked: %v", task.ctx.Path(), r)
		}

		q.lock.Lock()
		waitChans := task.waitChans
		q.chann++
		delete(q.queue, task)
		q.deleteTaskLocked(task.key, task)
		if task.canonical != "" && q.canonical[task.canonical] == task {
			delete(q.canonical, task.canonical)
		}
		q.startSchedulerLocked()
		q.lock.Unlock()

		for _, ch := range waitChans {
			if ch == nil {
				continue
			}
			select {
			case ch <- output:
			default:
				// the request may have already timed out while waiting on the build
			}
		}
	}()

	buildTimeout := 10 * time.Minute
	if config != nil && config.BuildTimeout > 0 {
		buildTimeout = time.Duration(config.BuildTimeout) * time.Second
	}
	buildCtx, cancel := context.WithTimeout(context.Background(), buildTimeout)
	defer cancel()

	task.ctx.onPathReady = func() bool {
		return q.rekey(task)
	}
	meta, err := task.ctx.Build(buildCtx)
	if errors.Is(err, context.DeadlineExceeded) {
		err = fmt.Errorf("build timeout after %d seconds", buildTimeout/time.Second)
	}
	if err == nil {
		task.ctx.status = "done"
		if task.ctx.target == "types" {
			task.ctx.logger.Infof("build '%s'(types) done in %v", task.ctx.Path(), time.Since(task.startedAt))
		} else {
			task.ctx.logger.Infof("build '%s' done in %v", task.ctx.Path(), time.Since(task.startedAt))
		}
	} else {
		task.ctx.status = "error"
		task.ctx.logger.Errorf("build '%s': %v", task.ctx.Path(), err)
	}
	output = BuildOutput{meta: meta, ctx: task.ctx, err: err}
}

func (q *BuildQueue) deleteTaskLocked(key string, task *BuildTask) {
	if key != "" && q.tasks[key] == task {
		delete(q.tasks, key)
	}
}

func (q *BuildQueue) rekey(task *BuildTask) bool {
	q.lock.Lock()
	defer q.lock.Unlock()

	key := task.ctx.Path()
	if current := q.canonical[key]; current != nil && current != task {
		current.waitChans = append(current.waitChans, task.waitChans...)
		task.waitChans = nil
		q.deleteTaskLocked(task.key, task)
		task.ctx.status = "joined"
		return false
	}
	q.canonical[key] = task
	task.canonical = key
	return true
}

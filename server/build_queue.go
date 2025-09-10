package server

import (
	"container/list"
	"sync"
	"sync/atomic"
	"time"
)

var taskPool = sync.Pool{
	New: func() interface{} {
		return &BuildTask{}
	},
}

// BuildQueue schedules build tasks of esm.sh
type BuildQueue struct {
	lock      sync.Mutex
	tasks     map[string]*BuildTask
	queue     *list.List
	chann     uint16
	scheduler int32 // scheduler state (0=not running, 1=running)
}

type BuildTask struct {
	ctx       *BuildContext
	el        *list.Element
	waitChans []chan BuildOutput
	createdAt time.Time
	startedAt time.Time
	pending   bool
}

type BuildOutput struct {
	meta *BuildMeta
	err  error
}

func NewBuildQueue(concurrency int) *BuildQueue {
	if concurrency <= 0 {
		concurrency = 1 // ensure at least 1 concurrent task
	}
	return &BuildQueue{
		queue: list.New(),
		tasks: map[string]*BuildTask{},
		chann: uint16(concurrency),
	}
}

// Add adds a new build task to the queue.
func (q *BuildQueue) Add(ctx *BuildContext) chan BuildOutput {
	q.lock.Lock()
	defer q.lock.Unlock()

	ch := make(chan BuildOutput, 1)

	task, ok := q.tasks[ctx.Path()]
	if ok {
		task.waitChans = append(task.waitChans, ch)
		return ch
	}

	task = taskPool.Get().(*BuildTask)
	task.ctx = ctx
	task.createdAt = time.Now()
	task.waitChans = []chan BuildOutput{ch}
	task.pending = true
	ctx.status = "pending"

	task.el = q.queue.PushBack(task)
	q.tasks[ctx.Path()] = task

	// start scheduler if not already running
	if atomic.CompareAndSwapInt32(&q.scheduler, 0, 1) {
		go q.schedule()
	}

	return ch
}

func (q *BuildQueue) schedule() {
	defer func() {
		// ensure scheduler state is reset even if panic occurs
		if r := recover(); r != nil {
			// TODO: log panic
		}
		atomic.StoreInt32(&q.scheduler, 0)
	}()

	for {
		q.lock.Lock()

		var task *BuildTask
		if q.chann > 0 {
			for el := q.queue.Front(); el != nil; el = el.Next() {
				t, ok := el.Value.(*BuildTask)
				if ok && t.pending {
					task = t
					break
				}
			}
		}

		if task != nil {
			q.chann -= 1
			task.pending = false
			task.startedAt = time.Now()
			q.lock.Unlock()

			go q.run(task)
		} else {
			// no more tasks to schedule, exit the loop
			q.lock.Unlock()
			break
		}
	}
}

func (q *BuildQueue) run(task *BuildTask) {
	defer func() {
		// ensure channel counter is always incremented, even if panic occurs
		if r := recover(); r != nil {
			// TODO: log panic
		}

		q.lock.Lock()
		q.chann += 1

		// check if there are more pending tasks and restart scheduler if needed
		// do this check atomically while holding the lock
		hasPending := false
		for el := q.queue.Front(); el != nil; el = el.Next() {
			if t, ok := el.Value.(*BuildTask); ok && t.pending {
				hasPending = true
				break
			}
		}

		// only restart scheduler if we're not already running one
		if hasPending && atomic.CompareAndSwapInt32(&q.scheduler, 0, 1) {
			// start scheduler in a separate goroutine to avoid deadlock
			go func() {
				q.schedule()
			}()
		}

		q.lock.Unlock()
	}()

	meta, err := task.ctx.Build()
	if err != nil {
		// another shot if failed to resolve build entry
		time.Sleep(100 * time.Millisecond)
		meta, err = task.ctx.Build()
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

	// clean up task while holding the lock to prevent race conditions
	q.lock.Lock()
	q.queue.Remove(task.el)
	delete(q.tasks, task.ctx.Path())
	if task.ctx.rawPath != "" {
		// the `Build` function may have changed the path
		delete(q.tasks, task.ctx.rawPath)
	}
	q.lock.Unlock()

	// store wait channels before recycling the task
	waitChans := task.waitChans

	// recycle the task object
	task.ctx = nil
	task.el = nil
	task.waitChans = nil
	task.createdAt = time.Time{}
	task.startedAt = time.Time{}
	task.pending = false
	taskPool.Put(task)

	// send the build output
	output := BuildOutput{meta, err}
	for _, ch := range waitChans {
		if ch != nil {
			select {
			case ch <- output:
				// successfully sent
			default:
				// channel is full or closed, log the drop
				// Note: task.ctx might be nil here due to recycling, so we can't log the path
			}
		}
	}
}

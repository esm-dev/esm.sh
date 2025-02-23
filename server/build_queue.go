package server

import (
	"container/list"
	"sync"
	"time"
)

var taskPool = sync.Pool{
	New: func() interface{} {
		return &BuildTask{}
	},
}

// BuildQueue schedules build tasks of esm.sh
type BuildQueue struct {
	lock  sync.Mutex
	tasks map[string]*BuildTask
	queue *list.List
	chann uint16
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

	go q.schedule()

	return ch
}

func (q *BuildQueue) schedule() {
	q.lock.Lock()
	defer q.lock.Unlock()

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
		go q.run(task)
	}
}

func (q *BuildQueue) run(task *BuildTask) {
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

	q.lock.Lock()
	q.queue.Remove(task.el)
	delete(q.tasks, task.ctx.Path())
	if task.ctx.rawPath != "" {
		// the `Build` function may have changed the path
		delete(q.tasks, task.ctx.rawPath)
	}
	q.chann += 1
	q.lock.Unlock()

	waitChans := task.waitChans

	// recycle the task object
	task.ctx = nil
	task.el = nil
	task.waitChans = nil
	task.createdAt = time.Time{}
	task.startedAt = time.Time{}
	task.pending = false
	taskPool.Put(task)

	// schedule next task if have any
	go q.schedule()

	// send the bulid output
	output := BuildOutput{meta, err}
	for _, ch := range waitChans {
		select {
		case ch <- output:
		default:
			// drop
		}
	}
}

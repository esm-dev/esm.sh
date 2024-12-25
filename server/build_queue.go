package server

import (
	"container/list"
	"sync"
	"time"
)

// BuildQueue schedules build tasks of esm.sh
type BuildQueue struct {
	lock  sync.RWMutex
	tasks map[string]*BuildTask
	queue *list.List
	chann uint16
}

type BuildTask struct {
	*BuildContext
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

	task = &BuildTask{
		BuildContext: ctx,
		createdAt:    time.Now(),
		waitChans:    []chan BuildOutput{ch},
		pending:      true,
	}
	ctx.status = "pending"

	task.el = q.queue.PushBack(task)
	q.tasks[ctx.Path()] = task

	go q.schedule()

	return ch
}

func (q *BuildQueue) schedule() {
	var task *BuildTask

	q.lock.RLock()
	if q.chann > 0 {
		for el := q.queue.Front(); el != nil; el = el.Next() {
			t, ok := el.Value.(*BuildTask)
			if ok && t.pending {
				task = t
				break
			}
		}
	}
	q.lock.RUnlock()

	if task != nil {
		q.lock.Lock()
		q.chann -= 1
		task.pending = false
		task.startedAt = time.Now()
		q.lock.Unlock()
		q.run(task)
	}
}

func (q *BuildQueue) run(task *BuildTask) {
	meta, err := task.Build()
	if err == nil {
		task.status = "done"
		if task.target == "types" {
			log.Infof("build '%s'(types) done in %v", task.Path(), time.Since(task.startedAt))
		} else {
			log.Infof("build '%s' done in %v", task.Path(), time.Since(task.startedAt))
		}
	} else {
		task.status = "error"
		log.Errorf("build '%s': %v", task.Path(), err)
	}

	q.lock.Lock()
	q.queue.Remove(task.el)
	delete(q.tasks, task.Path())
	q.chann += 1
	q.lock.Unlock()

	output := BuildOutput{meta, err}
	for _, ch := range task.waitChans {
		select {
		case ch <- output:
		default:
			// drop
		}
	}

	// schedule next task if have any
	go q.schedule()
}

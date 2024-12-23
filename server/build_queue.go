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
	lock  sync.RWMutex
	tasks map[string]*BuildTask
	queue *list.List
	idles uint16
}

type BuildTask struct {
	*BuildContext
	el        *list.Element
	waitChans []chan *BuildOutput
	createdAt time.Time
	startedAt time.Time
	pending   bool
}

type BuildOutput struct {
	result *BuildMeta
	err    error
}

func NewBuildQueue(concurrency int) *BuildQueue {
	return &BuildQueue{
		queue: list.New(),
		tasks: map[string]*BuildTask{},
		idles: uint16(concurrency),
	}
}

// Add adds a new build task to the queue.
func (q *BuildQueue) Add(ctx *BuildContext) chan *BuildOutput {
	q.lock.Lock()
	defer q.lock.Unlock()

	ch := make(chan *BuildOutput, 1)

	// check if the task is already in the queue
	task, ok := q.tasks[ctx.Path()]
	if ok {
		task.waitChans = append(task.waitChans, ch)
		return ch
	}

	ctx.status = "pending"

	task = taskPool.Get().(*BuildTask)
	task.BuildContext = ctx
	task.el = q.queue.PushBack(task)
	task.waitChans = []chan *BuildOutput{ch}
	task.createdAt = time.Now()
	task.startedAt = time.Time{}
	task.pending = true

	q.tasks[ctx.Path()] = task

	q.lock.Unlock()
	q.schedule()
	q.lock.Lock()

	return ch
}

func (q *BuildQueue) schedule() {
	var task *BuildTask

	q.lock.RLock()
	if q.idles > 0 {
		for el := q.queue.Front(); el != nil; el = el.Next() {
			t, ok := el.Value.(*BuildTask)
			if ok && t.pending {
				task = t
				break
			}
		}
	}
	q.lock.RUnlock()

	// no available task
	if task == nil {
		return
	}

	q.lock.Lock()
	q.idles -= 1
	task.pending = false
	task.startedAt = time.Now()
	q.lock.Unlock()

	go q.run(task)
}

func (q *BuildQueue) run(task *BuildTask) {
	// reuse the task object
	defer taskPool.Put(task)

	ret, err := task.Build()
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

	output := &BuildOutput{ret, err}
	q.lock.RLock()
	for _, ch := range task.waitChans {
		ch <- output
	}
	q.lock.RUnlock()

	q.lock.Lock()
	q.queue.Remove(task.el)
	delete(q.tasks, task.Path())
	q.idles += 1
	q.lock.Unlock()

	// schedule next task if have any
	q.schedule()
}

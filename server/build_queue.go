package server

import (
	"container/list"
	"sync"
	"time"
)

// BuildQueue schedules build tasks of esm.sh
type BuildQueue struct {
	lock  sync.RWMutex
	queue *list.List
	tasks map[string]*BuildTask
	idles int32
}

type BuildTask struct {
	*BuildContext
	el        *list.Element
	clients   []*QueueClient
	createdAt time.Time
	startedAt time.Time
	inProcess bool
}

type BuildOutput struct {
	result *BuildMeta
	err    error
}

type QueueClient struct {
	C  chan BuildOutput
	IP string
}

func NewBuildQueue(concurrency int) *BuildQueue {
	q := &BuildQueue{
		queue: list.New(),
		tasks: map[string]*BuildTask{},
		idles: int32(concurrency),
	}
	return q
}

// Add adds a new build task to the queue.
func (q *BuildQueue) Add(ctx *BuildContext, clientIp string) *QueueClient {
	client := &QueueClient{make(chan BuildOutput, 1), clientIp}

	// check if the task is already in the queue
	q.lock.RLock()
	t, ok := q.tasks[ctx.Path()]
	q.lock.RUnlock()

	if ok {
		t.clients = append(t.clients, client)
		return client
	}

	t = &BuildTask{
		BuildContext: ctx,
		createdAt:    time.Now(),
		clients:      []*QueueClient{client},
	}
	ctx.stage = "pending"

	q.lock.Lock()
	t.el = q.queue.PushBack(t)
	q.tasks[ctx.Path()] = t
	q.lock.Unlock()

	q.next()

	return client
}

func (q *BuildQueue) next() {
	var nextTask *BuildTask

	q.lock.RLock()
	if q.idles > 0 {
		for el := q.queue.Front(); el != nil; el = el.Next() {
			t, ok := el.Value.(*BuildTask)
			if ok && !t.inProcess {
				nextTask = t
				break
			}
		}
	}
	q.lock.RUnlock()

	if nextTask != nil {
		q.lock.Lock()
		q.idles -= 1
		nextTask.inProcess = true
		nextTask.startedAt = time.Now()
		q.lock.Unlock()
		go q.build(nextTask)
	}
}

func (q *BuildQueue) build(t *BuildTask) {
	ret, err := t.Build()
	if err == nil {
		if t.target == "types" {
			log.Infof("build '%s'(types) done in %v", t.Path(), time.Since(t.startedAt))
		} else if t.subBuilds != nil && t.subBuilds.Len() > 0 {
			log.Infof("build '%s'(%d sub-builds) done in %v", t.Path(), t.subBuilds.Len(), time.Since(t.startedAt))
		} else {
			log.Infof("build '%s' done in %v", t.Path(), time.Since(t.startedAt))
		}
	} else {
		log.Errorf("build '%s': %v", t.Path(), err)
	}

	output := BuildOutput{ret, err}
	for _, c := range t.clients {
		c.C <- output
	}

	q.lock.Lock()
	q.idles += 1
	q.queue.Remove(t.el)
	delete(q.tasks, t.Path())
	q.lock.Unlock()

	// call next task
	q.next()
}

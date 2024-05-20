package server

import (
	"container/list"
	"sync"
	"time"
)

// BuildQueue schedules build tasks of esm.sh
type BuildQueue struct {
	lock        sync.RWMutex
	queue       *list.List
	tasks       map[string]*QueueTask
	wip         int32
	concurrency int32
}

type QueueTask struct {
	*BuildContext
	el        *list.Element
	clients   []*QueueClient
	createdAt time.Time
	startedAt time.Time
	inProcess bool
}

type QueueClient struct {
	C  chan BuildOutput `json:"-"`
	IP string           `json:"ip"`
}

type BuildOutput struct {
	result BuildResult
	err    error
}

func NewBuildQueue(concurrency int) *BuildQueue {
	q := &BuildQueue{
		queue:       list.New(),
		tasks:       map[string]*QueueTask{},
		concurrency: int32(concurrency),
	}
	return q
}

func (t *QueueTask) build() BuildOutput {
	t.inProcess = true
	t.startedAt = time.Now()
	ret, err := t.Build()
	if err == nil {
		if t.subBuilds != nil && t.subBuilds.Len() > 0 {
			log.Infof("build '%s' (%d sub-builds) done in %v", t.Path(), t.subBuilds.Len(), time.Since(t.startedAt))
		} else {
			log.Infof("build '%s' done in %v", t.Path(), time.Since(t.startedAt))
		}
	} else {
		log.Errorf("build '%s': %v", t.Path(), err)
	}
	return BuildOutput{ret, err}
}

// Len returns the number of tasks of the queue.
func (q *BuildQueue) Len() int {
	q.lock.RLock()
	defer q.lock.RUnlock()

	return q.queue.Len()
}

// Add adds a new build task to the queue.
func (q *BuildQueue) Add(ctx *BuildContext, clientIp string) *QueueClient {
	client := &QueueClient{make(chan BuildOutput, 1), clientIp}

	// check if the task is already in the queue
	q.lock.RLock()
	t, ok := q.tasks[ctx.Path()]
	if ok && clientIp != "" {
		t.clients = append(t.clients, client)
	}
	q.lock.RUnlock()

	if ok {
		return client
	}

	ctx.stage = "pending"
	t = &QueueTask{
		BuildContext: ctx,
		createdAt:    time.Now(),
		clients:      []*QueueClient{},
	}
	if clientIp != "" {
		t.clients = []*QueueClient{client}
	}

	q.lock.Lock()
	t.el = q.queue.PushBack(t)
	q.tasks[ctx.Path()] = t
	q.lock.Unlock()

	q.next()

	return client
}

func (q *BuildQueue) next() {
	var nextTask *QueueTask

	q.lock.RLock()
	if q.wip < q.concurrency {
		for el := q.queue.Front(); el != nil; el = el.Next() {
			t, ok := el.Value.(*QueueTask)
			if ok && !t.inProcess {
				nextTask = t
				break
			}
		}
	}
	q.lock.RUnlock()

	if nextTask == nil {
		return
	}

	q.lock.Lock()
	q.wip += 1
	q.lock.Unlock()

	go q.run(nextTask)
}

func (q *BuildQueue) run(t *QueueTask) {
	output := t.build()
	for _, c := range t.clients {
		c.C <- output
	}

	q.lock.Lock()
	q.wip -= 1
	q.queue.Remove(t.el)
	delete(q.tasks, t.Path())
	q.lock.Unlock()

	// call next task
	q.next()
}

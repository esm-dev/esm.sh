package server

import (
	"container/list"
	"sync"
	"time"
)

// BuildQueue schedules build tasks of esm.sh
type BuildQueue struct {
	lock        sync.RWMutex
	concurrency int
	current     *list.List
	queue       *list.List
	tasks       map[string]*BuildTask
}

type BuildTask struct {
	ctx       *BuildContext
	clients   []*QueueClient
	createdAt time.Time
	startedAt time.Time
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
		concurrency: concurrency,
		current:     list.New(),
		queue:       list.New(),
		tasks:       map[string]*BuildTask{},
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
		ctx:       ctx,
		createdAt: time.Now(),
		clients:   []*QueueClient{client},
	}

	q.lock.Lock()
	q.tasks[ctx.Path()] = t
	q.queue.PushBack(t)
	q.lock.Unlock()

	if q.current.Len() < q.concurrency {
		q.next()
	}

	return client
}

func (q *BuildQueue) next() {
	q.lock.RLock()
	n := q.queue.Front()
	q.lock.RUnlock()

	if n != nil {
		task := n.Value.(*BuildTask)
		q.lock.Lock()
		q.queue.Remove(n)
		el := q.current.PushBack(task)
		q.lock.Unlock()
		go q.build(task, el)
	}
}

func (q *BuildQueue) build(t *BuildTask, el *list.Element) {
	t.startedAt = time.Now()
	ret, err := t.ctx.Build()
	if err == nil {
		if t.ctx.target == "types" {
			log.Infof("build '%s'(types) done in %v", t.ctx.Path(), time.Since(t.startedAt))
		} else {
			log.Infof("build '%s' done in %v", t.ctx.Path(), time.Since(t.startedAt))
		}
	} else {
		log.Errorf("build '%s': %v", t.ctx.Path(), err)
	}

	output := BuildOutput{ret, err}
	for _, c := range t.clients {
		c.C <- output
	}

	q.lock.Lock()
	q.current.Remove(el)
	delete(q.tasks, t.ctx.Path())
	q.lock.Unlock()

	// call next task
	q.next()
}

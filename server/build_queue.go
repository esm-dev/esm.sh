package server

import (
	"container/list"
	"sync"
	"time"
)

// BuildQueue schedules build tasks of esm.sh
type BuildQueue struct {
	lock        sync.Mutex
	concurrency int
	current     *list.List
	queue       *list.List
	tasks       map[string]*BuildTask
}

type BuildTask struct {
	*BuildContext
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
	q.lock.Lock()
	defer q.lock.Unlock()

	client := &QueueClient{make(chan BuildOutput, 1), clientIp}

	// check if the task is already in the queue
	t, ok := q.tasks[ctx.Path()]
	if ok {
		t.clients = append(t.clients, client)
		return client
	}

	t = &BuildTask{
		BuildContext: ctx,
		createdAt:    time.Now(),
		clients:      []*QueueClient{client},
	}
	q.tasks[ctx.Path()] = t
	q.queue.PushBack(t)

	q.lock.Unlock()
	q.next()
	q.lock.Lock()

	return client
}

func (q *BuildQueue) next() {
	if q.current.Len() < q.concurrency {
		n := q.queue.Front()
		if n != nil {
			q.queue.Remove(n)
			task := n.Value.(*BuildTask)
			go q.build(task, q.current.PushBack(task))
		}
	}
}

func (q *BuildQueue) build(t *BuildTask, el *list.Element) {
	t.startedAt = time.Now()
	ret, err := t.Build()
	if err == nil {
		if t.target == "types" {
			log.Infof("build '%s'(types) done in %v", t.Path(), time.Since(t.startedAt))
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
	q.current.Remove(el)
	delete(q.tasks, t.Path())
	q.lock.Unlock()

	// call next task
	q.next()
}

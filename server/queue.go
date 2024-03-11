package server

import (
	"container/list"
	"sync"
	"time"
)

// BuildQueue schedules build tasks of esm.sh
type BuildQueue struct {
	lock        sync.RWMutex
	list        *list.List
	tasks       map[string]*queueTask
	processes   []*queueTask
	concurrency int
}

type BuildQueueClient struct {
	IP string           `json:"ip"`
	C  chan BuildOutput `json:"-"`
}

type BuildOutput struct {
	meta *ESMBuild
	err  error
}

type queueTask struct {
	*BuildTask
	el        *list.Element
	createdAt time.Time
	startedAt time.Time
	clients   []*BuildQueueClient
	inProcess bool
}

func (t *queueTask) run() BuildOutput {
	meta, err := t.Build()
	if err == nil {
		if t.subBuilds != nil && t.subBuilds.Len() > 0 {
			log.Infof("build '%s' (%d sub-builds) done in %v", t.ID(), t.subBuilds.Len(), time.Since(t.startedAt))
		} else {
			log.Infof("build '%s' done in %v", t.ID(), time.Since(t.startedAt))
		}
	} else {
		log.Errorf("build '%s': %v", t.ID(), err)
	}
	return BuildOutput{meta, err}
}

func newBuildQueue(concurrency int) *BuildQueue {
	q := &BuildQueue{
		list:        list.New(),
		tasks:       map[string]*queueTask{},
		concurrency: concurrency,
	}
	return q
}

// Len returns the number of tasks of the queue.
func (q *BuildQueue) Len() int {
	q.lock.RLock()
	defer q.lock.RUnlock()

	return q.list.Len()
}

// Add adds a new build task.
func (q *BuildQueue) Add(task *BuildTask, clientIp string) *BuildQueueClient {
	client := &BuildQueueClient{clientIp, make(chan BuildOutput, 1)}
	q.lock.Lock()
	t, ok := q.tasks[task.ID()]
	if ok && clientIp != "" {
		t.clients = append(t.clients, client)
	}
	q.lock.Unlock()

	if ok {
		return client
	}

	task.stage = "pending"
	t = &queueTask{
		BuildTask: task,
		createdAt: time.Now(),
		clients:   []*BuildQueueClient{},
	}
	if clientIp != "" {
		t.clients = []*BuildQueueClient{client}
	}
	q.lock.Lock()
	t.el = q.list.PushBack(t)
	q.tasks[task.ID()] = t
	q.lock.Unlock()

	q.next()

	return client
}

func (q *BuildQueue) RemoveClient(task *BuildTask, c *BuildQueueClient) {
	q.lock.Lock()
	defer q.lock.Unlock()

	t, ok := q.tasks[task.ID()]
	if ok {
		clients := make([]*BuildQueueClient, len(t.clients))
		i := 0
		for _, _c := range t.clients {
			if _c != c {
				clients[i] = c
				i++
			}
		}
		t.clients = clients[0:i]
	}
}

func (q *BuildQueue) next() {
	var nextTask *queueTask
	q.lock.Lock()
	if len(q.processes) < q.concurrency {
		for el := q.list.Front(); el != nil; el = el.Next() {
			t, ok := el.Value.(*queueTask)
			if ok && !t.inProcess {
				nextTask = t
				break
			}
		}
	}
	q.lock.Unlock()

	if nextTask == nil {
		return
	}

	q.lock.Lock()
	nextTask.inProcess = true
	q.processes = append(q.processes, nextTask)
	q.lock.Unlock()

	go q.wait(nextTask)
}

func (q *BuildQueue) wait(t *queueTask) {
	t.startedAt = time.Now()

	output := t.run()

	q.lock.Lock()
	a := make([]*queueTask, len(q.processes))
	i := 0
	for _, _t := range q.processes {
		if _t != t {
			a[i] = _t
			i++
		}
	}
	q.processes = a[0:i]
	q.list.Remove(t.el)
	delete(q.tasks, t.ID())
	q.lock.Unlock()

	// call next task
	q.next()

	for _, c := range t.clients {
		c.C <- output
	}
}

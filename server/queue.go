package server

import (
	"container/list"
	"fmt"
	"sync"
	"time"
)

// A Queue for esm build tasks
type BuildQueue struct {
	lock         sync.RWMutex
	list         *list.List
	tasks        map[string]*queueTask
	processes    []*queueTask
	maxProcesses int
}

type BuildQueueConsumer struct {
	IP string           `json:"ip"`
	C  chan BuildOutput `json:"-"`
}

type BuildOutput struct {
	meta *ESMBuild
	err  error
}

type queueTask struct {
	*BuildTask
	inProcess bool
	el        *list.Element
	createdAt time.Time
	startedAt time.Time
	consumers []*BuildQueueConsumer
}

func (t *queueTask) run() BuildOutput {
	c := make(chan BuildOutput, 1)
	go func(c chan BuildOutput) {
		meta, err := t.Build()
		c <- BuildOutput{meta, err}
	}(c)

	var output BuildOutput
	select {
	case output = <-c:
		if output.err == nil {
			log.Infof("build '%s' done in %v", t.ID(), time.Since(t.startedAt))
		} else {
			log.Errorf("build '%s': %v", t.ID(), output.err)
		}
	case <-time.After(10 * time.Minute):
		log.Errorf("build '%s': timeout(%v)", t.ID(), time.Since(t.startedAt))
		output = BuildOutput{
			err: fmt.Errorf("build '%s': timeout(%v)", t.ID(), time.Since(t.startedAt)),
		}
	}

	return output
}

func newBuildQueue(maxProcesses int) *BuildQueue {
	q := &BuildQueue{
		list:         list.New(),
		tasks:        map[string]*queueTask{},
		maxProcesses: maxProcesses,
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
func (q *BuildQueue) Add(task *BuildTask, consumerIp string) *BuildQueueConsumer {
	c := &BuildQueueConsumer{consumerIp, make(chan BuildOutput, 1)}
	q.lock.Lock()
	t, ok := q.tasks[task.ID()]
	if ok && consumerIp != "" {
		t.consumers = append(t.consumers, c)
	}
	q.lock.Unlock()

	if ok {
		return c
	}

	task.stage = "pending"
	t = &queueTask{
		BuildTask: task,
		createdAt: time.Now(),
		consumers: []*BuildQueueConsumer{},
	}
	if consumerIp != "" {
		t.consumers = []*BuildQueueConsumer{c}
	}
	q.lock.Lock()
	t.el = q.list.PushBack(t)
	q.tasks[task.ID()] = t
	q.lock.Unlock()

	q.next()

	return c
}

func (q *BuildQueue) RemoveConsumer(task *BuildTask, c *BuildQueueConsumer) {
	q.lock.Lock()
	defer q.lock.Unlock()

	t, ok := q.tasks[task.ID()]
	if ok {
		consumers := make([]*BuildQueueConsumer, len(t.consumers))
		i := 0
		for _, _c := range t.consumers {
			if _c != c {
				consumers[i] = c
				i++
			}
		}
		t.consumers = consumers[0:i]
	}
}

func (q *BuildQueue) next() {
	var nextTask *queueTask
	q.lock.Lock()
	if len(q.processes) < q.maxProcesses {
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

	for _, c := range t.consumers {
		c.C <- output
	}
}

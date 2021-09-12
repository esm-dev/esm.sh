package server

import (
	"container/list"
	"fmt"
	"sync"
	"time"
)

// A Queue for esbuild
type BuildQueue struct {
	lock         sync.Mutex
	list         *list.List
	current      []*task
	tasks        map[string]*task
	maxProcesses int
}

type BuildQueueConsumer struct {
	C chan BuildOutput
}

type BuildOutput struct {
	esm *ESM
	err error
}

type task struct {
	*buildTask
	inProcess  bool
	el         *list.Element
	createTime time.Time
	startTime  time.Time
	consumers  []*BuildQueueConsumer
}

func newBuildQueue(maxProcesses int) *BuildQueue {
	q := &BuildQueue{
		list:         list.New(),
		tasks:        map[string]*task{},
		maxProcesses: maxProcesses,
	}
	return q
}

// Len returns the number of tasks of the queue.
func (q *BuildQueue) Len() int {
	q.lock.Lock()
	defer q.lock.Unlock()

	return q.list.Len()
}

// Add adds a new build task.
func (q *BuildQueue) Add(build *buildTask) *BuildQueueConsumer {
	build.beforeBuild()

	q.lock.Lock()
	defer q.lock.Unlock()

	c := &BuildQueueConsumer{make(chan BuildOutput, 1)}
	t, ok := q.tasks[build.ID()]
	if ok {
		t.consumers = append(t.consumers, c)
		return c
	}

	t = &task{
		buildTask:  build,
		createTime: time.Now(),
		consumers:  []*BuildQueueConsumer{c},
	}
	t.el = q.list.PushBack(t)
	q.tasks[build.ID()] = t
	q.next()

	return c
}

func (q *BuildQueue) RemoveConsumer(build *buildTask, c *BuildQueueConsumer) {
	q.lock.Lock()
	defer q.lock.Unlock()

	t, ok := q.tasks[build.ID()]
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
	var nextTask *task
	if len(q.current) < q.maxProcesses {
		for el := q.list.Front(); el != nil; el = el.Next() {
			t, ok := el.Value.(*task)
			if ok && !t.inProcess {
				nextTask = t
				break
			}
		}
	}
	if nextTask == nil {
		return
	}

	nextTask.inProcess = true
	q.current = append(q.current, nextTask)

	q.lock.Unlock()
	go q.wait(nextTask)
	q.lock.Lock()
}

func (t *task) wait() chan BuildOutput {
	c := make(chan BuildOutput, 1)
	go func(c chan BuildOutput) {
		esm, err := t.Build()
		c <- BuildOutput{esm, err}
	}(c)
	return c
}

func (q *BuildQueue) wait(t *task) {
	t.startTime = time.Now()

	var output BuildOutput
	select {
	case output = <-t.wait():
		if output.err != nil {
			log.Errorf("buildESM: %v", output.err)
		}
	case <-time.After(15 * time.Minute):
		output = BuildOutput{err: fmt.Errorf("build timeout")}
		log.Errorf("buildESM(%s): timeout", t.ID())
	}

	q.lock.Lock()
	defer q.lock.Unlock()

	for _, c := range t.consumers {
		c.C <- output
	}

	var p []*task
	for _, _t := range q.current {
		if _t != t {
			p = append(p, _t)
		}
	}
	q.current = p
	q.list.Remove(t.el)
	delete(q.tasks, t.ID())

	log.Debugf(
		"BuildQueue(%s,%s) done in %s",
		t.pkg.String(),
		t.target,
		time.Now().Sub(t.startTime),
	)
	q.next()
}

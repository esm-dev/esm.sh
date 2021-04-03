package server

import (
	"container/list"
	"sync"
	"time"
)

// A Queue for esbuild
type buildQueue struct {
	lock         sync.RWMutex
	queue        *list.List
	current      []*task
	tasks        map[string]*task
	maxProcesses int
}

type buildOutput struct {
	esm        *ESMeta
	packageCSS bool
	err        error
}

type task struct {
	*buildTask
	inProcess  bool
	el         *list.Element
	createTime time.Time
	startTime  time.Time
	consumers  []chan *buildOutput
}

func newBuildQueue(maxProcesses int) *buildQueue {
	q := &buildQueue{
		queue:        list.New(),
		tasks:        map[string]*task{},
		maxProcesses: maxProcesses,
	}
	return q
}

// Len returns the number of tasks of queue.
func (q *buildQueue) Len() int {
	q.lock.RLock()
	defer q.lock.RUnlock()

	return q.queue.Len()
}

// Has checks a task is exist.
func (q *buildQueue) Has(id string) (ok bool) {
	q.lock.RLock()
	defer q.lock.RUnlock()

	_, ok = q.tasks[id]
	return
}

// Has checks a task is exist.
func (q *buildQueue) Add(build *buildTask) chan *buildOutput {
	q.lock.Lock()
	defer q.lock.Unlock()

	c := make(chan *buildOutput, 1)
	t, ok := q.tasks[build.ID()]
	if ok {
		t.consumers = append(t.consumers, c)
		return c
	}

	t = &task{
		buildTask:  build,
		createTime: time.Now(),
		consumers:  []chan *buildOutput{c},
	}
	t.el = q.queue.PushBack(t)
	q.tasks[build.ID()] = t

	q.lock.Unlock()
	q.next()
	q.lock.Lock()

	return c
}

func (q *buildQueue) next() {
	q.lock.Lock()
	defer q.lock.Unlock()

	var nextTask *task
	if len(q.current) < q.maxProcesses {
		for el := q.queue.Front(); el != nil; el = el.Next() {
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

func (q *buildQueue) wait(t *task) {
	t.startTime = time.Now()
	esm, packageCSS, err := t.buildESM()
	for _, c := range t.consumers {
		c <- &buildOutput{
			esm:        esm,
			packageCSS: packageCSS,
			err:        err,
		}
	}
	log.Debugf(
		"queue(%s,%s) done in %s",
		t.pkg.String(),
		t.target,
		time.Now().Sub(t.startTime),
	)

	var p []*task
	q.lock.Lock()
	for _, _t := range q.current {
		if _t != t {
			p = append(p, _t)
		}
	}
	q.current = p
	q.queue.Remove(t.el)
	delete(q.tasks, t.ID())
	q.lock.Unlock()

	q.next()
}

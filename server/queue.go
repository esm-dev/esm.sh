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
	C          chan *buildOutput
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
func (q *buildQueue) Add(build *buildTask) (t *task) {
	q.lock.Lock()
	defer q.lock.Unlock()

	t, ok := q.tasks[build.ID()]
	if ok {
		return
	}

	t = &task{
		buildTask:  build,
		createTime: time.Now(),
		C:          make(chan *buildOutput, 1),
	}
	t.el = q.queue.PushBack(t)
	q.tasks[build.ID()] = t

	q.lock.Unlock()
	q.next()
	q.lock.Lock()
	return
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
	t.C <- &buildOutput{
		esm:        esm,
		packageCSS: packageCSS,
		err:        err,
	}
	log.Debug("[queue]", t.ID(), "done in ", time.Now().Sub(t.startTime))

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

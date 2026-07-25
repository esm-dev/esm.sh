package server

import (
	"path/filepath"
	"testing"

	"github.com/esm-dev/esm.sh/internal/storage"
	"github.com/ije/gox/log"
)

func TestBuildQueueKeepsCanonicalTaskWhenRawTaskRekeys(t *testing.T) {
	raw := &BuildTask{ctx: &BuildContext{path: "/canonical", rawPath: "/raw"}}
	canonical := &BuildTask{ctx: &BuildContext{path: "/canonical"}}
	q := &BuildQueue{tasks: map[string]*BuildTask{
		"/raw":       raw,
		"/canonical": canonical,
	}}

	q.deleteTaskLocked(raw.ctx.Path(), raw)
	q.deleteTaskLocked(raw.ctx.rawPath, raw)

	if q.tasks["/canonical"] != canonical {
		t.Fatal("raw task cleanup removed the canonical task")
	}
	if _, ok := q.tasks["/raw"]; ok {
		t.Fatal("raw task was not removed")
	}
}

func TestBuildQueueJoinsCanonicalizedVariants(t *testing.T) {
	first := &BuildTask{
		ctx:       &BuildContext{path: "/canonical"},
		key:       "/raw-a",
		waitChans: []chan BuildOutput{make(chan BuildOutput, 1)},
	}
	second := &BuildTask{
		ctx:       &BuildContext{path: "/canonical"},
		key:       "/raw-b",
		waitChans: []chan BuildOutput{make(chan BuildOutput, 1)},
	}
	q := &BuildQueue{
		tasks: map[string]*BuildTask{
			first.key:  first,
			second.key: second,
		},
		canonical: map[string]*BuildTask{},
	}

	if !q.rekey(first) {
		t.Fatal("first canonical task was not registered")
	}
	if q.rekey(second) {
		t.Fatal("equivalent variant was not joined")
	}
	if len(first.waitChans) != 2 || len(second.waitChans) != 0 {
		t.Fatal("variant waiters were not transferred")
	}
	if q.canonical["/canonical"] != first {
		t.Fatal("canonical task ownership changed")
	}
	if _, ok := q.tasks[second.key]; ok {
		t.Fatal("joined raw task remained addressable")
	}
}

func TestBuildQueueReturnsExecutedContextToAllWaiters(t *testing.T) {
	fs, err := storage.NewFSStorage(filepath.Join(t.TempDir(), "storage"))
	if err != nil {
		t.Fatal(err)
	}
	logger, err := log.New("")
	if err != nil {
		t.Fatal(err)
	}
	metaDB := NewBuildMetaDB(fs)
	first := &BuildContext{
		logger:  logger,
		metaDB:  metaDB,
		esmPath: EsmPath{PkgName: "queue-fixture", PkgVersion: "1.0.0"},
		target:  "es2022",
	}
	second := &BuildContext{
		logger:  logger,
		metaDB:  metaDB,
		esmPath: first.esmPath,
		target:  first.target,
	}
	if err := metaDB.Put(first.Path(), encodeBuildMeta(&BuildMeta{})); err != nil {
		t.Fatal(err)
	}

	q := NewBuildQueue(1)
	q.chann = 0
	firstOutput := q.Add(first)
	secondOutput := q.Add(second)
	q.lock.Lock()
	q.chann = 1
	q.startSchedulerLocked()
	q.lock.Unlock()

	for _, ch := range []chan BuildOutput{firstOutput, secondOutput} {
		output := <-ch
		if output.err != nil {
			t.Fatal(output.err)
		}
		if output.ctx != first {
			t.Fatal("waiter did not receive the executed build context")
		}
	}
}

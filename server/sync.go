package server

import (
	"sync"
)

var (
	fetchLocks   sync.Map
	installLocks sync.Map
)

func getInstallLock(key string) *sync.Mutex {
	v, _ := installLocks.LoadOrStore(key, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func getFetchLock(key string) *sync.Mutex {
	v, _ := fetchLocks.LoadOrStore(key, &sync.Mutex{})
	return v.(*sync.Mutex)
}

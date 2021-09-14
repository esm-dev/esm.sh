package storage

import (
	"errors"
	"net/url"
	"runtime"
	"sync"
	"time"
)

type mItem struct {
	value     []byte
	expiresAt int64
}

func (v mItem) isExpired() bool {
	return v.expiresAt > 0 && time.Now().UnixNano() > v.expiresAt
}

type mCache struct {
	lock       sync.RWMutex
	storage    map[string]mItem
	gcInterval time.Duration
	gcTimer    *time.Timer
}

func (mc *mCache) Has(key string) bool {
	mc.lock.RLock()
	s, ok := mc.storage[key]
	mc.lock.RUnlock()

	if ok && s.isExpired() {
		go mc.Delete(key)
		ok = false
	}

	return ok
}

func (mc *mCache) Get(key string) (value []byte, err error) {
	mc.lock.RLock()
	s, ok := mc.storage[key]
	mc.lock.RUnlock()
	if !ok {
		err = ErrNotFound
		return
	}

	if s.isExpired() {
		mc.Delete(key)
		err = ErrExpired
		return
	}

	value = s.value
	return
}

func (mc *mCache) Set(key string, value []byte, ttl time.Duration) error {
	mc.lock.Lock()
	defer mc.lock.Unlock()

	if ttl > 0 {
		mc.storage[key] = mItem{value, time.Now().Add(ttl).UnixNano()}
	} else {
		mc.storage[key] = mItem{value, 0}
	}
	return nil
}

func (mc *mCache) Delete(key string) error {
	mc.lock.Lock()
	defer mc.lock.Unlock()

	delete(mc.storage, key)
	return nil
}

func (mc *mCache) Flush() error {
	mc.lock.Lock()
	defer mc.lock.Unlock()

	mc.storage = map[string]mItem{}
	return nil
}

func (mc *mCache) setGCInterval(interval time.Duration) {
	if interval > 0 && interval != mc.gcInterval {
		if mc.gcTimer != nil {
			mc.gcTimer.Stop()
		}
		mc.gcInterval = interval
		if interval > time.Second {
			mc.gcTimer = time.AfterFunc(interval, mc.gc)
		}
	}
}

func (mc *mCache) gc() {
	mc.gcTimer = time.AfterFunc(mc.gcInterval, mc.gc)

	mc.lock.RLock()
	defer mc.lock.RUnlock()
	for key, d := range mc.storage {
		if d.isExpired() {
			mc.lock.RUnlock()
			mc.lock.Lock()
			delete(mc.storage, key)
			mc.lock.Unlock()
			mc.lock.RLock()
		}
	}

	runtime.GC()
}

type mcDriver struct{}

func (mcd *mcDriver) Open(region string, options url.Values) (Cache, error) {
	gcInterval, err := parseDurationValue(options.Get("gcInterval"), 30*time.Minute)
	if err != nil {
		return nil, errors.New("invalid gcInterval value")
	}

	c := &mCache{
		storage:    map[string]mItem{},
		gcInterval: gcInterval,
	}
	c.setGCInterval(gcInterval)
	return c, nil
}

func init() {
	RegisterCache("memory", &mcDriver{})
}

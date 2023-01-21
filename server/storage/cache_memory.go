package storage

import (
	"errors"
	"net/url"
	"runtime"
	"sync"
	"time"
)

type mValue struct {
	data      []byte
	expiredAt int64
}

func (v mValue) isExpired() bool {
	return v.expiredAt > 0 && time.Now().UnixNano() > v.expiredAt
}

type mCache struct {
	lock       sync.RWMutex
	gcInterval time.Duration
	gcTimer    *time.Timer
	storage    map[string]mValue
}

func (mc *mCache) Has(key string) (bool, error) {
	mc.lock.RLock()
	s, ok := mc.storage[key]
	mc.lock.RUnlock()

	if ok && s.isExpired() {
		go mc.Delete(key)
		return false, nil
	}

	return ok, nil
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

	value = s.data
	return
}

func (mc *mCache) Set(key string, value []byte, ttl time.Duration) error {
	mc.lock.Lock()
	defer mc.lock.Unlock()

	if ttl > 0 {
		mc.storage[key] = mValue{value, time.Now().Add(ttl).UnixNano()}
	} else {
		mc.storage[key] = mValue{value, 0}
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

	mc.storage = map[string]mValue{}
	return nil
}

func (mc *mCache) setGC(interval time.Duration) {
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
		storage:    map[string]mValue{},
		gcInterval: gcInterval,
	}
	c.setGC(gcInterval)
	return c, nil
}

func init() {
	RegisterCache("memory", &mcDriver{})
}

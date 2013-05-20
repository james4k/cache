package cache

import (
	"sync"
	"time"
)

type syncher struct {
	sync.RWMutex
	waitmu  sync.Mutex // syncs rlock/lock transition and waitc
	refs    int
	lastref time.Time
	path    string
	waitc   chan struct{}
}

var (
	synm  = map[string]*syncher{}
	synmu sync.Mutex
)

// synchronizes all cache file access
func synch(path string) *syncher {
	synmu.Lock()
	s, ok := synm[path]
	if !ok {
		s = &syncher{
			path: path,
		}
		synm[path] = s
	}
	s.refs++
	synmu.Unlock()
	s.RLock()
	return s
}

func (s *syncher) write(f func()) bool {
	s.RUnlock()
	s.waitmu.Lock()
	if s.waitc != nil {
		waitc := s.waitc
		s.waitmu.Unlock()
		<-waitc
		s.RLock()
		return false
	} else {
		s.Lock()
		s.waitc = make(chan struct{})
		s.waitmu.Unlock()
	}
	f()
	close(s.waitc)
	s.waitmu.Lock()
	s.waitc = nil
	s.waitmu.Unlock()
	s.Unlock()
	s.RLock()
	return true
}

func (s *syncher) done() {
	synmu.Lock()
	s.refs--
	s.lastref = time.Now()
	synmu.Unlock()
	s.RUnlock()
}

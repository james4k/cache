package cache

import (
	"sync"
	"time"
)

type syncher struct {
	sync.RWMutex
	writemu sync.Mutex
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
	defer synmu.Unlock()
	s, ok := synm[path]
	if !ok {
		s = &syncher{
			path: path,
		}
		synm[path] = s
	}
	s.refs++
	s.RLock()
	return s
}

func (s *syncher) write(f func()) bool {
	s.writemu.Lock()
	defer s.writemu.Unlock()
	s.RUnlock()
	defer s.RLock()
	s.Lock()
	if s.waitc != nil {
		s.Unlock()
		<-s.waitc
		return false
	} else {
		s.waitc = make(chan struct{})
		s.Unlock()
	}
	f()
	s.Lock()
	close(s.waitc)
	s.waitc = nil
	s.Unlock()
	return true
}

func (s *syncher) done() {
	synmu.Lock()
	defer synmu.Unlock()
	s.refs--
	s.lastref = time.Now()
	s.RUnlock()
}

package cache

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type UseStaleMask int

// UseStale options. Serve old cache when...
const (
	Supdating = 1 << iota // cache is being updated; prevents stampeding while waiting on a request
	//Serror                // error occured during user handler's ServeHTTP
	//S404                  // status of 404 is received
	//S500                  // status of 500 is received
	//S50x                  // status of 500-505 is received
)

var Logger *log.Logger

type CacheHandler struct {
	sync.RWMutex
	dir      string
	handler  http.Handler
	key      KeyMask
	valid    map[int]time.Duration
	useStale UseStaleMask
	//gziplevel int
}

func Handler(cachedir string, handler http.Handler) *CacheHandler {
	absdir, err := filepath.Abs(cachedir)
	if err != nil {
		absdir = cachedir
	}
	c := &CacheHandler{
		dir:     absdir,
		handler: handler,
		valid:   make(map[int]time.Duration),
		key:     Kdefault,
	}
	return c.Valid(7*24*time.Hour, 200, 301, 302)
}

func HandlerFunc(cachedir string, handler func(http.ResponseWriter, *http.Request)) *CacheHandler {
	return Handler(cachedir, http.HandlerFunc(handler))
}

func (c *CacheHandler) Key(k KeyMask) *CacheHandler {
	c.key = k
	return c
}

func (c *CacheHandler) Valid(dur time.Duration, statusCodes ...int) *CacheHandler {
	if len(statusCodes) == 0 {
		c.valid[200] = dur
	} else {
		for _, s := range statusCodes {
			c.valid[s] = dur
		}
	}
	return c
}

func (c *CacheHandler) UseStale(u UseStaleMask) *CacheHandler {
	c.useStale = u
	return c
}

//// Enable gzip compression with level of 0-9. See constants in compress/flate.
//func (c *CacheHandler) Gzip(level int) *CacheHandler {
//	c.gziplevel = level
//	return c
//}

func (c *CacheHandler) log(e error) {
	if Logger != nil {
		Logger.Output(2, e.Error()+"\n")
	}
}

func (c *CacheHandler) age(code int) time.Duration {
	dur, ok := c.valid[code]
	if !ok {
		dur, ok = c.valid[200]
		if !ok {
			dur = 10 * time.Minute
		}
	}
	return dur
}

func (c *CacheHandler) isValid(code int, t time.Time) bool {
	return time.Now().Sub(t) <= c.age(code)
}

type serveState struct {
	*syncher
	key, path, tmpPath string
	crc                uint32
	recursions         int
	wrlock             sync.Locker
}

func (c *CacheHandler) logrecover() {
	if err, ok := recover().(error); ok {
		c.log(err)
	}
}

var errExpired = errors.New("cache: expired")
var errInvalidCache = errors.New("cache: invalid header")
var errKeyCollision = errors.New("cache: key collision")

func (c *CacheHandler) save(st *serveState, lock func(), w http.ResponseWriter, req *http.Request) {
	defer c.logrecover()
	p := &proxyWriter{
		c:    c,
		st:   st,
		rw:   w,
		lock: lock,
		createf: func() io.WriteCloser {
			f, err := os.OpenFile(st.tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0660)
			if err != nil {
				panic(err)
			}
			return f
		},
	}
	defer p.Close()
	c.handler.ServeHTTP(p, req)
}

func (c *CacheHandler) read(st *serveState, w http.ResponseWriter, req *http.Request) error {
	f, err := os.Open(st.path)
	if err != nil {
		if os.IsNotExist(err) {
			return errInvalidCache
		}
		return err
	}
	defer f.Close()

	br := bufio.NewReader(f)
	tpr := textproto.NewReader(br)
	lead, err := tpr.ReadLine()
	if err != nil {
		return err
	}

	var tag string
	var code int
	var crc uint32
	n, err := fmt.Sscan(lead, &tag, &code, &crc)
	if err != nil {
		return err
	}
	if n != 3 || tag != "CACHE" || code == 0 || crc == 0 {
		return errInvalidCache
	}
	if st.crc != crc {
		return errKeyCollision
	}
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if c.useStale&Supdating == 0 && !c.isValid(code, fi.ModTime()) {
		return errExpired
	}

	hdr, err := tpr.ReadMIMEHeader()
	if err != nil {
		return err
	}
	c.copyHeader(w.Header(), http.Header(hdr))

	w.WriteHeader(code)
	_, err = io.Copy(w, br)
	if err != nil {
		c.log(err)
	}
	return nil
}

func (c *CacheHandler) copyHeader(dst, src http.Header) {
	for k, v := range src {
		s := make([]string, len(v))
		copy(s, v)
		dst[k] = s
	}
}

func (c *CacheHandler) fallback(w http.ResponseWriter, req *http.Request) {
	c.handler.ServeHTTP(w, req)
}

func (c *CacheHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer c.logrecover()
	key, crc := makeKey(c.key, req)
	path := filepath.Join(c.dir, key)
	tmpPath := filepath.Join(c.dir, "updating"+key)
	st := &serveState{
		syncher: synch(path),
		path:    path,
		tmpPath: tmpPath,
		key:     key,
		crc:     crc,
	}
	defer st.done()
	c.serve(st, w, req)
}

func (c *CacheHandler) serve(st *serveState, w http.ResponseWriter, req *http.Request) {
	err := c.read(st, w, req)
	switch err {
	case nil:
	case errExpired, errInvalidCache:
		written := st.write(func(lock func()) {
			c.save(st, lock, w, req)
		})
		// if we didn't write, than another request did and we should try reading again
		if !written {
			st.recursions++
			if st.recursions > 1 {
				http.Error(w, "500 internal server error", 500)
				return
			}
			c.serve(st, w, req)
		}
	case errKeyCollision:
		c.log(fmt.Errorf("cache: key collision for %s (%s)", st.key, req.URL.Path))
		c.fallback(w, req)
	default:
		c.log(err)
		c.fallback(w, req)
	}
}

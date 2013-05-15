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

// TODO:
// - respect Cache-Control: max-age=X and Expires, default to 10min
// - ensure we log everywhere there can be an error, and fallback to original handler
// - all of the configurables

var Logger *log.Logger

type cacheHandler struct {
	sync.RWMutex
	cachesync sync.Mutex
	dir       string
	handler   http.Handler
	key       KeyMask
	ttl       map[int]time.Duration
	gziplevel int
}

func Handler(cachedir string, handler http.Handler) *cacheHandler {
	absdir, err := filepath.Abs(cachedir)
	if err != nil {
		absdir = cachedir
	}
	c := &cacheHandler{
		dir:     absdir,
		handler: handler,
		ttl: map[int]time.Duration{
			200: 7 * 24 * time.Hour,
			404: 1 * time.Minute,
		},
	}
	return c.Valid(7*24*time.Hour, 200, 301, 302).Headers("Content-Type")
}

func HandlerFunc(cachedir string, cacheHandler func(http.ResponseWriter, *http.Request)) *cacheHandler {
	return Handler(cachedir, http.HandlerFunc(cacheHandler))
}

func (c *cacheHandler) Headers(headers ...string) *cacheHandler {
	return c
}

func (c *cacheHandler) Valid(dur time.Duration, statusCodes ...int) *cacheHandler {
	//c.ttl[statusCode] = dur
	return c
}

// Enable gzip compression with level of 0-9. See constants in compress/flate.
func (c *cacheHandler) Gzip(level int) *cacheHandler {
	c.gziplevel = level
	return c
}

func (c *cacheHandler) UseStale() *cacheHandler {
	return c
}

func (c *cacheHandler) Methods() *cacheHandler {
	return c
}

func (c *cacheHandler) log(e error) {
	// TODO: may have to use Logger.Output directly so we get a good line number
	if Logger != nil {
		Logger.Output(2, e.Error()+"\n")
	} else {
		log.Println(e)
	}
}

func (c *cacheHandler) age(statusCode int) int {
	/*dur, ok := c.ttl[statusCode]
	if !ok {
		dur = c.ttl[200]
	}*/
	return 0
}

func (c *cacheHandler) isValid(t time.Time) bool {
	return true
}

type serveState struct {
	*syncher
	key, path  string
	crc        uint32
	recursions int
}

func (c *cacheHandler) logrecover() {
	if err, ok := recover().(error); ok {
		c.log(err)
	}
}

var errExpired = errors.New("cache: expired")
var errInvalidCache = errors.New("cache: invalid header")
var errKeyCollision = errors.New("cache: key collision")

func (c *cacheHandler) save(st *serveState, w http.ResponseWriter, req *http.Request) {
	defer c.logrecover()
	p := &proxyWriter{
		st: st,
		rw: w,
		createf: func() io.WriteCloser {
			f, err := os.OpenFile(st.path, os.O_WRONLY|os.O_CREATE, 0660)
			if err != nil {
				panic(err)
			}
			return f
		},
	}
	defer p.Close()
	c.handler.ServeHTTP(p, req)
}

func (c *cacheHandler) read(st *serveState, w http.ResponseWriter, req *http.Request) error {
	f, err := os.Open(st.path)
	if err != nil {
		if os.IsNotExist(err) {
			return errInvalidCache
		}
		return err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if !c.isValid(fi.ModTime()) {
		return errExpired
	}

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

func (c *cacheHandler) copyHeader(dst, src http.Header) {
	// TODO: anything we should filter out?
	for k, v := range src {
		s := make([]string, len(v))
		copy(s, v)
		dst[k] = s
	}
}

func (c *cacheHandler) fallback(w http.ResponseWriter, req *http.Request) {
	c.handler.ServeHTTP(w, req)
}

func (c *cacheHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	defer c.logrecover()
	key, crc := makeKey(Kdefault, req)
	path := filepath.Join(c.dir, key)
	st := &serveState{
		syncher: synch(path),
		path:    path,
		key:     key,
		crc:     crc,
	}
	defer st.done()
	c.serve(st, w, req)
}

func (c *cacheHandler) serve(st *serveState, w http.ResponseWriter, req *http.Request) {
	err := c.read(st, w, req)
	switch err {
	case nil:
	case errExpired, errInvalidCache:
		written := st.write(func() {
			c.save(st, w, req)
		})
		// if we didn't write, than another request did and we should read
		if !written {
			st.recursions++
			if st.recursions > 1 {
				return
			}
			c.serve(st, w, req)
		}
	case errKeyCollision:
		c.log(fmt.Errorf("cache: key collision for %s (%s)", st.key, req.URL.Path))
		c.fallback(w, req)
	default:
		c.log(err)
	}
}

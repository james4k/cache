package cache

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"
	"time"
)

// proxyWriter writes to the ResponseWriter and the cache
type proxyWriter struct {
	c          *cacheHandler
	st         *serveState
	rw         http.ResponseWriter
	statusCode int
	createf    func() io.WriteCloser
	f          io.WriteCloser
	bw         *bufio.Writer
}

func (p *proxyWriter) Header() http.Header {
	return p.rw.Header()
}

func (p *proxyWriter) WriteHeader(code int) {
	age, valid := p.cacheAge(p.rw.Header(), code)
	if valid {
		p.rw.Header().Set("Cache-Control", fmt.Sprintf("maxage=%d", age))
	}
	p.rw.WriteHeader(code)
	if p.statusCode == 0 {
		p.statusCode = code
		if valid {
			p.cacheHeader()
		}
	}
}

func (p *proxyWriter) Write(b []byte) (int, error) {
	if p.statusCode == 0 {
		age, valid := p.cacheAge(p.rw.Header(), 200)
		if valid {
			p.rw.Header().Set("Cache-Control", fmt.Sprintf("maxage=%d", age))
		} else {
			// don't cache the header below
			p.statusCode = 200
		}
	}
	c, err := p.rw.Write(b)
	if err != nil {
		return c, err
	}
	if p.statusCode == 0 {
		p.statusCode = 200
		p.cacheHeader()
	}
	if p.f != nil {
		return p.bw.Write(b)
	} else {
		return c, err
	}
}

func (p *proxyWriter) Close() {
	if p.f != nil {
		p.bw.Flush()
		p.f.Close()
	}
}

// Cache file format looks a bit like an HTTP response:
/*
CACHE <statusCode> <crc32>\r\n
Headers: Values\r\n
\r\n
<body>
*/
func (p *proxyWriter) cacheHeader() {
	hdr := p.rw.Header()
	p.f = p.createf()
	p.bw = bufio.NewWriter(p.f)
	tpw := textproto.NewWriter(p.bw)
	err := tpw.PrintfLine("CACHE %d %d", p.statusCode, p.st.crc)
	if err != nil {
		panic(err)
	}
	for k, v := range hdr {
		switch k {
		case "Cache-Control", "Expires":
			continue
		}
		for _, s := range v {
			err = tpw.PrintfLine("%s: %s", k, s)
			if err != nil {
				panic(err)
			}
		}
	}
	_, err = p.bw.WriteString("\r\n")
	if err != nil {
		panic(err)
	}
}

func (p *proxyWriter) cacheAge(hdr http.Header, statusCode int) (int64, bool) {
	if _, ok := hdr["Set-Cookie"]; ok {
		return 0, false
	}
	if v := hdr.Get("Expires"); v != "" {
		t, err := time.Parse(http.TimeFormat, v)
		if err != nil || time.Now().After(t) {
			return 0, false
		}
	}
	if vals, ok := hdr["Cache-Control"]; ok {
		for _, v := range vals {
			fields := strings.Fields(v)
			for _, f := range fields {
				if f == "no-store" ||
					strings.HasPrefix(f, "no-cache") ||
					strings.HasPrefix(f, "private") {
					return 0, false
				}
				if strings.HasPrefix(f, "max-age=") {
					age, err := strconv.ParseInt(f[len("max-age="):], 10, 64)
					if err != nil || age <= 0 {
						return 0, false
					}
					return age, true
				}
			}
		}
	}
	return int64(p.c.age(statusCode) / time.Second), true
}

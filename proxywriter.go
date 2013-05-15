package cache

import (
	"bufio"
	"io"
	"net/http"
	"net/textproto"
)

// proxyWriter writes to the ResponseWriter and the cache
type proxyWriter struct {
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
	p.rw.WriteHeader(code)
	if p.statusCode == 0 {
		p.statusCode = code
		p.cacheHeader()
	}
}

func (p *proxyWriter) Write(b []byte) (int, error) {
	c, err := p.rw.Write(b)
	if err != nil {
		return c, err
	}
	if p.statusCode == 0 {
		p.statusCode = 200
		p.cacheHeader()
	}
	return p.bw.Write(b)
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
	p.f = p.createf()
	p.bw = bufio.NewWriter(p.f)
	tpw := textproto.NewWriter(p.bw)
	err := tpw.PrintfLine("CACHE %d %d", p.statusCode, p.st.crc)
	if err != nil {
		panic(err)
	}
	hdr := p.rw.Header()
	for k, v := range hdr {
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

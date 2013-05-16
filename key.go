package cache

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"hash/crc32"
	"net/http"
)

type KeyMask int

const (
	// TODO: perhaps Kmethod should just always be implied
	Kmethod = 1 << iota
	Kscheme
	Khost
	Kpath
	Kquery
	Kcookie
	Kdefault = Kmethod | Kscheme | Kpath | Kquery
)

func makeKey(mask KeyMask, req *http.Request) (sum string, crc uint32) {
	h := md5.New()
	sumb := make([]byte, 0, h.Size())
	buf := bytes.NewBuffer(make([]byte, 0, 128))
	if mask&Kmethod != 0 {
		buf.WriteString(req.Method)
	}
	if mask&Kscheme != 0 {
		buf.WriteString(req.URL.Scheme)
	}
	if mask&Khost != 0 {
		buf.WriteString(req.URL.Host)
	}
	if mask&Kpath != 0 {
		buf.WriteString(req.URL.Path)
	}
	if mask&Kquery != 0 {
		buf.WriteString(req.URL.RawQuery)
	}
	if mask&Kcookie != 0 {
		if cookies, ok := req.Header["Cookie"]; ok {
			for _, co := range cookies {
				buf.WriteString(co)
			}
		}
	}
	h.Write(buf.Bytes())
	sum = hex.EncodeToString(h.Sum(sumb))
	crc = crc32.ChecksumIEEE(buf.Bytes())
	return sum, crc
}

package cache

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sync"
	"testing"
)

// TODO: would be nice if we didn't touch the disk

const keepTempDir = false

func init() {
	Logger = log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile)
	runtime.GOMAXPROCS(2)
}

func headersEqual(a, b http.Header) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		vb, ok := b[k]
		if !ok {
			return false
		}
		if len(v) != len(vb) {
			return false
		}
		if k == "Date" {
			continue
		}
		for i, s := range v {
			if s != vb[i] {
				return false
			}
		}
	}
	return true
}

// TODO: test that headers and body are equal
// TODO: break up this test into smaller tests
func TestFileServer(t *testing.T) {
	dir, err := ioutil.TempDir("", "cachetest")
	if err != nil {
		t.Fatal(err)
	}
	if !keepTempDir {
		defer os.RemoveAll(dir)
	} else {
		println(dir)
	}

	fs := http.FileServer(http.Dir("testdata"))
	srvActual := httptest.NewServer(Handler(dir, fs))
	srvExpected := httptest.NewServer(fs)
	defer srvActual.Close()
	defer srvExpected.Close()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Head(srvActual.URL + "/file.txt")
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			respExpected, err := http.Head(srvExpected.URL + "/file.txt")
			if err != nil {
				t.Fatal(err)
			}
			respExpected.Body.Close()
			if !headersEqual(resp.Header, respExpected.Header) {
				t.Log(resp.Request.Method, resp.StatusCode, resp.Header)
				t.Log(respExpected.Request.Method, respExpected.StatusCode, respExpected.Header)
				t.Fatal("headers not equal")
			}
			resp, err = http.Get(srvActual.URL + "/file.txt")
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			buf := bytes.NewBuffer(make([]byte, 0, 256))
			_, err = io.Copy(buf, resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			respExpected, err = http.Get(srvExpected.URL + "/file.txt")
			if err != nil {
				t.Fatal(err)
			}
			bufExpected := bytes.NewBuffer(make([]byte, 0, 256))
			defer respExpected.Body.Close()
			_, err = io.Copy(bufExpected, respExpected.Body)
			if err != nil {
				t.Fatal(err)
			}
			if !headersEqual(resp.Header, respExpected.Header) {
				t.Log(resp.Request.Method, resp.StatusCode, resp.Header)
				t.Log(respExpected.Request.Method, respExpected.StatusCode, respExpected.Header)
				t.Fatal("headers not equal")
			}
			if !bytes.Equal(buf.Bytes(), bufExpected.Bytes()) {
				t.Fatal("bodies not equal")
			}
		}()
	}
	wg.Wait()
}

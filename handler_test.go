package cache

import (
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
)

// TODO: would be nice if we didn't touch the disk

const debug = false

func init() {
	Logger = log.New(os.Stdout, "", log.LstdFlags|log.Lshortfile)
}

// TODO: test that headers and body are equal
func TestFileServer(t *testing.T) {
	dir, err := ioutil.TempDir("", "cachetest")
	if err != nil {
		t.Fatal(err)
	}
	if !debug {
		defer os.RemoveAll(dir)
	} else {
		println(dir)
	}

	fs := http.FileServer(http.Dir("testdata"))
	srv := httptest.NewServer(Handler(dir, fs))
	defer srv.Close()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Head(srv.URL + "/file.txt")
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			resp, err = http.Get(srv.URL + "/file.txt")
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			_, err = io.Copy(ioutil.Discard, resp.Body)
			if err != nil {
				t.Fatal(err)
			}
		}()
	}
	wg.Wait()
}

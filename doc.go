/*
Package cache takes http.Handler's and caches their output. API is shamelessly
modeled after nginx proxy_cache.

INCOMPLETE.  Still lots of stuff to do and test.

http.Hijacker and http.Flusher are unavailable to handler's passed through the
cache.

TODO:
- "ignore headers" configurable (for ignoring Cache-Control/Expires)
- "use stale" configurable
- gzip configurable
- perhaps use an Options struct instead of (or to supplement) method chaining
- streaming the cache to clients as it's being written? atm all long requests mean long waits
- detect malformed cache files, response body and all
- documentation
- support other storage types
*/
package cache

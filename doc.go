/*
Package cache takes http.Handler's and caches their output. API is shamelessly
modeled after nginx proxy_cache.

INCOMPLETE.  Still lots of stuff to do and test.

http.Hijacker and http.Flusher are unavailable to handler's passed through the
cache.

TODO:
- "use stale" configurable
- cache file pruning
- syncher pruning
- gzip configurable
- limit number of open reads; just do it via syncher.refs
- perhaps use an Options struct instead of (or to supplement) method chaining
- detect malformed cache files, response body and all
- documentation
- support other storage types
- "ignore headers" configurable (for ignoring Cache-Control/Expires)
*/
package cache

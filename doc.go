/*
Package cache takes http.Handler's and caches their outputs. API is shamelessly
modeled after nginx proxy_cache.

http.Hijacker and http.Flusher are unavailable to handler's passed through the
cache.

INCOMPLETE.
Still lots of stuff to do and test.
*/
package cache

## j4k.co/cache
Package cache takes http.Handler's and caches their output. API is shamelessly
modeled after nginx proxy_cache.

INCOMPLETE. Still lots of stuff to do and test.

http.Hijacker and http.Flusher are unavailable to handler's passed through the
cache.


package cache

import (
	"bytes"
	"crypto/sha1"
	"encoding/gob"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gin-contrib/cache/persistence"
	"github.com/gin-gonic/gin"
)

const (
	CACHE_MIDDLEWARE_KEY = "gincontrib.cache"
)

var (
	PageCachePrefix = "gincontrib.page.cache"
)

type responseCache struct {
	Status int
	Header http.Header
	Data   []byte
}
// RegisterResponseCacheGob registers the responseCache type with the encoding/gob package
func RegisterResponseCacheGob() {
	gob.Register(responseCache{})
}

type cachedWriter struct {
	gin.ResponseWriter
	status  int
	buffer  []byte
	written bool
	store   persistence.CacheStore
	expire  time.Duration
	key     string
}

var _ gin.ResponseWriter = &cachedWriter{}

// CreateKey creates a package specific key for a given string
func CreateKey(u string) string {
	return urlEscape(PageCachePrefix, u)
}

func urlEscape(prefix string, u string) string {
	key := url.QueryEscape(u)
	if len(key) > 200 {
		h := sha1.New()
		io.WriteString(h, u)
		key = string(h.Sum(nil))
	}
	var buffer bytes.Buffer
	buffer.WriteString(prefix)
	buffer.WriteString(":")
	buffer.WriteString(key)
	return buffer.String()
}

func newCachedWriter(store persistence.CacheStore, expire time.Duration, writer gin.ResponseWriter, key string) *cachedWriter {
	return &cachedWriter{writer, 0, make([]byte, 0), false, store, expire, key}
}

func (w *cachedWriter) WriteHeader(code int) {
	w.status = code
	w.written = true
	w.ResponseWriter.WriteHeader(code)
}

func (w *cachedWriter) Status() int {
	return w.ResponseWriter.Status()
}

func (w *cachedWriter) Written() bool {
	return w.ResponseWriter.Written()
}

func (w *cachedWriter) Write(data []byte) (int, error) {
	bytesWritten, err := w.ResponseWriter.Write(data)

	if err == nil {
		w.buffer = append(w.buffer, data[0:bytesWritten]...)
	}

	return bytesWritten, err
}

func (w *cachedWriter) WriteString(data string) (n int, err error) {
	bytesWritten, err := w.ResponseWriter.WriteString(data)

	if err == nil {
		w.buffer = append(w.buffer, []byte(data)[0:bytesWritten]...)
	}

	return bytesWritten, err
}

func (w *cachedWriter) commitToCache() {
	store := w.store

	//cache responses with a status code < 300
	if w.Status() < 300 {
		val := responseCache{
			w.Status(),
			w.Header(),
			w.buffer,
		}
		err := store.Set(w.key, val, w.expire)
		if err != nil {
			log.Println("Unable to store cache entry: " + err.Error())
		}
	}
}

// Cache Middleware
func Cache(store *persistence.CacheStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(CACHE_MIDDLEWARE_KEY, store)
		c.Next()
	}
}

func SiteCache(store persistence.CacheStore, expire time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		var cache responseCache
		url := c.Request.URL
		key := CreateKey(url.RequestURI())
		if err := store.Get(key, &cache); err != nil {
			c.Next()
		} else {
			c.Writer.WriteHeader(cache.Status)
			for k, vals := range cache.Header {
				for _, v := range vals {
					c.Writer.Header().Set(k, v)
				}
			}
			c.Writer.Write(cache.Data)
			//Since have cache hit it's fine to abort any further handlers i guess
			c.Abort()
		}
	}
}

// CachePage Decorator
func CachePage(store persistence.CacheStore, expire time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		var cache responseCache
		url := c.Request.URL
		key := CreateKey(url.RequestURI())
		if err := store.Get(key, &cache); err != nil {
			if err != persistence.ErrCacheMiss {
				log.Println(err.Error())
			}
			// replace writer
			writer := newCachedWriter(store, expire, c.Writer, key)
			c.Writer = writer
			c.Next()

			if !c.IsAborted() {
				writer.commitToCache()
			}
		} else {
			c.Writer.WriteHeader(cache.Status)
			for k, vals := range cache.Header {
				for _, v := range vals {
					c.Writer.Header().Set(k, v)
				}
			}
			c.Writer.Write(cache.Data)
			//Since have cache hit it's fine to abort any further handlers i guess
			c.Abort()
		}
	}
}

// CachePageWithoutQuery add ability to ignore GET query parameters.
func CachePageWithoutQuery(store persistence.CacheStore, expire time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		var cache responseCache
		key := CreateKey(c.Request.URL.Path)
		if err := store.Get(key, &cache); err != nil {
			if err != persistence.ErrCacheMiss {
				log.Println(err.Error())
			}
			// replace writer
			writer := newCachedWriter(store, expire, c.Writer, key)
			c.Writer = writer
			c.Next()

			if !c.IsAborted() {
				writer.commitToCache()
			}
		} else {
			c.Writer.WriteHeader(cache.Status)
			for k, vals := range cache.Header {
				for _, v := range vals {
					c.Writer.Header().Set(k, v)
				}
			}
			c.Writer.Write(cache.Data)
			//Since have cache hit it's fine to abort any further handlers i guess
			c.Abort()
		}
	}
}

// CachePageAtomic Decorator
func CachePageAtomic(store persistence.CacheStore, expire time.Duration) gin.HandlerFunc {
	var m sync.Mutex
	p := CachePage(store, expire)
	return func(c *gin.Context) {
		m.Lock()
		defer m.Unlock()
		p(c)
	}
}

func CachePageWithoutHeader(store persistence.CacheStore, expire time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		var cache responseCache
		url := c.Request.URL
		key := CreateKey(url.RequestURI())
		if err := store.Get(key, &cache); err != nil {
			if err != persistence.ErrCacheMiss {
				log.Println(err.Error())
			}
			// replace writer
			writer := newCachedWriter(store, expire, c.Writer, key)
			c.Writer = writer
			c.Next()

			if !c.IsAborted() {
				writer.commitToCache()
			}
		} else {
			c.Writer.WriteHeader(cache.Status)
			c.Writer.Write(cache.Data)
			//Since have cache hit it's fine to abort any further handlers i guess
			c.Abort()
		}
	}
}

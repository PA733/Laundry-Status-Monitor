package mw

import (
	"bytes"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
)

type cachedResponse struct {
	status  int
	headers http.Header
	body    []byte
}

type bodyCacheWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w bodyCacheWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

func (w bodyCacheWriter) WriteString(s string) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

// Cache is a middleware for in-memory caching of GET requests.
func Cache(store *cache.Cache, duration time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet {
			c.Next()
			return
		}

		key := c.Request.RequestURI
		if resp, found := store.Get(key); found {
			cached := resp.(cachedResponse)
			c.Writer.WriteHeader(cached.status)
			for k, v := range cached.headers {
				c.Writer.Header()[k] = v
			}
			c.Writer.Write(cached.body)
			c.Abort()
			return
		}

		blw := &bodyCacheWriter{body: bytes.NewBuffer(nil), ResponseWriter: c.Writer}
		c.Writer = blw

		c.Next()

		// Only cache successful responses
		if blw.Status() >= 200 && blw.Status() < 300 {
			response := cachedResponse{
				status: blw.Status(),
				// Make a copy of the header map.
				headers: blw.Header().Clone(),
				body:    blw.body.Bytes(),
			}
			store.Set(key, response, duration)
		}
	}
}

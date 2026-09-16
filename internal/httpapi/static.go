package httpapi

import (
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

// staticHandler serves the embedded site. Real files are served with their
// precompressed .br/.gz sibling when the client accepts it; any other path
// without an extension falls back to index.html so the Svelte app can route.
func staticHandler(site fs.FS) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			fail(c, http.StatusNotFound, "Not found")
			return
		}
		name := strings.TrimPrefix(path.Clean("/"+c.Request.URL.Path), "/")
		if name == "" {
			name = "index.html"
		}
		if !exists(site, name) {
			if path.Ext(name) != "" || strings.HasPrefix(name, "api/") {
				c.Status(http.StatusNotFound)
				return
			}
			name = "index.html"
		}
		h := c.Writer.Header()
		if strings.HasPrefix(name, "_app/immutable/") {
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			h.Set("Cache-Control", "no-cache")
		}
		ctype := mime.TypeByExtension(path.Ext(name))
		if ctype == "" {
			ctype = "application/octet-stream"
		}
		h.Set("Content-Type", ctype)
		h.Set("Vary", "Accept-Encoding")
		accept := c.GetHeader("Accept-Encoding")
		for _, enc := range []struct{ token, ext string }{{"br", ".br"}, {"gzip", ".gz"}} {
			if strings.Contains(accept, enc.token) && exists(site, name+enc.ext) {
				h.Set("Content-Encoding", enc.token)
				name += enc.ext
				break
			}
		}
		f, err := site.Open(name)
		if err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		defer f.Close()
		c.Status(http.StatusOK)
		if c.Request.Method == http.MethodHead {
			return
		}
		_, _ = io.Copy(c.Writer, f)
	}
}

func exists(site fs.FS, name string) bool {
	info, err := fs.Stat(site, name)
	return err == nil && !info.IsDir()
}

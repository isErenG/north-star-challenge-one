package httpapi

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
)

// securityHeaders applies to every response, API and static alike.
func securityHeaders(c *gin.Context) {
	h := c.Writer.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Content-Security-Policy", csp)
	c.Next()
}

// csp allows the app's own assets, Google Fonts, and Mapbox (styles, tiles,
// telemetry, and its blob: web worker). Nothing else may load.
const csp = "default-src 'self'; " +
	"script-src 'self' 'unsafe-inline'; " +
	"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com https://api.mapbox.com; " +
	"font-src 'self' https://fonts.gstatic.com; " +
	"img-src 'self' data: blob: https://api.mapbox.com https://*.tiles.mapbox.com; " +
	"connect-src 'self' https://api.mapbox.com https://*.tiles.mapbox.com https://events.mapbox.com; " +
	"worker-src blob:; child-src blob:; " +
	"frame-ancestors 'none'; base-uri 'self'; form-action 'self'"

func noStore(c *gin.Context) {
	c.Writer.Header().Set("Cache-Control", "no-store")
	c.Next()
}

// sameOrigin rejects state-changing requests whose Origin header does not
// match the host the request arrived on. Browsers always send Origin on
// cross-site POST/PATCH, so this blocks CSRF from other sites.
func sameOrigin(c *gin.Context) {
	if c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead {
		c.Next()
		return
	}
	origin, err := url.Parse(c.GetHeader("Origin"))
	if err != nil || origin.Host == "" || origin.Host != c.Request.Host {
		fail(c, http.StatusForbidden, "Invalid request origin")
		return
	}
	c.Next()
}

// limitBody caps request bodies so oversized uploads fail early.
func limitBody(n int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.ContentLength > n {
			fail(c, http.StatusRequestEntityTooLarge, "Upload a file smaller than 10 MB.")
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, n)
		c.Next()
	}
}

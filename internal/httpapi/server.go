// Package httpapi exposes the jobs service over HTTP with Gin and serves the
// embedded frontend. It is the only package that knows about HTTP.
package httpapi

import (
	"io/fs"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"kbo-review/internal/config"
	"kbo-review/internal/jobs"
)

// MaxUpload is the largest accepted request body.
const MaxUpload = 10 << 20

// Server wires handlers to the jobs service.
type Server struct {
	cfg config.Config
	svc *jobs.Service
}

// New builds the Gin engine. site is the built frontend to serve for every
// non-API path; pass nil to serve the API only.
func New(cfg config.Config, svc *jobs.Service, site fs.FS) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	s := &Server{cfg: cfg, svc: svc}
	r := gin.New()
	r.Use(gin.Recovery(), securityHeaders)
	r.MaxMultipartMemory = MaxUpload

	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	api := r.Group("/api", noStore, sameOrigin, limitBody(MaxUpload))
	api.GET("/config", s.getConfig)
	api.POST("/jobs", s.createJob)
	api.GET("/jobs/:id", s.getJob)
	api.PATCH("/jobs/:id", s.updateJob)

	if site != nil {
		r.NoRoute(staticHandler(site))
	} else {
		r.NoRoute(func(c *gin.Context) { fail(c, http.StatusNotFound, "Not found") })
	}
	return r
}

// Listen serves the engine on addr with the same timeouts as before.
func Listen(addr string, handler http.Handler) error {
	log.Printf("KBO review listening on %s", addr)
	srv := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second}
	return srv.ListenAndServe()
}

func fail(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(code, gin.H{"error": msg})
}

// failWith maps a jobs error to its status code.
func failWith(c *gin.Context, err error) {
	if e, ok := err.(*jobs.Error); ok {
		fail(c, e.Code, e.Msg)
		return
	}
	fail(c, http.StatusInternalServerError, "Something went wrong. Please try again.")
}

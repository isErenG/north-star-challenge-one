package httpapi

import (
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"kbo-review/internal/jobs"
	"kbo-review/internal/parse"
	"kbo-review/internal/validate"
)

func (s *Server) getConfig(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"google":           s.svc.GoogleReady(),
		"email":            s.svc.EmailReady(),
		"mapbox":           s.cfg.MapboxToken,
		"sources":          s.svc.Sources(),
		"defaultBudgetEur": s.svc.DefaultBudget(),
	})
}

func (s *Server) createJob(c *gin.Context) {
	f, h, err := c.Request.FormFile("file")
	if err != nil {
		if strings.Contains(err.Error(), "too large") || strings.Contains(err.Error(), "request body") {
			fail(c, http.StatusBadRequest, "Upload a file smaller than 10 MB.")
			return
		}
		fail(c, http.StatusBadRequest, "Choose a CSV, JSON or GeoJSON file.")
		return
	}
	defer f.Close()
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	email := strings.TrimSpace(c.PostForm("email"))
	if email != "" && (!s.svc.EmailReady() || !validate.Email(email)) {
		fail(c, http.StatusBadRequest, "A valid email and a configured notification service are required.")
		return
	}
	opts := jobs.CreateOptions{Enrich: c.PostForm("enrich") == "true"}
	if opts.Enrich {
		if b, err := strconv.ParseFloat(strings.TrimSpace(c.PostForm("budget")), 64); err == nil && b > 0 {
			opts.BudgetEUR = b
		}
		for _, key := range strings.Split(c.PostForm("sources"), ",") {
			if key = strings.ToLower(strings.TrimSpace(key)); key != "" {
				opts.Sources = append(opts.Sources, key)
			}
		}
		any := false
		for _, ready := range s.svc.Sources() {
			any = any || ready
		}
		if !any {
			fail(c, http.StatusBadRequest, "No verification source is configured.")
			return
		}
	}
	records, err := parse.File(f, strings.ToLower(filepath.Ext(h.Filename)))
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	j, err := s.svc.Create(h.Filename, records, email, opts)
	if err != nil {
		failWith(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"id": j.ID})
}

func (s *Server) getJob(c *gin.Context) {
	j, err := s.svc.Get(c.Param("id"))
	if err != nil {
		failWith(c, err)
		return
	}
	c.JSON(http.StatusOK, j)
}

func (s *Server) updateJob(c *gin.Context) {
	var in jobs.Update
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, http.StatusBadRequest, "Invalid update.")
		return
	}
	j, err := s.svc.Apply(c.Param("id"), in)
	if err != nil {
		failWith(c, err)
		return
	}
	if in.Verify != nil {
		c.JSON(http.StatusAccepted, j)
		return
	}
	c.JSON(http.StatusOK, j)
}

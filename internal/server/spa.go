package server

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/labstack/echo/v5"
)

// mountSPA serves the embedded frontend with history fallback.
func (a *App) mountSPA() {
	dist := a.dist
	fileServer := http.FileServer(http.FS(dist))
	handler := func(c *echo.Context) error {
		p := strings.TrimPrefix(path.Clean(c.Request().URL.Path), "/")
		if strings.HasPrefix(p, "api/") {
			return errNotFound
		}
		h := c.Response().Header()
		if p != "" && p != "index.html" {
			if st, err := fs.Stat(dist, p); err == nil && !st.IsDir() {
				if strings.HasPrefix(p, "assets/") {
					h.Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					h.Set("Cache-Control", "public, max-age=3600")
				}
				fileServer.ServeHTTP(c.Response(), c.Request())
				return nil
			}
			if strings.HasPrefix(p, "assets/") {
				return errNotFound
			}
		}
		idx, err := fs.ReadFile(dist, "index.html")
		if err != nil {
			return c.String(http.StatusServiceUnavailable, "frontend not built")
		}
		h.Set("Cache-Control", "no-cache")
		return c.HTMLBlob(http.StatusOK, idx)
	}
	a.Echo.GET("/*", handler)
	a.Echo.GET("/", handler)
}

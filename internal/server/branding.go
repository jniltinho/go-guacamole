package server

import (
	"context"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/labstack/echo/v5"
)

// settingBranding holds the application name shown in the sidebar, the browser tab and the sign-in page.
const settingBranding = "ui.branding"

type branding struct {
	Title string `json:"title"` // empty: the default name
}

// appTitle is the configured application name, or "" for the default.
func (a *App) appTitle(ctx context.Context) string {
	var b branding
	_ = a.Store.GetSetting(ctx, settingBranding, &b)
	return b.Title
}

func (a *App) getBranding(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, branding{Title: a.appTitle(c.Request().Context())})
}

func (a *App) putBranding(c *echo.Context) error {
	if err := requireAdmin(c); err != nil {
		return err
	}
	var in branding
	if err := c.Bind(&in); err != nil {
		return errBadRequest
	}
	in.Title = strings.TrimSpace(in.Title)
	if utf8.RuneCountInString(in.Title) > 40 || strings.IndexFunc(in.Title, unicode.IsControl) >= 0 {
		return validation("title", "up to 40 characters, no control characters")
	}
	ctx := c.Request().Context()
	old := a.appTitle(ctx)
	if err := a.Store.PutSetting(ctx, settingBranding, in); err != nil {
		return err
	}
	a.auditBy(c, "settings.branding.update", "settings", settingBranding, "branding", map[string]any{"old": old, "new": in.Title})
	return c.JSON(http.StatusOK, in)
}

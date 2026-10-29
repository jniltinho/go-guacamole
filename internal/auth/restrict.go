package auth

import (
	"errors"
	"time"

	"github.com/jniltinho/go-guacamole/internal/store"
)

// Restriction errors (codes are returned to the client).
var (
	ErrAccountDisabled = errors.New("ACCOUNT_DISABLED")
	ErrAccountExpired  = errors.New("ACCOUNT_EXPIRED")
	ErrAccessWindow    = errors.New("ACCESS_WINDOW")
)

// Location returns the user's time zone (server local when unset/invalid).
func Location(u *store.User) *time.Location {
	if u.Timezone != nil && *u.Timezone != "" {
		if loc, err := time.LoadLocation(*u.Timezone); err == nil {
			return loc
		}
	}
	return time.Local
}

// CheckRestrictions validates disabled flag, validity dates and access window
// at instant now.
func CheckRestrictions(u *store.User, now time.Time) error {
	if u.Disabled {
		return ErrAccountDisabled
	}
	local := now.In(Location(u))
	today := local.Format("2006-01-02")
	for _, d := range []*string{u.ValidFrom, u.ValidUntil} {
		if d != nil && *d != "" {
			if _, err := time.Parse("2006-01-02", *d); err != nil {
				return ErrAccountExpired // malformed date: fail closed
			}
		}
	}
	if u.ValidFrom != nil && *u.ValidFrom != "" && today < *u.ValidFrom {
		return ErrAccountExpired
	}
	if u.ValidUntil != nil && *u.ValidUntil != "" && today > *u.ValidUntil {
		return ErrAccountExpired
	}
	if inWindow, ok := withinWindow(u, local); ok && !inWindow {
		return ErrAccessWindow
	}
	return nil
}

func parseClock(s string) (int, bool) {
	for _, layout := range []string{"15:04:05", "15:04"} {
		if t, err := time.Parse(layout, s); err == nil && len(s) == len(layout) {
			return t.Hour()*3600 + t.Minute()*60 + t.Second(), true
		}
	}
	return 0, false
}

// ValidClock reports whether s is a valid HH:MM or HH:MM:SS time.
func ValidClock(s string) bool {
	_, ok := parseClock(s)
	return ok
}

func withinWindow(u *store.User, local time.Time) (inside, configured bool) {
	hasStart := u.AccessWindowStart != nil && *u.AccessWindowStart != ""
	hasEnd := u.AccessWindowEnd != nil && *u.AccessWindowEnd != ""
	if !hasStart && !hasEnd {
		return true, false
	}
	if !hasStart || !hasEnd {
		return false, true // incomplete window: fail closed
	}
	start, ok1 := parseClock(*u.AccessWindowStart)
	end, ok2 := parseClock(*u.AccessWindowEnd)
	if !ok1 || !ok2 {
		return false, true // malformed window: fail closed
	}
	cur := local.Hour()*3600 + local.Minute()*60 + local.Second()
	if start <= end {
		return cur >= start && cur < end, true
	}
	// window crosses midnight
	return cur >= start || cur < end, true
}

// NextRestrictionDeadline returns the next instant at which the user's
// current access ends (window end or validity end), or zero if none.
func NextRestrictionDeadline(u *store.User, now time.Time) time.Time {
	loc := Location(u)
	local := now.In(loc)
	var best time.Time
	consider := func(t time.Time) {
		if t.After(now) && (best.IsZero() || t.Before(best)) {
			best = t
		}
	}
	if _, configured := withinWindow(u, local); configured && u.AccessWindowEnd != nil && ValidClock(*u.AccessWindowEnd) {
		end, _ := parseClock(*u.AccessWindowEnd)
		midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
		t := midnight.Add(time.Duration(end) * time.Second)
		if !t.After(now) {
			t = t.AddDate(0, 0, 1)
		}
		consider(t)
	}
	if u.ValidUntil != nil && *u.ValidUntil != "" {
		if d, err := time.ParseInLocation("2006-01-02", *u.ValidUntil, loc); err == nil {
			consider(d.AddDate(0, 0, 1))
		}
	}
	return best
}

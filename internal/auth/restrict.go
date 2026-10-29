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
		if t, err := time.Parse(layout, s); err == nil {
			return t.Hour()*3600 + t.Minute()*60 + t.Second(), true
		}
	}
	return 0, false
}

func withinWindow(u *store.User, local time.Time) (inside, configured bool) {
	if u.AccessWindowStart == nil || u.AccessWindowEnd == nil || *u.AccessWindowStart == "" || *u.AccessWindowEnd == "" {
		return true, false
	}
	start, ok1 := parseClock(*u.AccessWindowStart)
	end, ok2 := parseClock(*u.AccessWindowEnd)
	if !ok1 || !ok2 {
		return true, false
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
	if _, configured := withinWindow(u, local); configured {
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

package humanize

import (
	"math"
	"testing"
	"time"
)

func TestPast(t *testing.T) {
	now := time.Now()
	testList{
		{"now", Time(now), "now"},
		{"1 second ago", Time(now.Add(-1 * time.Second)), "1 second ago"},
		{"12 seconds ago", Time(now.Add(-12 * time.Second)), "12 seconds ago"},
		{"30 seconds ago", Time(now.Add(-30 * time.Second)), "30 seconds ago"},
		{"45 seconds ago", Time(now.Add(-45 * time.Second)), "45 seconds ago"},
		{"1 minute ago", Time(now.Add(-63 * time.Second)), "1 minute ago"},
		{"15 minutes ago", Time(now.Add(-15 * time.Minute)), "15 minutes ago"},
		{"1 hour ago", Time(now.Add(-63 * time.Minute)), "1 hour ago"},
		{"2 hours ago", Time(now.Add(-2 * time.Hour)), "2 hours ago"},
		{"21 hours ago", Time(now.Add(-21 * time.Hour)), "21 hours ago"},
		{"1 day ago", Time(now.Add(-26 * time.Hour)), "1 day ago"},
		{"2 days ago", Time(now.Add(-49 * time.Hour)), "2 days ago"},
		{"3 days ago", Time(now.Add(-3 * Day)), "3 days ago"},
		{"1 week ago (1)", Time(now.Add(-7 * Day)), "1 week ago"},
		{"1 week ago (2)", Time(now.Add(-12 * Day)), "1 week ago"},
		{"2 weeks ago", Time(now.Add(-15 * Day)), "2 weeks ago"},
		{"1 month ago", Time(now.Add(-39 * Day)), "1 month ago"},
		{"3 months ago", Time(now.Add(-99 * Day)), "3 months ago"},
		{"1 year ago (1)", Time(now.Add(-365 * Day)), "1 year ago"},
		{"1 year ago (1)", Time(now.Add(-400 * Day)), "1 year ago"},
		{"2 years ago (1)", Time(now.Add(-548 * Day)), "2 years ago"},
		{"2 years ago (2)", Time(now.Add(-725 * Day)), "2 years ago"},
		{"2 years ago (3)", Time(now.Add(-800 * Day)), "2 years ago"},
		{"3 years ago", Time(now.Add(-3 * Year)), "3 years ago"},
		{"long ago", Time(now.Add(-LongTime)), "a long while ago"},
	}.validate(t)
}

func TestFuture(t *testing.T) {
	// Add a little time so that these things properly line up in
	// the future.
	now := time.Now().Add(time.Millisecond * 250)
	testList{
		{"now", Time(now), "now"},
		{"1 second from now", Time(now.Add(+1 * time.Second)), "1 second from now"},
		{"12 seconds from now", Time(now.Add(+12 * time.Second)), "12 seconds from now"},
		{"30 seconds from now", Time(now.Add(+30 * time.Second)), "30 seconds from now"},
		{"45 seconds from now", Time(now.Add(+45 * time.Second)), "45 seconds from now"},
		{"15 minutes from now", Time(now.Add(+15 * time.Minute)), "15 minutes from now"},
		{"2 hours from now", Time(now.Add(+2 * time.Hour)), "2 hours from now"},
		{"21 hours from now", Time(now.Add(+21 * time.Hour)), "21 hours from now"},
		{"1 day from now", Time(now.Add(+26 * time.Hour)), "1 day from now"},
		{"2 days from now", Time(now.Add(+49 * time.Hour)), "2 days from now"},
		{"3 days from now", Time(now.Add(+3 * Day)), "3 days from now"},
		{"1 week from now (1)", Time(now.Add(+7 * Day)), "1 week from now"},
		{"1 week from now (2)", Time(now.Add(+12 * Day)), "1 week from now"},
		{"2 weeks from now", Time(now.Add(+15 * Day)), "2 weeks from now"},
		{"1 month from now", Time(now.Add(+30 * Day)), "1 month from now"},
		{"1 year from now", Time(now.Add(+365 * Day)), "1 year from now"},
		{"2 years from now", Time(now.Add(+2 * Year)), "2 years from now"},
		{"a while from now", Time(now.Add(+LongTime)), "a long while from now"},
	}.validate(t)
}

func TestRange(t *testing.T) {
	start := time.Time{}
	end := time.Unix(math.MaxInt64, math.MaxInt64)
	x := RelTime(start, end, "ago", "from now")
	if x != "a long while from now" {
		t.Errorf("Expected a long while from now, got %q", x)
	}
}

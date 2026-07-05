package ui

import (
	"fmt"
	"time"
)

// Dash is rendered for absent values.
const Dash = "—"

// timeLayouts are attempted in order when parsing service timestamps.
// Python services may emit ISO 8601 without a timezone offset.
var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02T15:04:05",
}

// Odds renders American odds with an explicit sign: +150, -110.
func Odds(american int) string {
	return fmt.Sprintf("%+d", american)
}

// Percent renders a 0..1 probability as a percentage with one decimal.
func Percent(p float64) string {
	return fmt.Sprintf("%.1f%%", p*100)
}

// PercentPtr renders a nullable probability, or a dash when absent.
func PercentPtr(p *float32) string {
	if p == nil {
		return Dash
	}
	return Percent(float64(*p))
}

// EdgePercent renders an edge given in percentage points (4.2 = 4.2%)
// with an explicit sign and one decimal.
func EdgePercent(points float64) string {
	return fmt.Sprintf("%+.1f%%", points)
}

// Units renders a signed unit amount with two decimals: +1.25, -0.50.
func Units(u float64) string {
	return fmt.Sprintf("%+.2f", u)
}

// UnitsPtr renders a nullable signed unit amount, or a dash when absent.
func UnitsPtr(u *float32) string {
	if u == nil {
		return Dash
	}
	return Units(float64(*u))
}

// Stake renders an unsigned unit amount with two decimals.
func Stake(u float64) string {
	return fmt.Sprintf("%.2f", u)
}

// LineValue renders a nullable point spread or total, or a dash.
func LineValue(v *float32) string {
	if v == nil {
		return Dash
	}
	return fmt.Sprintf("%+.1f", *v)
}

// Timestamp parses an RFC 3339-ish timestamp string and renders it in
// local time as "Jan 02 15:04". Unparseable values pass through verbatim.
func Timestamp(s string) string {
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return TimeShort(t)
		}
	}
	return s
}

// TimeShort renders a time in the local timezone as "Jan 02 15:04".
func TimeShort(t time.Time) string {
	return t.Local().Format("Jan 02 15:04")
}

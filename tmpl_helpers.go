package main

import (
	"fmt"
	"html/template"
	"time"
)

// tmplFuncs returns the template.FuncMap used by all templates.
func tmplFuncs() template.FuncMap {
	return template.FuncMap{
		"formatTime":     formatTime,
		"formatDuration": formatDuration,
		"formatPct":      formatPct,
		"statusClass":    statusClass,
		"sparkline":      sparklineFunc,
		"dict":           dict,
		"lt":             func(a, b int) bool { return a < b },
		"passName":       passName,
	}
}

// formatTime renders a timestamp string in a human-readable form.
// Input is the SQLite datetime string "YYYY-MM-DD HH:MM:SS".
func formatTime(ts string) string {
	t, err := time.Parse("2006-01-02 15:04:05", ts)
	if err != nil {
		return ts
	}
	return t.Format("2006-01-02 15:04:05 MST")
}

// formatDuration renders milliseconds as a human-readable string.
func formatDuration(ms float64) string {
	d := time.Duration(ms) * time.Millisecond
	switch {
	case d < time.Millisecond:
		return fmt.Sprintf("%.0fµs", ms*1000)
	case d < time.Second:
		return fmt.Sprintf("%.0fms", ms)
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", ms/1000)
	default:
		m := int(d.Minutes())
		s := d.Seconds() - float64(m*60)
		return fmt.Sprintf("%dm%.0fs", m, s)
	}
}

// formatPct renders a float as a percentage string.
func formatPct(v float64) string {
	return fmt.Sprintf("%.2f%%", v)
}

// statusClass returns a CSS class for the pass value.
func statusClass(pass int) string {
	switch pass {
	case 2:
		return "pass"
	case 1:
		return "degraded"
	default:
		return "fail"
	}
}

// sparklineFunc wraps Sparkline for use in templates. It accepts a typed
// DB history slice and extracts common fields for rendering.
func sparklineFunc(history interface{}, width, height int) template.HTML {
	pts := extractPoints(history)
	return Sparkline(pts, width, height)
}

// dict builds a map from alternating key/value pairs. Used in templates
// to pass multiple values to sub-templates.
func dict(values ...interface{}) (map[string]interface{}, error) {
	if len(values)%2 != 0 {
		return nil, fmt.Errorf("dict: odd number of arguments")
	}
	m := make(map[string]interface{}, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key %d is not a string", i)
		}
		m[key] = values[i+1]
	}
	return m, nil
}

// calcUptimePct returns the uptime percentage (pass or degraded / total) for a set of points.
func calcUptimePct(pts []checkPoint) float64 {
	if len(pts) == 0 {
		return 0
	}
	passCount := 0
	for _, p := range pts {
		if p.pass >= 1 {
			passCount++
		}
	}
	return float64(passCount) / float64(len(pts)) * 100
}

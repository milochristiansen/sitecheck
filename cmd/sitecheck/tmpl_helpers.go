package main

import (
	"fmt"
	"html/template"
	"time"

	"sitecheck/core"
)

// tmplFuncs returns the template.FuncMap used by all templates.
func tmplFuncs() template.FuncMap {
	return template.FuncMap{
		"formatTime":       formatTime,
		"formatDuration":   formatDuration,
		"formatDurationMS": formatDurationMS,
		"formatPct":        formatPct,
		"statusClass":      statusClass,
		"passName":         passName,
		"dict":             dict,
		"windowLabel":      windowLabel,
	}
}

// formatTime renders a timestamp string in a human-readable form. Input is the SQLite datetime string "YYYY-MM-DD
// HH:MM:SS".
func formatTime(ts string) string {
	t, err := time.Parse("2006-01-02 15:04:05", ts)
	if err != nil {
		return ts
	}
	return t.Format("2006-01-02 15:04:05 MST")
}

// windowLabel renders the heading for a graph window: named for the 30-day chart,
// "Last Nh" for everything else.
func windowLabel(w int) string {
	if w == 720 {
		return "Last 30 days"
	}
	return fmt.Sprintf("Last %dh", w)
}

// formatDuration renders milliseconds as a human-readable string.
func formatDuration(ms float64) string {
	if ms >= 1000 {
		return fmt.Sprintf("%.2fs", ms/1000)
	}
	return fmt.Sprintf("%.1fms", ms)
}

// formatDurationMS renders int64 milliseconds as a human-readable string.
func formatDurationMS(ms int64) string {
	return formatDuration(float64(ms))
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
	case 0:
		return "fail"
	case core.UNKNOWN:
		return "error"
	default:
		return "unknown"
	}
}

// dict builds a map from alternating key/value pairs. Used in templates to pass multiple values to sub-templates.
func dict(values ...interface{}) (map[string]interface{}, error) {
	if len(values)%2 != 0 {
		return nil, fmt.Errorf("dict requires even number of arguments")
	}
	m := make(map[string]interface{}, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict keys must be strings")
		}
		m[key] = values[i+1]
	}
	return m, nil
}

// calcUptimePct returns the uptime percentage (pass or degraded / total) for a set of points.
func calcUptimePct(pts []core.CheckPoint) float64 {
	if len(pts) == 0 {
		return 0
	}
	var ok int
	for _, p := range pts {
		if p.Pass == 2 || p.Pass == 1 {
			ok++
		}
	}
	return float64(ok) / float64(len(pts)) * 100
}

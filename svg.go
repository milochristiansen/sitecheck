package main

import (
	"fmt"
	"html/template"
	"strings"

	"sitecheck/db"
)

// checkPoint is an internal type used by sparkline, line chart and uptime calculations.
// It extracts the common fields from any typed DB check struct.
type checkPoint struct {
	pass int
	resp float64
	ts   string
}

// extractPoints converts a typed DB check slice to the common internal form.
func extractPoints(history interface{}) []checkPoint {
	switch h := history.(type) {
	case []db.HTTPCheck:
		pts := make([]checkPoint, len(h))
		for i, c := range h {
			pts[i] = checkPoint{c.Pass, c.ResponseTimeMS, c.Timestamp}
		}
		return pts
	case []db.PingCheck:
		pts := make([]checkPoint, len(h))
		for i, c := range h {
			pts[i] = checkPoint{c.Pass, c.ResponseTimeMS, c.Timestamp}
		}
		return pts
	case []db.TCPCheck:
		pts := make([]checkPoint, len(h))
		for i, c := range h {
			pts[i] = checkPoint{c.Pass, c.ResponseTimeMS, c.Timestamp}
		}
		return pts
	case []db.DNSCheck:
		pts := make([]checkPoint, len(h))
		for i, c := range h {
			pts[i] = checkPoint{c.Pass, c.ResponseTimeMS, c.Timestamp}
		}
		return pts
	case []db.SSLCheck:
		pts := make([]checkPoint, len(h))
		for i, c := range h {
			pts[i] = checkPoint{c.Pass, c.ResponseTimeMS, c.Timestamp}
		}
		return pts
	}
	return nil
}

// Sparkline returns an inline SVG sparkline for the given check points.
// If there are fewer than 2 points, returns an empty string.
func Sparkline(pts []checkPoint, width, height int) template.HTML {
	if len(pts) < 2 {
		return template.HTML("")
	}

	pad := 4.0
	plotW := float64(width) - pad*2
	plotH := float64(height) - pad*2

	// Find value range.
	minV, maxV := pts[0].resp, pts[0].resp
	for _, p := range pts[1:] {
		if p.resp < minV {
			minV = p.resp
		}
		if p.resp > maxV {
			maxV = p.resp
		}
	}
	// Ensure at least 1ms range so we don't divide by zero.
	rangeV := maxV - minV
	if rangeV < 1 {
		rangeV = 1
	}

	// Build polyline points.
	var coords []string
	for i, p := range pts {
		x := pad + float64(i)/float64(len(pts)-1)*plotW
		y := pad + plotH - (p.resp-minV)/rangeV*plotH
		coords = append(coords, fmt.Sprintf("%.1f,%.1f", x, y))
	}

	// Build circle dots with status color.
	var dots strings.Builder
	for i, p := range pts {
		x := pad + float64(i)/float64(len(pts)-1)*plotW
		y := pad + plotH - (p.resp-minV)/rangeV*plotH
		color := pointColor(p.pass)
		fmt.Fprintf(&dots, `<circle cx="%.1f" cy="%.1f" r="2" fill="%s"/>`, x, y, color)
	}

	svg := fmt.Sprintf(
		`<svg width="%d" height="%d" viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg">`+
			`<polyline fill="none" stroke="#64748b" stroke-width="1.5" points="%s"/>`+
			`%s`+
			`</svg>`,
		width, height, width, height,
		strings.Join(coords, " "),
		dots.String(),
	)
	return template.HTML(svg)
}

// LineChart returns a full inline SVG line chart with axes, grid, and labels.
// pts must be in chronological order (oldest first).
func LineChart(pts []checkPoint, width, height int) template.HTML {
	if len(pts) < 2 {
		return template.HTML("")
	}

	const (
		padLeft   = 60.0
		padRight  = 16.0
		padTop    = 12.0
		padBottom = 40.0
	)
	plotW := float64(width) - padLeft - padRight
	plotH := float64(height) - padTop - padBottom

	// Value range.
	minV, maxV := pts[0].resp, pts[0].resp
	for _, p := range pts[1:] {
		if p.resp < minV {
			minV = p.resp
		}
		if p.resp > maxV {
			maxV = p.resp
		}
	}
	if maxV-minV < 1 {
		maxV = minV + 1
	}
	rangeV := maxV - minV

	// Y-axis ticks.
	yticks := yTicks(minV, maxV)

	var b strings.Builder
	b.WriteString(fmt.Sprintf(
		`<svg width="%d" height="%d" viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg">`,
		width, height, width, height,
	))

	// Background.
	b.WriteString(fmt.Sprintf(
		`<rect x="%g" y="%g" width="%g" height="%g" fill="#1e293b" stroke="#334155" stroke-width="1"/>`,
		padLeft, padTop, plotW, plotH,
	))

	// Grid lines and Y labels.
	for _, yv := range yticks {
		y := padTop + plotH - (yv-minV)/rangeV*plotH
		b.WriteString(fmt.Sprintf(
			`<line x1="%g" y1="%g" x2="%g" y2="%g" stroke="#334155" stroke-width="1" stroke-dasharray="4,3"/>`,
			padLeft, y, padLeft+plotW, y,
		))
		b.WriteString(fmt.Sprintf(
			`<text x="%g" y="%g" fill="#94a3b8" font-size="11" text-anchor="end" dominant-baseline="middle">%s</text>`,
			padLeft-6, y, formatMS(yv),
		))
	}

	// X-axis time labels (up to ~8 labels).
	n := len(pts)
	step := 1
	if n > 8 {
		step = n / 8
	}
	for i := 0; i < n; i += step {
		x := padLeft + float64(i)/float64(n-1)*plotW
		label := shortTime(pts[i].ts)
		b.WriteString(fmt.Sprintf(
			`<text x="%g" y="%g" fill="#94a3b8" font-size="10" text-anchor="middle">%s</text>`,
			x, float64(height)-6, label,
		))
	}

	// Data polyline.
	var coords []string
	for i, p := range pts {
		x := padLeft + float64(i)/float64(n-1)*plotW
		y := padTop + plotH - (p.resp-minV)/rangeV*plotH
		coords = append(coords, fmt.Sprintf("%.1f,%.1f", x, y))
	}
	b.WriteString(fmt.Sprintf(
		`<polyline fill="none" stroke="#22d3ee" stroke-width="2" points="%s"/>`,
		strings.Join(coords, " "),
	))

	// Data points (dots).
	for i, p := range pts {
		x := padLeft + float64(i)/float64(n-1)*plotW
		y := padTop + plotH - (p.resp-minV)/rangeV*plotH
		color := pointColor(p.pass)
		b.WriteString(fmt.Sprintf(
			`<circle cx="%.1f" cy="%.1f" r="3" fill="%s" stroke="#0f172a" stroke-width="1"/>`,
			x, y, color,
		))
	}

	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// yTicks returns nice round tick values covering [minV, maxV] with 4–5 ticks.
func yTicks(minV, maxV float64) []float64 {
	rough := (maxV - minV) / 4
	step := niceStep(rough)
	lo := float64(int(minV/step)) * step
	if lo < minV {
		lo += step
	}
	var ticks []float64
	for v := lo; v <= maxV+step/2; v += step {
		ticks = append(ticks, v)
	}
	return ticks
}

func niceStep(rough float64) float64 {
	if rough <= 0 {
		return 1
	}
	exp := 1.0
	for exp*10 <= rough {
		exp *= 10
	}
	for exp > rough {
		exp /= 10
	}
	for _, m := range []float64{1, 2, 5, 10} {
		if m*exp >= rough {
			return m * exp
		}
	}
	return 10 * exp
}

// formatMS formats milliseconds as a short label.
func formatMS(ms float64) string {
	if ms >= 1000 {
		return fmt.Sprintf("%.1fs", ms/1000)
	}
	return fmt.Sprintf("%.0fms", ms)
}

// shortTime extracts HH:MM from a SQLite timestamp.
func shortTime(ts string) string {
	if len(ts) >= 16 {
		return ts[11:16]
	}
	return ts
}

func pointColor(pass int) string {
	switch pass {
	case 2:
		return "#22c55e"
	case 1:
		return "#eab308"
	default:
		return "#ef4444"
	}
}

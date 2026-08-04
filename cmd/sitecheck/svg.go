package main

import (
	"fmt"
	"html/template"
	"math"
	"strings"
	"time"

	"sitecheck/checktypes/registry"
)

// sqliteTimeFormat is the layout of the SQLite datetime strings used as check timestamps.
const sqliteTimeFormat = "2006-01-02 15:04:05"

// checkPoint is an internal type used by sparkline, line chart and uptime calculations. It extracts the common fields
// from any typed DB check struct.
type checkPoint struct {
	pass int
	resp float64
	ts   string
}

// extractPoints converts a typed DB check slice to the common internal form via the plugin.
func extractPoints(history interface{}, p registry.CheckPlugin) []checkPoint {
	if history == nil || p == nil {
		return nil
	}
	pts := p.ExtractPoints(history)
	out := make([]checkPoint, len(pts))
	for i, pt := range pts {
		out[i] = checkPoint{pt.Pass, pt.Resp, pt.TS}
	}
	return out
}

// extractDurationPoints is like extractPoints but uses duration fields. Only types that support duration charts
// return non-empty results.
func extractDurationPoints(history interface{}, p registry.CheckPlugin) []checkPoint {
	if history == nil || p == nil {
		return nil
	}
	pts := p.ExtractDurationPoints(history)
	out := make([]checkPoint, len(pts))
	for i, pt := range pts {
		out[i] = checkPoint{pt.Pass, pt.Resp, pt.TS}
	}
	return out
}

// Sparkline returns an inline SVG sparkline for the given check points. If there are fewer than 2 points, returns an
// empty string.
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

// renderLineChart draws a single line chart SVG of the given size. cls, when non-empty,
// is added to the <svg> element's class attribute. pts must be in chronological order
// (oldest first). The x-axis always spans exactly [windowStart, windowEnd]; each point is
// plotted at its time position within that window (points outside it are clamped to the
// edges), so the time scale is fixed no matter how many points exist.
func renderLineChart(pts []checkPoint, width, height int, windowStart, windowEnd time.Time, cls string) template.HTML {
	if len(pts) < 2 {
		return template.HTML("")
	}
	windowDur := windowEnd.Sub(windowStart)
	if windowDur <= 0 {
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

	// Position points by their timestamp within the fixed window. Rows with unparseable
	// timestamps are skipped.
	type placed struct {
		pt   checkPoint
		x, y float64
	}
	points := make([]placed, 0, len(pts))
	minV, maxV := pts[0].resp, pts[0].resp
	for _, p := range pts {
		t, err := time.Parse(sqliteTimeFormat, p.ts)
		if err != nil {
			continue
		}
		frac := float64(t.Sub(windowStart)) / float64(windowDur)
		if frac < 0 {
			frac = 0
		} else if frac > 1 {
			frac = 1
		}
		if p.resp < minV {
			minV = p.resp
		}
		if p.resp > maxV {
			maxV = p.resp
		}
		points = append(points, placed{pt: p, x: padLeft + frac*plotW})
	}
	if len(points) < 2 {
		return template.HTML("")
	}
	minV, maxV = widenFlatRange(minV, maxV)
	rangeV := maxV - minV
	for i := range points {
		points[i].y = padTop + plotH - (points[i].pt.resp-minV)/rangeV*plotH
	}

	// Y-axis ticks.
	yticks := yTicks(minV, maxV)

	var b strings.Builder
	classAttr := ""
	if cls != "" {
		classAttr = ` class="` + cls + `"`
	}
	b.WriteString(fmt.Sprintf(
		`<svg%s width="%d" height="%d" viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg">`,
		classAttr, width, height, width, height,
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

	// X-axis time labels at fixed positions across the window.
	for _, tk := range chartTicks(windowStart, windowEnd) {
		x := padLeft + float64(tk.Sub(windowStart))/float64(windowDur)*plotW
		b.WriteString(fmt.Sprintf(
			`<text x="%g" y="%g" fill="#94a3b8" font-size="10" text-anchor="middle">%s</text>`,
			x, float64(height)-6, chartTimeLabel(tk, windowDur),
		))
	}

	// Data polyline.
	coords := make([]string, len(points))
	for i, p := range points {
		coords[i] = fmt.Sprintf("%.1f,%.1f", p.x, p.y)
	}
	b.WriteString(fmt.Sprintf(
		`<polyline fill="none" stroke="#22d3ee" stroke-width="2" points="%s"/>`,
		strings.Join(coords, " "),
	))

	// Data points (dots) with a native tooltip per dot.
	for _, p := range points {
		b.WriteString(fmt.Sprintf(
			`<circle cx="%.1f" cy="%.1f" r="3" fill="%s" stroke="#0f172a" stroke-width="1"><title>%s</title></circle>`,
			p.x, p.y, pointColor(p.pt.pass), pointTitle(p.pt),
		))
	}

	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// LineChart returns a full inline SVG line chart with axes, grid, and labels at the given
// size. See renderLineChart for the fixed time-scale semantics.
func LineChart(pts []checkPoint, width, height int, windowStart, windowEnd time.Time) template.HTML {
	return renderLineChart(pts, width, height, windowStart, windowEnd, "")
}

// Sizes for the responsive 24h chart pair. The wide version matches the detail page's
// maximum content width (1200px main minus 2*1rem padding). Both versions share the same
// height — they differ only in viewport width, so a larger device gets a wider chart,
// not a taller one. CSS shows one or the other at 100% based on device width.
const (
	chartNarrowW, chartNarrowH = 700, 280
	chartWideW, chartWideH     = 1168, 280
)

// LineChartPair returns two renders of the same chart in one HTML string: a
// chart-wide version sized for the full page width and a chart-narrow version at the
// standard size. Both carry class attributes so CSS can display exactly one of them.
func LineChartPair(pts []checkPoint, windowStart, windowEnd time.Time) template.HTML {
	if len(pts) < 2 {
		return template.HTML("")
	}
	wide := renderLineChart(pts, chartWideW, chartWideH, windowStart, windowEnd, "chart-wide")
	narrow := renderLineChart(pts, chartNarrowW, chartNarrowH, windowStart, windowEnd, "chart-narrow")
	if wide == "" && narrow == "" {
		return template.HTML("")
	}
	return wide + narrow
}

// chartBucketWindow is the aggregation window covered by each dot of the 30-day chart.
const chartBucketWindow = 8 * time.Hour

// bucket aggregates the checks that fell in one chartBucketWindow of the 30-day chart.
type bucket struct {
	count                         int
	sum                           float64
	pass, degraded, fail, unknown int
}

// ThirtyDayChart renders the 30-day chart: one dot per 6-hour window (120 total) across
// the fixed window [windowStart, windowEnd]. Each dot sits at its window's time position,
// its y encodes the average response time over that window, and it is colored by the
// worst single check in the window (fail > degraded > unknown > pass). A native tooltip
// per dot shows the window start, the average, and the pass/degraded/fail/unknown counts.
func renderThirtyDayChart(pts []checkPoint, width, height int, windowStart, windowEnd time.Time, cls string) template.HTML {
	if len(pts) < 2 {
		return template.HTML("")
	}
	windowDur := windowEnd.Sub(windowStart)
	if windowDur <= 0 {
		return template.HTML("")
	}
	nBuckets := int(windowDur / chartBucketWindow)
	if nBuckets < 1 {
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

	// Bucket points into UTC chartBucketWindow slots anchored to the window start.
	buckets := make([]bucket, nBuckets)
	gridStart := windowStart.Truncate(chartBucketWindow)
	for _, p := range pts {
		t, err := time.Parse(sqliteTimeFormat, p.ts)
		if err != nil {
			continue
		}
		th := t.Truncate(chartBucketWindow)
		if th.Before(gridStart) {
			continue
		}
		k := int(th.Sub(gridStart) / chartBucketWindow)
		if k >= nBuckets {
			// The window end can fall mid-window; fold that partial final window (points
			// with k == nBuckets, i.e. in [gridStart+n*bucket, windowEnd)) into the last
			// bucket so the newest checks are never dropped.
			k = nBuckets - 1
		}
		b := &buckets[k]
		b.count++
		b.sum += p.resp
		switch p.pass {
		case 2:
			b.pass++
		case 1:
			b.degraded++
		case -1:
			b.unknown++
		default:
			b.fail++
		}
	}

	// Y range over bucket averages.
	first := true
	minV, maxV := 0.0, 0.0
	for i := range buckets {
		if buckets[i].count == 0 {
			continue
		}
		avg := buckets[i].sum / float64(buckets[i].count)
		if first {
			minV, maxV = avg, avg
			first = false
		} else {
			if avg < minV {
				minV = avg
			}
			if avg > maxV {
				maxV = avg
			}
		}
	}
	if first {
		return template.HTML("")
	}
	minV, maxV = widenFlatRange(minV, maxV)
	rangeV := maxV - minV

	// Y-axis ticks.
	yticks := yTicks(minV, maxV)

	var b strings.Builder
	classAttr := ""
	if cls != "" {
		classAttr = ` class="` + cls + `"`
	}
	b.WriteString(fmt.Sprintf(
		`<svg%s width="%d" height="%d" viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg">`,
		classAttr, width, height, width, height,
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

	// X-axis time labels at fixed positions across the window.
	for _, tk := range chartTicks(windowStart, windowEnd) {
		x := padLeft + float64(tk.Sub(windowStart))/float64(windowDur)*plotW
		b.WriteString(fmt.Sprintf(
			`<text x="%g" y="%g" fill="#94a3b8" font-size="10" text-anchor="middle">%s</text>`,
			x, float64(height)-6, chartTimeLabel(tk, windowDur),
		))
	}

	// Dot geometry: one dot per bucket window at its fixed-window position. Empty
	// windows sit on the zero baseline.
	xs := make([]float64, nBuckets)
	ys := make([]float64, nBuckets)
	for k := range buckets {
		center := gridStart.Add(time.Duration(k)*chartBucketWindow + chartBucketWindow/2)
		frac := float64(center.Sub(windowStart)) / float64(windowDur)
		if frac < 0 {
			frac = 0
		} else if frac > 1 {
			frac = 1
		}
		xs[k] = padLeft + frac*plotW
		if buckets[k].count == 0 {
			ys[k] = padTop + plotH // zero baseline
		} else {
			ys[k] = padTop + plotH - (buckets[k].sum/float64(buckets[k].count)-minV)/rangeV*plotH
		}
	}

	// Connecting line: one polyline through every bucket dot, so periods without checks
	// sit on the zero baseline and the line dips there between bursts of data.
	coords := make([]string, nBuckets)
	for k := range buckets {
		coords[k] = fmt.Sprintf("%.1f,%.1f", xs[k], ys[k])
	}
	b.WriteString(fmt.Sprintf(
		`<polyline fill="none" stroke="#22d3ee" stroke-width="1.5" points="%s"/>`,
		strings.Join(coords, " "),
	))

	// Dots on top of the connecting line.
	for k := range buckets {
		start := gridStart.Add(time.Duration(k) * chartBucketWindow)
		b.WriteString(fmt.Sprintf(
			`<circle cx="%.1f" cy="%.1f" r="2" fill="%s"><title>%s</title></circle>`,
			xs[k], ys[k], bucketColor(&buckets[k]), bucketTitle(&buckets[k], start),
		))
	}

	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// ThirtyDayChart returns the 30-day chart as a single SVG at the given size. See
// renderThirtyDayChart for the bucket semantics.
func ThirtyDayChart(pts []checkPoint, width, height int, windowStart, windowEnd time.Time) template.HTML {
	return renderThirtyDayChart(pts, width, height, windowStart, windowEnd, "")
}

// ThirtyDayChartPair returns two renders of the 30-day chart (page-width and standard),
// mirroring the responsive pair used for the 24h chart. CSS shows one at a time based on
// device width.
func ThirtyDayChartPair(pts []checkPoint, windowStart, windowEnd time.Time) template.HTML {
	if len(pts) < 2 {
		return template.HTML("")
	}
	wide := renderThirtyDayChart(pts, chartWideW, chartWideH, windowStart, windowEnd, "chart-wide")
	narrow := renderThirtyDayChart(pts, chartNarrowW, chartNarrowH, windowStart, windowEnd, "chart-narrow")
	if wide == "" && narrow == "" {
		return template.HTML("")
	}
	return wide + narrow
}

// bucketColor picks the dot color for one bucket: red if any check failed, else yellow if
// any degraded, else purple if any unknown, green only when every check passed, and
// muted gray when the window has no checks.
func bucketColor(b *bucket) string {
	if b.count == 0 {
		return "#64748b"
	}
	if b.fail > 0 {
		return "#ef4444"
	}
	if b.degraded > 0 {
		return "#eab308"
	}
	if b.unknown > 0 {
		return "#8b5cf6"
	}
	return "#22c55e"
}

// bucketTitle is the native tooltip for one bucket dot: the window start, the average
// response time, and the pass/degraded/fail/unknown counts.
func bucketTitle(b *bucket, start time.Time) string {
	if b.count == 0 {
		return fmt.Sprintf("%s\nNo checks in this period", start.Format("2006-01-02 15:04"))
	}
	return fmt.Sprintf("%s\navg %s\n%d pass, %d degraded, %d fail, %d unknown",
		start.Format("2006-01-02 15:04"),
		formatMS(b.sum/float64(b.count)),
		b.pass, b.degraded, b.fail, b.unknown)
}

// chartTicks returns tick times for the x-axis on a fixed grid spanning
// [windowStart, windowEnd]: 6h for windows up to a day, daily for up to a week, 5 days
// beyond that. The grid is aligned to step boundaries (midnight UTC for whole-day steps),
// so labels are stable from one page generation to the next.
func chartTicks(windowStart, windowEnd time.Time) []time.Time {
	windowDur := windowEnd.Sub(windowStart)
	step := 24 * time.Hour
	switch {
	case windowDur <= 24*time.Hour:
		step = 6 * time.Hour
	case windowDur <= 7*24*time.Hour:
		step = 24 * time.Hour
	default:
		step = 5 * 24 * time.Hour
	}
	if step > windowDur {
		step = windowDur / 4
	}
	t := windowStart.Truncate(step).UTC()
	for t.Before(windowStart) {
		t = t.Add(step)
	}
	var ticks []time.Time
	for ; !t.After(windowEnd); t = t.Add(step) {
		ticks = append(ticks, t)
	}
	return ticks
}

// chartTimeLabel formats an x-axis tick: clock time for windows up to 48h, month-day
// beyond that.
func chartTimeLabel(t time.Time, windowDur time.Duration) string {
	if windowDur <= 48*time.Hour {
		return t.Format("15:04")
	}
	return t.Format("01-02")
}

// widenFlatRange expands a near-flat value range so the y-axis ticks don't all collapse
// onto one label (e.g. everything reading "2.0s"). The range is widened by 10% of the
// max value (at least 1ms) on each side, clamped so the axis never goes negative.
func widenFlatRange(minV, maxV float64) (float64, float64) {
	if maxV-minV < math.Max(maxV*0.1, 1) {
		pad := math.Max(maxV*0.1, 1)
		minV -= pad
		if minV < 0 {
			minV = 0
		}
		maxV += pad
	}
	return minV, maxV
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
		return fmt.Sprintf("%.2fs", ms/1000)
	}
	return fmt.Sprintf("%.0fms", ms)
}

func pointColor(pass int) string {
	switch pass {
	case 2:
		return "#22c55e"
	case 1:
		return "#eab308"
	case -1:
		return "#8b5cf6"
	default:
		return "#ef4444"
	}
}

// pointTitle is the native tooltip for a line-chart dot: the check's date and time on
// the first line, its response time on the second.
func pointTitle(pt checkPoint) string {
	return fmt.Sprintf("%s\n%s", pt.ts, formatMS(pt.resp))
}

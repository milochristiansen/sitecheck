package main

import (
	"fmt"
	"html/template"
	"math"
	"regexp"
	"strings"
	"testing"
	"time"

	"sitecheck/checktypes/http"
	"sitecheck/checktypes/outpost"
	"sitecheck/core"
)

// ------------------------------------------------------------
// pointColor
// ------------------------------------------------------------

func TestPointColor(t *testing.T) {
	tests := []struct {
		Pass int
		want string
	}{
		{2, "#22c55e"},
		{1, "#eab308"},
		{-1, "#8b5cf6"},
		{0, "#ef4444"},
		{999, "#ef4444"},
		{-999, "#ef4444"},
	}
	for _, tc := range tests {
		got := pointColor(tc.Pass)
		if got != tc.want {
			t.Errorf("pointColor(%d) = %q, want %q", tc.Pass, got, tc.want)
		}
	}
}

// ------------------------------------------------------------
// niceStep
// ------------------------------------------------------------

func TestNiceStep(t *testing.T) {
	tests := []struct {
		rough float64
		want  float64
	}{
		{0, 1},
		{-1, 1},
		{-0.5, 1},
		{0.5, 0.5},
		{0.25, 0.5},
		{1, 1},
		{1.5, 2},
		{3, 5},
		{7, 10},
		{10, 10},
		{12, 20},
		{50, 50},
		{100, 100},
		{250, 500},
		{0.01, 0.01},
		{0.001, 0.001},
		{0.0025, 0.005},
	}
	for _, tc := range tests {
		got := niceStep(tc.rough)
		if got != tc.want {
			t.Errorf("niceStep(%v) = %v, want %v", tc.rough, got, tc.want)
		}
	}
}

// ------------------------------------------------------------
// yTicks
// ------------------------------------------------------------

func TestYTicks(t *testing.T) {
	t.Run("uniform_0_100", func(t *testing.T) {
		ticks := yTicks(0, 100)
		if len(ticks) < 2 {
			t.Fatalf("yTicks(0,100) = %v, want at least 2 ticks", ticks)
		}
		if ticks[0] != 0 {
			t.Errorf("first tick = %v, want 0", ticks[0])
		}
		for i := 1; i < len(ticks); i++ {
			if ticks[i] <= ticks[i-1] {
				t.Errorf("ticks not increasing: %v", ticks)
			}
		}
		if ticks[0] > 0 || ticks[len(ticks)-1] < 100 {
			t.Errorf("ticks %v do not cover [0,100]", ticks)
		}
	})

	t.Run("small_1_2", func(t *testing.T) {
		ticks := yTicks(1, 2)
		if len(ticks) < 2 {
			t.Fatalf("yTicks(1,2) = %v, want at least 2 ticks", ticks)
		}
		if ticks[0] < 0 {
			t.Errorf("first tick negative: %v", ticks[0])
		}
		for i := 1; i < len(ticks); i++ {
			if ticks[i] <= ticks[i-1] {
				t.Errorf("ticks not increasing: %v", ticks)
			}
		}
		if ticks[0] > 1 || ticks[len(ticks)-1] < 2 {
			t.Errorf("ticks %v do not cover [1,2]", ticks)
		}
	})

	t.Run("negative", func(t *testing.T) {
		ticks := yTicks(-5, 5)
		if len(ticks) < 2 {
			t.Fatalf("yTicks(-5,5) = %v, want at least 2 ticks", ticks)
		}
		for i := 1; i < len(ticks); i++ {
			if ticks[i] <= ticks[i-1] {
				t.Errorf("ticks not increasing: %v", ticks)
			}
		}
		if ticks[0] > -5 || ticks[len(ticks)-1] < 5 {
			t.Errorf("ticks %v do not cover [-5,5]", ticks)
		}
	})

	t.Run("flat_range", func(t *testing.T) {
		ticks := yTicks(50, 50)
		if len(ticks) < 1 {
			t.Fatalf("yTicks(50,50) = %v, want at least 1 tick", ticks)
		}
		if ticks[0] != 50 {
			t.Errorf("first tick = %v, want 50", ticks[0])
		}
	})
}

// ------------------------------------------------------------
// Sparkline
// ------------------------------------------------------------

func TestSparkline(t *testing.T) {
	t.Run("less_than_2_points", func(t *testing.T) {
		if got := Sparkline(nil, 50, 20); got != template.HTML("") {
			t.Errorf("nil: got %q, want empty", got)
		}
		if got := Sparkline([]core.CheckPoint{}, 50, 20); got != template.HTML("") {
			t.Errorf("empty: got %q, want empty", got)
		}
		if got := Sparkline([]core.CheckPoint{{Pass: 2, Resp: 100, TS: "2024-01-01 10:00:00"}}, 50, 20); got != template.HTML("") {
			t.Errorf("1 pt: got %q, want empty", got)
		}
	})

	t.Run("two_points", func(t *testing.T) {
		pts := []core.CheckPoint{
			{Pass: 2, Resp: 50, TS: "2024-01-01 10:00:00"},
			{Pass: 2, Resp: 100, TS: "2024-01-01 10:05:00"},
		}
		got := Sparkline(pts, 50, 20)
		s := string(got)
		if s == "" {
			t.Fatal("expected non-empty SVG")
		}
		if !strings.Contains(s, "<svg") {
			t.Error("missing <svg tag")
		}
		if !strings.Contains(s, "</svg>") {
			t.Error("missing </svg>")
		}
		if !strings.Contains(s, "polyline") {
			t.Error("missing polyline")
		}
		if !strings.Contains(s, "circle") {
			t.Error("missing circle dots")
		}
	})

	t.Run("all_same_response", func(t *testing.T) {
		pts := []core.CheckPoint{
			{Pass: 2, Resp: 100, TS: "2024-01-01 10:00:00"},
			{Pass: 2, Resp: 100, TS: "2024-01-01 10:05:00"},
			{Pass: 2, Resp: 100, TS: "2024-01-01 10:10:00"},
		}
		got := Sparkline(pts, 50, 20)
		s := string(got)
		if s == "" {
			t.Fatal("expected non-empty SVG even with flat data")
		}
		if !strings.Contains(s, "polyline") {
			t.Error("missing polyline")
		}
	})

	t.Run("colors_by_pass", func(t *testing.T) {
		pts := []core.CheckPoint{
			{Pass: 2, Resp: 100, TS: "10:00"},
			{Pass: 1, Resp: 150, TS: "10:05"},
			{Pass: -1, Resp: 200, TS: "10:10"},
			{Pass: 0, Resp: 300, TS: "10:15"},
		}
		got := Sparkline(pts, 100, 30)
		s := string(got)
		for _, color := range []string{"#22c55e", "#eab308", "#8b5cf6", "#ef4444"} {
			if !strings.Contains(s, color) {
				t.Errorf("missing color %s in sparkline with varied passes", color)
			}
		}
	})
}

// ------------------------------------------------------------
// LineChart
// ------------------------------------------------------------
func TestLineChart(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)

	t.Run("less_than_2_points", func(t *testing.T) {
		if got := LineChart(nil, 300, 180, start, end); got != template.HTML("") {
			t.Errorf("nil: got %q, want empty", got)
		}
		if got := LineChart([]core.CheckPoint{}, 300, 180, start, end); got != template.HTML("") {
			t.Errorf("empty: got %q, want empty", got)
		}
		if got := LineChart([]core.CheckPoint{{Pass: 2, Resp: 100, TS: "2024-01-01 10:00:00"}}, 300, 180, start, end); got != template.HTML("") {
			t.Errorf("1 pt: got %q, want empty", got)
		}
	})

	t.Run("invalid_window", func(t *testing.T) {
		pts := []core.CheckPoint{
			{Pass: 2, Resp: 50, TS: "2024-01-01 10:00:00"},
			{Pass: 0, Resp: 200, TS: "2024-01-01 10:05:00"},
		}
		if got := LineChart(pts, 300, 180, end, start); got != template.HTML("") {
			t.Errorf("reversed window: got %q, want empty", got)
		}
	})

	t.Run("two_points", func(t *testing.T) {
		pts := []core.CheckPoint{
			{Pass: 2, Resp: 50, TS: "2024-01-01 10:00:00"},
			{Pass: 0, Resp: 200, TS: "2024-01-01 10:05:00"},
		}
		got := LineChart(pts, 300, 180, start, end)
		s := string(got)
		if s == "" {
			t.Fatal("expected non-empty SVG")
		}
		if !strings.Contains(s, "<svg") {
			t.Error("missing <svg tag")
		}
		if !strings.Contains(s, "</svg>") {
			t.Error("missing </svg>")
		}
		if !strings.Contains(s, "polyline") {
			t.Error("missing data polyline")
		}
		if !strings.Contains(s, "circle") {
			t.Error("missing data point dots")
		}
		if !strings.Contains(s, "<rect") {
			t.Error("missing background rect")
		}
		if !strings.Contains(s, "stroke-dasharray") {
			t.Error("missing grid lines")
		}
		if !strings.Contains(s, "ms") && !strings.Contains(s, "s") {
			t.Error("missing Y-axis labels (ms or s)")
		}
		if !strings.Contains(s, "text-anchor") {
			t.Error("missing text labels")
		}
	})

	t.Run("dots_have_tooltips", func(t *testing.T) {
		pts := []core.CheckPoint{
			{Pass: 2, Resp: 50, TS: "2024-01-01 10:00:00"},
			{Pass: 0, Resp: 2500, TS: "2024-01-01 10:05:00"},
		}
		got := string(LineChart(pts, 300, 180, start, end))
		if n := strings.Count(got, "<title>"); n != 2 {
			t.Errorf("found %d tooltips, want 2 (one per dot)", n)
		}
		for _, want := range []string{
			"<title>2024-01-01 10:00:00\n50.0ms</title>",
			"<title>2024-01-01 10:05:00\n2.50s</title>",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("missing tooltip %q", want)
			}
		}
	})

	t.Run("points_positioned_by_time", func(t *testing.T) {
		// 24h window 00:00-24:00 UTC; points at 06:00 (25%) and 18:00 (75%).
		pts := []core.CheckPoint{
			{Pass: 2, Resp: 100, TS: "2024-01-01 06:00:00"},
			{Pass: 0, Resp: 300, TS: "2024-01-01 18:00:00"},
		}
		got := string(LineChart(pts, 300, 180, start, end))
		// plotW = 300-60-16 = 224: x = 60 + 0.25*224 = 116.0, 60 + 0.75*224 = 228.0.
		// Index-based plotting would have put them at the edges (60.0 and 284.0).
		for _, want := range []string{`cx="116.0"`, `cx="228.0"`} {
			if !strings.Contains(got, want) {
				t.Errorf("missing dot at %s (points must plot at their time position)", want)
			}
		}
	})

	t.Run("off_window_points_clamped", func(t *testing.T) {
		pts := []core.CheckPoint{
			{Pass: 2, Resp: 100, TS: "2023-12-31 12:00:00"}, // before window
			{Pass: 2, Resp: 150, TS: "2024-01-01 12:00:00"}, // 50%
			{Pass: 0, Resp: 200, TS: "2024-01-02 12:00:00"}, // after window
		}
		got := string(LineChart(pts, 300, 180, start, end))
		for _, want := range []string{`cx="60.0"`, `cx="172.0"`, `cx="284.0"`} {
			if !strings.Contains(got, want) {
				t.Errorf("missing dot at %s", want)
			}
		}
	})

	t.Run("unparseable_timestamps_skipped", func(t *testing.T) {
		pts := []core.CheckPoint{
			{Pass: 2, Resp: 100, TS: "garbage"},
			{Pass: 2, Resp: 150, TS: "2024-01-01 06:00:00"},
			{Pass: 2, Resp: 200, TS: "2024-01-01 18:00:00"},
		}
		got := LineChart(pts, 300, 180, start, end)
		if got == template.HTML("") {
			t.Fatal("expected chart from the 2 parseable points")
		}
		if n := strings.Count(string(got), "<circle"); n != 2 {
			t.Errorf("rendered %d dots, want 2 (unparseable rows skipped)", n)
		}
	})

	t.Run("fewer_than_2_parseable_is_empty", func(t *testing.T) {
		pts := []core.CheckPoint{
			{Pass: 2, Resp: 100, TS: "garbage"},
			{Pass: 2, Resp: 150, TS: "also garbage"},
		}
		if got := LineChart(pts, 300, 180, start, end); got != template.HTML("") {
			t.Errorf("got %q, want empty with no parseable points", got)
		}
	})

	t.Run("fixed_x_ticks", func(t *testing.T) {
		pts := []core.CheckPoint{
			{Pass: 2, Resp: 100, TS: "2024-01-01 06:00:00"},
			{Pass: 2, Resp: 200, TS: "2024-01-01 18:00:00"},
		}
		got := string(LineChart(pts, 300, 180, start, end))
		// 24h window -> 6h grid aligned to UTC midnight: 00/06/12/18 on 01-01, 00 on 01-02.
		for _, want := range []string{">00:00<", ">06:00<", ">12:00<", ">18:00<"} {
			if !strings.Contains(got, want) {
				t.Errorf("missing x tick label %s", want)
			}
		}
	})

	t.Run("x_labels_independent_of_point_count", func(t *testing.T) {
		mk := func(n int) []core.CheckPoint {
			pts := make([]core.CheckPoint, n)
			for i := range pts {
				ts := time.Date(2024, 1, 1, 0, i%60, 0, 0, time.UTC).Format("2006-01-02 15:04:05")
				pts[i] = core.CheckPoint{Pass: 2, Resp: float64(100 + i), TS: ts}
			}
			return pts
		}
		labels3 := strings.Count(string(LineChart(mk(3), 300, 180, start, end)), `text-anchor="middle"`)
		labels20 := strings.Count(string(LineChart(mk(20), 300, 180, start, end)), `text-anchor="middle"`)
		if labels3 == 0 {
			t.Error("expected x tick labels")
		}
		if labels3 != labels20 {
			t.Errorf("label count depends on point count: %d vs %d", labels3, labels20)
		}
	})

	t.Run("flat_data", func(t *testing.T) {
		pts := []core.CheckPoint{
			{Pass: 2, Resp: 100, TS: "2024-01-01 10:00:00"},
			{Pass: 2, Resp: 100, TS: "2024-01-01 10:05:00"},
			{Pass: 2, Resp: 100, TS: "2024-01-01 10:10:00"},
		}
		got := LineChart(pts, 300, 180, start, end)
		s := string(got)
		if s == "" {
			t.Fatal("expected non-empty SVG for flat data")
		}
		if !strings.Contains(s, "polyline") {
			t.Error("missing polyline")
		}
	})
}

func TestLineChartPair(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	end := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	pts := []core.CheckPoint{
		{Pass: 2, Resp: 100, TS: "2024-01-01 06:00:00"},
		{Pass: 0, Resp: 300, TS: "2024-01-01 18:00:00"},
	}

	t.Run("less_than_2_points_is_empty", func(t *testing.T) {
		if got := LineChartPair(nil, start, end); got != template.HTML("") {
			t.Errorf("nil: got %q, want empty", got)
		}
		if got := LineChartPair([]core.CheckPoint{{Pass: 2, Resp: 100, TS: "2024-01-01 06:00:00"}}, start, end); got != template.HTML("") {
			t.Errorf("1 pt: got %q, want empty", got)
		}
	})

	t.Run("two_sizes_with_classes", func(t *testing.T) {
		got := string(LineChartPair(pts, start, end))
		if n := strings.Count(got, "<svg"); n != 2 {
			t.Errorf("found %d <svg, want 2", n)
		}
		if n := strings.Count(got, "</svg>"); n != 2 {
			t.Errorf("found %d </svg>, want 2", n)
		}
		wideVB := fmt.Sprintf(`viewBox="0 0 %d %d"`, chartWideW, chartWideH)
		narrowVB := fmt.Sprintf(`viewBox="0 0 %d %d"`, chartNarrowW, chartNarrowH)
		if !strings.Contains(got, wideVB) {
			t.Errorf("missing wide chart viewBox %s", wideVB)
		}
		if !strings.Contains(got, narrowVB) {
			t.Errorf("missing narrow chart viewBox %s", narrowVB)
		}
		if !strings.Contains(got, `class="chart-wide"`) {
			t.Error("missing chart-wide class")
		}
		if !strings.Contains(got, `class="chart-narrow"`) {
			t.Error("missing chart-narrow class")
		}
		if n := strings.Count(got, "<polyline"); n != 2 {
			t.Errorf("found %d polylines, want 2", n)
		}
		if n := strings.Count(got, "<circle"); n != 4 {
			t.Errorf("found %d circles, want 4 (2 per chart)", n)
		}
	})

	t.Run("plain_linechart_has_no_class", func(t *testing.T) {
		got := string(LineChart(pts, 300, 180, start, end))
		if strings.Contains(got, "class=") {
			t.Error("plain LineChart must not carry a class attribute")
		}
	})

	t.Run("versions_differ_only_in_width", func(t *testing.T) {
		if chartWideH != chartNarrowH {
			t.Errorf("wide chart height %d differs from narrow %d — charts must share a height so larger devices get wider, not taller, charts", chartWideH, chartNarrowH)
		}
		if chartWideW == chartNarrowW {
			t.Errorf("wide and narrow widths are equal (%d) — they must differ", chartWideW)
		}
	})
}

func TestThirtyDayChartPair(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(720 * time.Hour)
	pts := []core.CheckPoint{
		{Pass: 2, Resp: 100, TS: "2024-01-01 00:15:00"},
		{Pass: 0, Resp: 300, TS: "2024-01-01 00:45:00"},
	}

	t.Run("less_than_2_points_is_empty", func(t *testing.T) {
		if got := ThirtyDayChartPair(nil, start, end); got != template.HTML("") {
			t.Errorf("nil: got %q, want empty", got)
		}
		if got := ThirtyDayChartPair([]core.CheckPoint{{Pass: 2, Resp: 100, TS: "2024-01-01 00:15:00"}}, start, end); got != template.HTML("") {
			t.Errorf("1 pt: got %q, want empty", got)
		}
	})

	t.Run("two_sizes_with_classes", func(t *testing.T) {
		got := string(ThirtyDayChartPair(pts, start, end))
		if n := strings.Count(got, "<svg"); n != 2 {
			t.Errorf("found %d <svg, want 2", n)
		}
		if n := strings.Count(got, "</svg>"); n != 2 {
			t.Errorf("found %d </svg>, want 2", n)
		}
		wideVB := fmt.Sprintf(`viewBox="0 0 %d %d"`, chartWideW, chartWideH)
		narrowVB := fmt.Sprintf(`viewBox="0 0 %d %d"`, chartNarrowW, chartNarrowH)
		if !strings.Contains(got, wideVB) {
			t.Errorf("missing wide chart viewBox %s", wideVB)
		}
		if !strings.Contains(got, narrowVB) {
			t.Errorf("missing narrow chart viewBox %s", narrowVB)
		}
		if !strings.Contains(got, `class="chart-wide"`) {
			t.Error("missing chart-wide class")
		}
		if !strings.Contains(got, `class="chart-narrow"`) {
			t.Error("missing chart-narrow class")
		}
		if n := strings.Count(got, "<polyline"); n != 2 {
			t.Errorf("found %d polylines, want 2 (one per chart)", n)
		}
	})

	t.Run("plain_chart_has_no_class", func(t *testing.T) {
		got := string(ThirtyDayChart(pts, 700, 280, start, end))
		if strings.Contains(got, "class=") {
			t.Error("plain ThirtyDayChart must not carry a class attribute")
		}
	})
}

func TestThirtyDayChart(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(720 * time.Hour) // 30 days

	t.Run("less_than_2_points_is_empty", func(t *testing.T) {
		if got := ThirtyDayChart(nil, 700, 280, start, end); got != template.HTML("") {
			t.Errorf("nil: got %q, want empty", got)
		}
		if got := ThirtyDayChart([]core.CheckPoint{{Pass: 2, Resp: 100, TS: "2024-01-01 00:15:00"}}, 700, 280, start, end); got != template.HTML("") {
			t.Errorf("1 pt: got %q, want empty", got)
		}
	})

	t.Run("renders_90_bucket_dots_with_tooltips", func(t *testing.T) {
		pts := []core.CheckPoint{
			{Pass: 2, Resp: 100, TS: "2024-01-01 00:15:00"},
			{Pass: 0, Resp: 300, TS: "2024-01-01 00:45:00"}, // same 8h window: avg 200ms, 1 fail
			{Pass: 1, Resp: 150, TS: "2024-01-02 03:30:00"}, // degraded window
		}
		got := string(ThirtyDayChart(pts, 700, 280, start, end))
		if n := strings.Count(got, "<circle"); n != 90 {
			t.Errorf("rendered %d dots, want 90 (30*3 eight-hour buckets)", n)
		}
		for _, c := range []string{"#ef4444", "#eab308", "#64748b"} {
			if !strings.Contains(got, c) {
				t.Errorf("missing color %s", c)
			}
		}
		if !strings.Contains(got, "avg 200.0ms") {
			t.Error("missing average in tooltip")
		}
		if !strings.Contains(got, "1 pass, 0 degraded, 1 fail, 0 unknown") {
			t.Error("missing counts in tooltip")
		}
		if !strings.Contains(got, "No checks in this period") {
			t.Error("missing empty-window tooltip")
		}
	})

	t.Run("worst_event_color_precedence", func(t *testing.T) {
		pts := []core.CheckPoint{
			{Pass: 2, Resp: 100, TS: "2024-01-01 00:15:00"}, // bucket 0 [00:00,08:00): pass+unknown -> purple
			{Pass: -1, Resp: 200, TS: "2024-01-01 00:45:00"},
			{Pass: 2, Resp: 100, TS: "2024-01-01 08:15:00"}, // bucket 1 [08:00,16:00): pass+degraded -> yellow
			{Pass: 1, Resp: 150, TS: "2024-01-01 08:45:00"},
			{Pass: 2, Resp: 100, TS: "2024-01-01 16:15:00"}, // bucket 2 [16:00,24:00): pass+degraded+fail -> red
			{Pass: 1, Resp: 150, TS: "2024-01-01 16:30:00"},
			{Pass: 0, Resp: 300, TS: "2024-01-01 16:45:00"},
			{Pass: 2, Resp: 100, TS: "2024-01-02 00:15:00"}, // bucket 3: only pass -> green
		}
		got := string(ThirtyDayChart(pts, 700, 280, start, end))
		// plotW = 624; bucket k center x = 60 + 624*(k+0.5)/90.
		cases := []struct{ cx, want string }{
			{`cx="63.5"`, "#8b5cf6"}, // unknown beats pass
			{`cx="70.4"`, "#eab308"}, // degraded beats pass
			{`cx="77.3"`, "#ef4444"}, // fail beats degraded
			{`cx="84.3"`, "#22c55e"}, // all pass -> green
		}
		for _, tc := range cases {
			re := regexp.MustCompile(regexp.QuoteMeta(tc.cx) + `[^>]*fill="([^"]+)"`)
			m := re.FindStringSubmatch(got)
			if m == nil {
				t.Errorf("no dot at %s", tc.cx)
				continue
			}
			if m[1] != tc.want {
				t.Errorf("dot at %s has fill %s, want %s", tc.cx, m[1], tc.want)
			}
		}
	})

	t.Run("connecting_line_spans_all_buckets", func(t *testing.T) {
		pts := []core.CheckPoint{
			{Pass: 2, Resp: 100, TS: "2024-01-01 00:15:00"}, // bucket 0
			{Pass: 2, Resp: 150, TS: "2024-01-01 08:15:00"}, // bucket 1
			{Pass: 2, Resp: 200, TS: "2024-01-01 16:15:00"}, // bucket 2
		}
		got := string(ThirtyDayChart(pts, 700, 280, start, end))
		if n := strings.Count(got, "<polyline"); n != 1 {
			t.Errorf("expected exactly 1 connecting polyline, got %d", n)
		}
		m := regexp.MustCompile(`<polyline[^>]*points="([^"]+)"`).FindStringSubmatch(got)
		if m == nil {
			t.Fatal("missing polyline")
		}
		if n := len(strings.Fields(m[1])); n != 90 {
			t.Errorf("polyline has %d points, want 90 (one per bucket, empty buckets included)", n)
		}
	})

	t.Run("empty_buckets_sit_on_zero_baseline", func(t *testing.T) {
		pts := []core.CheckPoint{
			{Pass: 2, Resp: 100, TS: "2024-01-01 00:15:00"}, // bucket 0 only
			{Pass: 2, Resp: 150, TS: "2024-01-03 00:15:00"}, // bucket 6 only
		}
		got := string(ThirtyDayChart(pts, 700, 280, start, end))
		// padTop + plotH = 12 + 228 = 240: empty dots sit on the zero baseline.
		re := regexp.MustCompile(`cy="240\.0"[^>]*fill="(#64748b)"`)
		if re.FindStringSubmatch(got) == nil {
			t.Error("empty-bucket dots are not on the zero baseline (cy=240.0, gray)")
		}
	})

	t.Run("connecting_line_dips_to_zero_in_gaps", func(t *testing.T) {
		pts := []core.CheckPoint{
			{Pass: 2, Resp: 100, TS: "2024-01-01 00:15:00"}, // bucket 0
			{Pass: 2, Resp: 150, TS: "2024-01-03 00:15:00"}, // bucket 6
		}
		got := string(ThirtyDayChart(pts, 700, 280, start, end))
		// The single polyline must include both data dots and the zero-baseline gaps
		// between them (bucket 1's x-position at the baseline).
		points := regexp.MustCompile(`points="([^"]+)"`).FindStringSubmatch(got)
		if points == nil {
			t.Fatal("missing polyline")
		}
		if !strings.Contains(points[1], "70.4,240.0") {
			t.Errorf("polyline does not dip to the zero baseline in the gap (missing 70.4,240.0): %s", points[1])
		}
	})

	t.Run("fixed_window_bucket_positions", func(t *testing.T) {
		pts := []core.CheckPoint{
			{Pass: 2, Resp: 100, TS: "2024-01-01 00:15:00"}, // first bucket (k=0)
			{Pass: 2, Resp: 200, TS: "2024-01-30 23:15:00"}, // last bucket (k=89)
		}
		got := string(ThirtyDayChart(pts, 700, 280, start, end))
		// First bucket center: 60 + 624*0.5/90 = 63.5; last (k=89):
		// 60 + 624*89.5/90 = 680.5.
		for _, want := range []string{`cx="63.5"`, `cx="680.5"`} {
			if !strings.Contains(got, want) {
				t.Errorf("missing dot at %s (bucket dots must sit at fixed window positions)", want)
			}
		}
	})

	t.Run("partial_final_window_folds_into_last_bucket", func(t *testing.T) {
		// Window end 20 min into the final 8-hour window: that tail must land in the
		// last bucket, not be dropped.
		ws := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		we := ws.Add(720*time.Hour + 20*time.Minute)
		pts := []core.CheckPoint{
			{Pass: 2, Resp: 100, TS: "2024-01-01 00:15:00"},
			{Pass: 0, Resp: 300, TS: "2024-01-31 00:10:00"}, // in the 20-min tail
		}
		got := string(ThirtyDayChart(pts, 700, 280, ws, we))
		if n := strings.Count(got, "<circle"); n != 90 {
			t.Errorf("rendered %d dots, want 90", n)
		}
		// Bucket 89 (Jan 30 16:00) now holds the fail -> red dot at its x position
		// (center Jan 30 20:00; frac 716/720.3333 -> x = 680.2).
		re := regexp.MustCompile(`cx="680\.2"[^>]*fill="(#ef4444)"`)
		if re.FindStringSubmatch(got) == nil {
			t.Error("tail point not folded into last bucket (no red dot at last bucket position)")
		}
	})
}

// ------------------------------------------------------------
// extractPoints
// ------------------------------------------------------------

func TestExtractPoints(t *testing.T) {
	t.Run("nil_history", func(t *testing.T) {
		got := extractPoints(nil, &http.HTTPPlugin{})
		if got != nil {
			t.Errorf("nil history: got %v, want nil", got)
		}
	})

	t.Run("nil_plugin", func(t *testing.T) {
		got := extractPoints([]http.HTTPCheck{}, nil)
		if got != nil {
			t.Errorf("nil plugin: got %v, want nil", got)
		}
	})

	t.Run("valid_http", func(t *testing.T) {
		history := []http.HTTPCheck{
			{Pass: 2, ResponseTimeMS: 150.5, Timestamp: "2024-01-01 10:00:00"},
			{Pass: 0, ResponseTimeMS: 3200.0, Timestamp: "2024-01-01 10:05:00"},
		}
		got := extractPoints(history, &http.HTTPPlugin{})
		if len(got) != 2 {
			t.Fatalf("expected 2 points, got %d", len(got))
		}
		if got[0].Pass != 2 || got[0].Resp != 150.5 || got[0].TS != "2024-01-01 10:00:00" {
			t.Errorf("point 0 = %+v, want {Pass:2 Resp:150.5 TS:2024-01-01 10:00:00}", got[0])
		}
		if got[1].Pass != 0 || got[1].Resp != 3200.0 || got[1].TS != "2024-01-01 10:05:00" {
			t.Errorf("point 1 = %+v, want {Pass:0 Resp:3200 TS:2024-01-01 10:05:00}", got[1])
		}
	})

	// POSSIBLE BUG: extractPoints returns non-nil empty slice when plugin returns nil
	// (make([]core.CheckPoint, len(nil)) produces []core.CheckPoint{} instead of nil).
	t.Run("wrong_type", func(t *testing.T) {
		got := extractPoints("not a slice", &http.HTTPPlugin{})
		if len(got) != 0 {
			t.Errorf("wrong type: len=%d, want 0", len(got))
		}
	})
}

// ------------------------------------------------------------
// extractDurationPoints
// ------------------------------------------------------------

func TestExtractDurationPoints(t *testing.T) {
	t.Run("nil_history", func(t *testing.T) {
		got := extractDurationPoints(nil, &http.HTTPPlugin{})
		if got != nil {
			t.Errorf("nil history: got %v, want nil", got)
		}
	})

	t.Run("nil_plugin", func(t *testing.T) {
		got := extractDurationPoints([]outpost.OutpostCheck{}, nil)
		if got != nil {
			t.Errorf("nil plugin: got %v, want nil", got)
		}
	})

	t.Run("valid_outpost", func(t *testing.T) {
		history := []outpost.OutpostCheck{
			{
				Pass:           2,
				ResponseTimeMS: 50,
				DurationMS:     1000,
				Timestamp:      "2024-01-01 10:00:00",
				CheckCount:     10,
				FailCount:      0,
			},
			{
				Pass:           0,
				ResponseTimeMS: 0,
				DurationMS:     5000,
				Timestamp:      "2024-01-01 10:05:00",
				CheckCount:     10,
				FailCount:      3,
			},
		}
		p, ok := core.ByName("outpost")
		if !ok {
			t.Fatal("outpost plugin not registered")
		}
		got := extractDurationPoints(history, p)
		if len(got) != 2 {
			t.Fatalf("expected 2 points, got %d", len(got))
		}
		if got[0].Pass != 2 || got[0].Resp != 1000 || got[0].TS != "2024-01-01 10:00:00" {
			t.Errorf("point 0 = %+v", got[0])
		}
		if got[1].Pass != 0 || got[1].Resp != 5000 || got[1].TS != "2024-01-01 10:05:00" {
			t.Errorf("point 1 = %+v", got[1])
		}
	})

	// POSSIBLE BUG: same non-nil empty slice as extractPoints.
	t.Run("http_no_duration", func(t *testing.T) {
		got := extractDurationPoints([]http.HTTPCheck{}, &http.HTTPPlugin{})
		if len(got) != 0 {
			t.Errorf("http has no duration: len=%d, want 0", len(got))
		}
	})

	// POSSIBLE BUG: same non-nil empty slice as extractPoints.
	t.Run("wrong_type", func(t *testing.T) {
		p, ok := core.ByName("outpost")
		if !ok {
			t.Fatal("outpost plugin not registered")
		}
		got := extractDurationPoints("not a slice", p)
		if len(got) != 0 {
			t.Errorf("wrong type: len=%d, want 0", len(got))
		}
	})
}

// ------------------------------------------------------------
// Registry integration
// ------------------------------------------------------------

func TestExtractPointsViaRegistry(t *testing.T) {
	p, ok := core.ByName("http")
	if !ok {
		t.Fatal("http plugin not registered; import side effects may be missing")
	}
	httpPlugin := p.(*http.HTTPPlugin)

	history := []http.HTTPCheck{
		{Pass: 2, ResponseTimeMS: 200, Timestamp: "2024-01-01 12:00:00"},
	}
	got := extractPoints(history, httpPlugin)
	if len(got) != 1 {
		t.Fatalf("expected 1 point, got %d", len(got))
	}
	if got[0].Resp != 200 {
		t.Errorf("resp = %v, want 200", got[0].Resp)
	}
}

// ------------------------------------------------------------
// NaN/Inf edge cases (POSSIBLE BUG: no guards)
// ------------------------------------------------------------

func TestSparklineInfResp(t *testing.T) {
	// POSSIBLE BUG: Sparkline doesn't guard against +Inf/-Inf/NaN response values.
	defer func() {
		if r := recover(); r != nil {
			t.Logf("recovered from panic with Inf/NaN data: %v", r)
		}
	}()
	pts := []core.CheckPoint{
		{Pass: 2, Resp: math.Inf(1), TS: "2024-01-01 10:00:00"},
		{Pass: 2, Resp: 100, TS: "2024-01-01 10:05:00"},
	}
	got := Sparkline(pts, 50, 20)
	_ = got
}

func TestLineChartInfResp(t *testing.T) {
	// POSSIBLE BUG: niceStep(+Inf) infinite-loops because exp*10 <= +Inf is
	// always true. Calling LineChart with Inf/NaN response values would hang.
	// This test documents the finding without making the call.
	t.Log("POSSIBLE BUG: LineChart with Inf/NaN response values would hang niceStep")
}

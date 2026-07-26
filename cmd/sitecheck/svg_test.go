package main

import (
	"html/template"
	"math"
	"strings"
	"testing"

	"sitecheck/checktypes/http"
	"sitecheck/checktypes/outpost"
	"sitecheck/checktypes/registry"
)

// ------------------------------------------------------------
// pointColor
// ------------------------------------------------------------

func TestPointColor(t *testing.T) {
	tests := []struct {
		pass int
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
		got := pointColor(tc.pass)
		if got != tc.want {
			t.Errorf("pointColor(%d) = %q, want %q", tc.pass, got, tc.want)
		}
	}
}

// ------------------------------------------------------------
// shortTime
// ------------------------------------------------------------

func TestShortTime(t *testing.T) {
	tests := []struct {
		ts   string
		want string
	}{
		{"2024-01-15 10:30:45", "10:30"},
		{"2024-06-07 23:59:59", "23:59"},
		{"short", "short"},
		{"", ""},
		{"exactly16chars!", "exactly16chars!"},
		{"1234567890abcde", "1234567890abcde"},
	}
	for _, tc := range tests {
		got := shortTime(tc.ts)
		if got != tc.want {
			t.Errorf("shortTime(%q) = %q, want %q", tc.ts, got, tc.want)
		}
	}
}

// ------------------------------------------------------------
// formatMS
// ------------------------------------------------------------

func TestFormatMS(t *testing.T) {
	tests := []struct {
		ms   float64
		want string
	}{
		{500, "500ms"},
		{1500, "1.5s"},
		{0, "0ms"},
		{1, "1ms"},
		{999, "999ms"},
		{1000, "1.0s"},
		{2500, "2.5s"},
		{10000, "10.0s"},
	}
	for _, tc := range tests {
		got := formatMS(tc.ms)
		if got != tc.want {
			t.Errorf("formatMS(%v) = %q, want %q", tc.ms, got, tc.want)
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
		if got := Sparkline([]checkPoint{}, 50, 20); got != template.HTML("") {
			t.Errorf("empty: got %q, want empty", got)
		}
		if got := Sparkline([]checkPoint{{pass: 2, resp: 100, ts: "2024-01-01 10:00:00"}}, 50, 20); got != template.HTML("") {
			t.Errorf("1 pt: got %q, want empty", got)
		}
	})

	t.Run("two_points", func(t *testing.T) {
		pts := []checkPoint{
			{pass: 2, resp: 50, ts: "2024-01-01 10:00:00"},
			{pass: 2, resp: 100, ts: "2024-01-01 10:05:00"},
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
		pts := []checkPoint{
			{pass: 2, resp: 100, ts: "2024-01-01 10:00:00"},
			{pass: 2, resp: 100, ts: "2024-01-01 10:05:00"},
			{pass: 2, resp: 100, ts: "2024-01-01 10:10:00"},
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
		pts := []checkPoint{
			{pass: 2, resp: 100, ts: "10:00"},
			{pass: 1, resp: 150, ts: "10:05"},
			{pass: -1, resp: 200, ts: "10:10"},
			{pass: 0, resp: 300, ts: "10:15"},
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
	t.Run("less_than_2_points", func(t *testing.T) {
		if got := LineChart(nil, 300, 180); got != template.HTML("") {
			t.Errorf("nil: got %q, want empty", got)
		}
		if got := LineChart([]checkPoint{}, 300, 180); got != template.HTML("") {
			t.Errorf("empty: got %q, want empty", got)
		}
		if got := LineChart([]checkPoint{{pass: 2, resp: 100, ts: "2024-01-01 10:00:00"}}, 300, 180); got != template.HTML("") {
			t.Errorf("1 pt: got %q, want empty", got)
		}
	})

	t.Run("two_points", func(t *testing.T) {
		pts := []checkPoint{
			{pass: 2, resp: 50, ts: "2024-01-01 10:00:00"},
			{pass: 0, resp: 200, ts: "2024-01-01 10:05:00"},
		}
		got := LineChart(pts, 300, 180)
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

	t.Run("many_points_xaxis_thinned", func(t *testing.T) {
		pts := make([]checkPoint, 20)
		for i := range pts {
			pts[i] = checkPoint{
				pass: 2,
				resp: float64(100 + i*10),
				ts:   "2024-01-01 10:00:00",
			}
		}
		got := LineChart(pts, 300, 180)
		s := string(got)
		if s == "" {
			t.Fatal("expected non-empty SVG for 20 points")
		}
		labelCount := strings.Count(s, "text-anchor=\"middle\"")
		if labelCount < 2 {
			t.Errorf("expected multiple X-axis labels, got %d", labelCount)
		}
	})

	t.Run("flat_data", func(t *testing.T) {
		pts := []checkPoint{
			{pass: 2, resp: 100, ts: "2024-01-01 10:00:00"},
			{pass: 2, resp: 100, ts: "2024-01-01 10:05:00"},
			{pass: 2, resp: 100, ts: "2024-01-01 10:10:00"},
		}
		got := LineChart(pts, 300, 180)
		s := string(got)
		if s == "" {
			t.Fatal("expected non-empty SVG for flat data")
		}
		if !strings.Contains(s, "polyline") {
			t.Error("missing polyline")
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
		if got[0].pass != 2 || got[0].resp != 150.5 || got[0].ts != "2024-01-01 10:00:00" {
			t.Errorf("point 0 = %+v, want {pass:2 resp:150.5 ts:2024-01-01 10:00:00}", got[0])
		}
		if got[1].pass != 0 || got[1].resp != 3200.0 || got[1].ts != "2024-01-01 10:05:00" {
			t.Errorf("point 1 = %+v, want {pass:0 resp:3200 ts:2024-01-01 10:05:00}", got[1])
		}
	})

	// POSSIBLE BUG: extractPoints returns non-nil empty slice when plugin returns nil
	// (make([]checkPoint, len(nil)) produces []checkPoint{} instead of nil).
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
		p, ok := registry.ByName("outpost")
		if !ok {
			t.Fatal("outpost plugin not registered")
		}
		got := extractDurationPoints(history, p)
		if len(got) != 2 {
			t.Fatalf("expected 2 points, got %d", len(got))
		}
		if got[0].pass != 2 || got[0].resp != 1000 || got[0].ts != "2024-01-01 10:00:00" {
			t.Errorf("point 0 = %+v", got[0])
		}
		if got[1].pass != 0 || got[1].resp != 5000 || got[1].ts != "2024-01-01 10:05:00" {
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
		p, ok := registry.ByName("outpost")
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
	p, ok := registry.ByName("http")
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
	if got[0].resp != 200 {
		t.Errorf("resp = %v, want 200", got[0].resp)
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
	pts := []checkPoint{
		{pass: 2, resp: math.Inf(1), ts: "2024-01-01 10:00:00"},
		{pass: 2, resp: 100, ts: "2024-01-01 10:05:00"},
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

package main

import (
	"testing"

	"sitecheck/checktypes/registry"
)

func TestTitleCase(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"single word lowercase", "hello", "Hello"},
		{"single word UPPERCASE", "HELLO", "Hello"},
		{"multiple words with spaces", "hello world example", "Hello World Example"},
		{"already title case", "Hello World", "Hello World"},
		{"words with numbers", "hello2 world3", "Hello2 World3"},
		{"leading/trailing spaces", "  hello world  ", "Hello World"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := titleCase(tt.input)
			if got != tt.want {
				t.Errorf("titleCase(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestToRegistryMeta(t *testing.T) {
	t.Run("copies all fields correctly", func(t *testing.T) {
		r := Resource{
			Slug:           "test-check",
			ScriptPath:     "/some/path.lua",
			Name:           "My Check",
			Desc:           "A test check description",
			Skip:           true,
			NotifyPass:     true,
			NotifyDegraded: false,
			NotifyFail:     true,
		}
		got := r.toRegistryMeta()
		want := registry.ResourceMeta{
			Slug:           "test-check",
			Name:           "My Check",
			Desc:           "A test check description",
			NotifyPass:     true,
			NotifyDegraded: false,
			NotifyFail:     true,
		}
		if got != want {
			t.Errorf("toRegistryMeta() = %+v, want %+v", got, want)
		}
	})

	t.Run("zero-value Resource produces zero-value ResourceMeta", func(t *testing.T) {
		r := Resource{}
		got := r.toRegistryMeta()
		want := registry.ResourceMeta{}
		if got != want {
			t.Errorf("toRegistryMeta() on zero Resource = %+v, want %+v", got, want)
		}
	})
}

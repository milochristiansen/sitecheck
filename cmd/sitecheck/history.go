package main

import (
	"time"

	"sitecheck/cmd/sitecheck/db"
	"sitecheck/core"
)

// historySource is the seam between site generation and the database. Site
// generation only asks for data it is about to use, and each call returns a
// small, short-lived result that is dropped as soon as the page is written.
type historySource interface {
	// Plugin returns the check plugin for a type, if registered.
	Plugin(checkType string) (core.CheckPlugin, bool)
	// Points returns narrow numeric history for sparklines, charts, and stats.
	// limit <= 0 is unbounded; a positive limit returns the most recent rows.
	Points(checkType, slug, outpostSlug string, since time.Time, limit int) ([]core.CheckPoint, error)
	// EachRecentLight streams light rows newest-first. The callback receives the
	// row's primary key and the typed row.
	EachRecentLight(checkType, slug, outpostSlug string, since time.Time, fn func(id string, row interface{}) error) error
	// LoadFull returns the fully hydrated row for a primary key.
	LoadFull(checkType, id string) (interface{}, error)
}

// dbSource adapts a *db.DB to historySource via the check plugin registry.
type dbSource struct{ db *db.DB }

func (s dbSource) Plugin(checkType string) (core.CheckPlugin, bool) {
	return core.ByName(checkType)
}

func (s dbSource) Points(checkType, slug, outpostSlug string, since time.Time, limit int) ([]core.CheckPoint, error) {
	p, ok := core.ByName(checkType)
	if !ok {
		return nil, nil
	}
	return p.QueryPoints(s.db.DB, slug, outpostSlug, since, limit)
}

func (s dbSource) EachRecentLight(checkType, slug, outpostSlug string, since time.Time, fn func(id string, row interface{}) error) error {
	p, ok := core.ByName(checkType)
	if !ok {
		return nil
	}
	return p.EachRecentLight(s.db.DB, slug, outpostSlug, since, fn)
}

func (s dbSource) LoadFull(checkType, id string) (interface{}, error) {
	p, ok := core.ByName(checkType)
	if !ok {
		return nil, nil
	}
	return p.LoadFull(s.db.DB, id)
}

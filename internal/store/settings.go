package store

import (
	"context"
	"encoding/json/v2"
	"fmt"

	"github.com/frostyard/frostmail/api"
)

// Settings are maild's preferences (ADR-0026, docs/design/organize.md,
// Settings); a setting without a row has its default.
type Settings struct {
	UndoDelay     int    // seconds a sent message waits: 0, 10, 20 or 30
	NotifyScope   string // api.NotifyScope
	NotifySmartID int64  // the smart mailbox of scope smart; 0 otherwise
	FlagNames     []string
}

// FlagColors is how many flag colors there are, and so flag names.
const FlagColors = 7

// DefaultSettings are the settings of a new database.
func DefaultSettings() Settings {
	return Settings{UndoDelay: 10, NotifyScope: string(api.NotifyScopeInbox), FlagNames: make([]string, FlagColors)}
}

// fields maps each key of the settings table to its field of s.
func (s *Settings) fields() map[string]any {
	return map[string]any{
		"undoDelay":     &s.UndoDelay,
		"notifyScope":   &s.NotifyScope,
		"notifySmartId": &s.NotifySmartID,
		"flagNames":     &s.FlagNames,
	}
}

// Settings returns the preferences, defaults where nothing is stored.
func (d *DB) Settings(ctx context.Context) (Settings, error) {
	s := DefaultSettings()
	fields := s.fields()
	rows, err := d.db.QueryContext(ctx, `SELECT key, value_json FROM settings`)
	if err != nil {
		return Settings{}, fmt.Errorf("settings: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return Settings{}, fmt.Errorf("settings: %w", err)
		}
		field, ok := fields[key]
		if !ok {
			continue // a setting a newer maild wrote
		}
		if err := json.Unmarshal([]byte(value), field); err != nil {
			return Settings{}, fmt.Errorf("setting %s: %w", key, err)
		}
	}
	if err := rows.Err(); err != nil {
		return Settings{}, fmt.Errorf("settings: %w", err)
	}
	if len(s.FlagNames) != FlagColors {
		s.FlagNames = append(s.FlagNames, make([]string, FlagColors)...)[:FlagColors]
	}
	return s, nil
}

// SetSettings stores every setting of s and emits api.SettingsChanged. The
// caller has checked the values.
func (t *Tx) SetSettings(ctx context.Context, s Settings) error {
	for key, field := range s.fields() {
		value, err := json.Marshal(field)
		if err != nil {
			return fmt.Errorf("setting %s: %w", key, err)
		}
		if _, err := t.ExecContext(ctx, `INSERT INTO settings (key, value_json) VALUES (?, ?)
			ON CONFLICT (key) DO UPDATE SET value_json = excluded.value_json`, key, string(value)); err != nil {
			return fmt.Errorf("setting %s: %w", key, err)
		}
	}
	return t.Emit(ctx, api.SettingsChanged{})
}

package config

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Mode says which maild a desktop uses: the local one, or the one behind
// lodge (docs/design/desktop.md, On a maild behind lodge).
type Mode string

// The two modes.
const (
	ModeLocal Mode = "local"
	ModeLodge Mode = "lodge"
)

// ReadMode reads the mode from the app's app.json at path: lodge when it
// has a non-empty "lodge" string, else local. A missing file is local;
// unknown keys are ignored; a file that is not valid JSON, or whose "lodge"
// is not a string, is an error that names the file. The app writes this
// file (the Server pane); maild only reads it.
func ReadMode(path string) (Mode, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return ModeLocal, nil
	}
	if err != nil {
		return "", err
	}
	var cfg struct {
		Lodge string `json:"lodge"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	if cfg.Lodge != "" {
		return ModeLodge, nil
	}
	return ModeLocal, nil
}

// AppMode is ReadMode of the app.json that AppConfigFile names.
func AppMode(getenv func(string) string) (Mode, error) {
	path, err := AppConfigFile(getenv)
	if err != nil {
		return "", err
	}
	return ReadMode(path)
}

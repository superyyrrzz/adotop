// Package applog sets up file-only logging. Stdout/stderr would corrupt the TUI.
package applog

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
)

// Dir returns the platform log directory.
func Dir() (string, error) {
	if runtime.GOOS == "windows" {
		appdata := os.Getenv("LOCALAPPDATA")
		if appdata == "" {
			appdata = os.Getenv("APPDATA")
		}
		if appdata == "" {
			return "", errors.New("LOCALAPPDATA/APPDATA not set")
		}
		return filepath.Join(appdata, "adotop", "logs"), nil
	}
	if xdg := os.Getenv("XDG_STATE_HOME"); xdg != "" {
		return filepath.Join(xdg, "adotop"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Logs", "adotop"), nil
	}
	return filepath.Join(home, ".local", "state", "adotop"), nil
}

// Init opens (or creates) the log file and installs a slog default logger writing to it.
// Returns the file path and a close function.
func Init(level slog.Level) (string, func() error, error) {
	dir, err := Dir()
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
		return "", nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}
	p := filepath.Join(dir, "adotop.log")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return "", nil, fmt.Errorf("open log %s: %w", p, err)
	}
	h := slog.NewTextHandler(f, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(h))
	return p, f.Close, nil
}

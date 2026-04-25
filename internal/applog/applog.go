// Package applog sets up file-only logging. Stdout/stderr would corrupt the TUI.
package applog

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// Dir returns the log directory: ~/.adotop/logs on every OS.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".adotop", "logs"), nil
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

package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Org             string            `toml:"org"`
	Project         string            `toml:"project"`
	RefreshInterval Duration          `toml:"refresh_interval"`
	Keybindings     map[string]string `toml:"keybindings"`
}

type Duration struct{ time.Duration }

func (d *Duration) UnmarshalText(text []byte) error {
	v, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	d.Duration = v
	return nil
}

func Default() Config {
	return Config{
		RefreshInterval: Duration{60 * time.Second},
		Keybindings:     map[string]string{},
	}
}

// Path returns the config file path. Same on every OS: ~/.adotop/config.toml.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".adotop", "config.toml"), nil
}

// Load reads the config file. Missing file returns defaults.
func Load() (Config, string, error) {
	p, err := Path()
	if err != nil {
		return Config{}, "", err
	}
	cfg := Default()
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, p, nil
		}
		return Config{}, p, fmt.Errorf("read config: %w", err)
	}
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return Config{}, p, fmt.Errorf("parse config %s: %w", p, err)
	}
	if cfg.RefreshInterval.Duration <= 0 {
		cfg.RefreshInterval = Default().RefreshInterval
	}
	return cfg, p, nil
}

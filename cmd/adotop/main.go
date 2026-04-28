package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/superyyrrzz/adotop/internal/ado"
	"github.com/superyyrrzz/adotop/internal/applog"
	"github.com/superyyrrzz/adotop/internal/config"
	"github.com/superyyrrzz/adotop/internal/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "adotop:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, cfgPath, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logPath, closeLog, err := applog.Init(slog.LevelInfo)
	if err != nil {
		return fmt.Errorf("init log: %w", err)
	}
	defer closeLog()
	slog.Info("adotop starting", "config", cfgPath, "log", logPath, "org", cfg.Org, "project", cfg.Project)

	if cfg.Org == "" {
		// Org isn't fatal — we render a clear message in the shell — but we can't call ADO without it.
		slog.Warn("no org configured; ADO calls will fail until you set 'org' in the config file")
	}

	tokens := ado.NewAzCLITokenProvider()
	client := ado.NewClient(cfg.Org, tokens)

	return ui.Run(cfg, client)
}

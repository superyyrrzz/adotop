package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strconv"
	"strings"

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

	prID, err := parsePRArg(os.Args[1:])
	if err != nil {
		return err
	}

	logPath, closeLog, err := applog.Init(slog.LevelInfo)
	if err != nil {
		return fmt.Errorf("init log: %w", err)
	}
	defer closeLog()
	slog.Info("adotop starting", "config", cfgPath, "log", logPath, "org", cfg.Org, "project", cfg.Project, "initial_pr", prID)

	if cfg.Org == "" {
		// Org isn't fatal — we render a clear message in the shell — but we can't call ADO without it.
		slog.Warn("no org configured; ADO calls will fail until you set 'org' in the config file")
	}

	tokens := ado.NewAzCLITokenProvider()
	client := ado.NewClient(cfg.Org, tokens)

	return ui.Run(cfg, client, prID)
}

// prURLRE matches the trailing `/pullrequest/<id>` (also `/pull/<id>` for
// safety) of an Azure DevOps PR URL. The host/org/project/repo prefix
// varies across dev.azure.com vs visualstudio.com vs on-prem; we only
// care about the ID.
var prURLRE = regexp.MustCompile(`(?i)/pull(?:request)?/(\d+)`)

// parsePRArg interprets the program's positional args. Accepts:
//   - no args (returns 0, no error — normal startup)
//   - one arg: an ADO PR URL or a bare numeric PR ID
//
// Returns 0 and a friendly error for anything else.
func parsePRArg(args []string) (int, error) {
	if len(args) == 0 {
		return 0, nil
	}
	if len(args) > 1 {
		return 0, errors.New("usage: adotop [<pr-url-or-id>]")
	}
	a := strings.TrimSpace(args[0])
	if n, err := strconv.Atoi(a); err == nil && n > 0 {
		return n, nil
	}
	if m := prURLRE.FindStringSubmatch(a); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n, nil
	}
	return 0, fmt.Errorf("can't parse %q as a PR URL or ID", a)
}

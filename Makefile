GO ?= go

.PHONY: build test test-live lint tidy

build:
	$(GO) build -o adotop.exe ./cmd/adotop

test:
	$(GO) test ./...

# Lint runs golangci-lint with our .golangci.yml config. Same gate
# CI uses, so a clean local run means CI will pass too. Install
# golangci-lint via `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`.
lint:
	golangci-lint run ./...

# Live tests hit a real Azure DevOps PR across the canonical pane
# geometry matrix. Requires az login AND a PR ID set in
# ~/.adotop/config.toml (pr_id_for_live_test). Run before declaring a
# "PR view" bug fixed.
test-live:
	$(GO) test -tags=live -run TestLivePRHeaderVisible -v ./internal/ui

tidy:
	$(GO) mod tidy

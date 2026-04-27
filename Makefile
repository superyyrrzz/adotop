GO ?= go

.PHONY: build test test-live tidy

build:
	$(GO) build -o adotop.exe ./cmd/adotop

test:
	$(GO) test ./...

# Live tests hit a real Azure DevOps PR (#1145087) across the canonical
# pane geometry matrix. Requires az login. Run before declaring a "PR
# view" bug fixed.
test-live:
	$(GO) test -tags=live -run TestLivePR1145087HeaderVisible -v ./internal/ui

tidy:
	$(GO) mod tidy

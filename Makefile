.PHONY: help build install install-app uninstall fmt lint lint-version-check gen gen-check test test-fork engine-it e2e mailtest-seed \
	verify check ci ui-check ui-test ui-vitest ui-e2e ui-fmt app-test app-build app-dev app-run mailtest-up mailtest-reset \
	tasks accept task task-verify task-finish clean

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

# golangci-lint's release, read from mise.toml (frostyard/core ADR-0043).
GOLANGCI_LINT_VERSION := $(strip $(shell sed -n 's/^golangci-lint = "\(.*\)"/\1/p' mise.toml))
GO_VERSION := $(strip $(shell sed -n 's/^go \([0-9.]*\)$$/\1/p' go.mod))
# Tracked or new Go files outside third_party (the fork keeps upstream style)
# and outside docs/tasks (given files are checked when copied into place),
# skipping files deleted but not yet staged.
GO_FILES = $(shell git ls-files --cached --others --exclude-standard '*.go' | grep -v -e '^third_party/' -e '^docs/tasks/' | while read -r f; do [ -e "$$f" ] && echo "$$f"; done)

# The app toolchain runs in the nsl machine (docs/adr/0006-development-environment.md).
NSL_MACHINE ?= frostmail
IN_NSL := nsl run -m $(NSL_MACHINE) sh -lc
# The executor model for task cards: an opencode provider/model.
EXECUTOR_MODEL ?= selfie/halogen-qwen3.8-flash-next

help: ## List targets
	@grep -E '^[a-z0-9-]+:.*## ' $(MAKEFILE_LIST) | sed 's/:.*## /\t/' | expand -t 18

build: ## Build maild and mailctl into build/
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o build/ ./cmd/maild ./cmd/mailctl

PREFIX ?= $(HOME)/.local
UNIT_DIR ?= $(HOME)/.config/systemd/user

install: build ## Install maild and mailctl for this user and (re)start maild as a user service
	install -Dm755 build/maild $(PREFIX)/bin/maild
	install -Dm755 build/mailctl $(PREFIX)/bin/mailctl
	install -Dm644 packaging/systemd/maild.service $(UNIT_DIR)/maild.service
	systemctl --user daemon-reload
	systemctl --user enable maild.service
	systemctl --user restart maild.service

install-app: app-build ## Package the app as a local Flatpak and install it for this user (ADR-0013)
	packaging/flatpak/install.sh

uninstall: ## Stop maild and remove it, mailctl and the user service; mail data stays
	-systemctl --user disable --now maild.service
	rm -f $(UNIT_DIR)/maild.service $(PREFIX)/bin/maild $(PREFIX)/bin/mailctl
	systemctl --user daemon-reload

fmt: ## Format Go code
	gofmt -w $(GO_FILES)

gen: ## Regenerate the RPC contract from schema/rpc
	go run ./tools/rpcgen

gen-check: ## Fail if generated RPC files are stale
	go run ./tools/rpcgen -check

lint: lint-version-check ## Run the pinned golangci-lint
	golangci-lint run

lint-version-check:
	@test -n "$(GOLANGCI_LINT_VERSION)" || { echo "mise.toml pins no golangci-lint"; exit 1; }
	@installed="$$(golangci-lint version --short 2>/dev/null)" || { \
		echo "golangci-lint $(GOLANGCI_LINT_VERSION) is required (install with: mise install)"; exit 1; }; \
	if [ "$$installed" != "$(GOLANGCI_LINT_VERSION)" ]; then \
		echo "expected golangci-lint $(GOLANGCI_LINT_VERSION), found $$installed (install with: mise install)"; exit 1; \
	fi

test: ## Run the engine's unit tests
	go test ./...

test-fork: ## Run the go-imap fork's tests
	cd third_party/go-imap && go test ./...

engine-it: ## Run integration tests against the mailtest container (reset first)
	dev/incus/mailtest.sh reset
	FROSTMAIL_IT_HOST=$$(dev/incus/mailtest.sh ip) go test -tags integration -count=1 ./...

# The gate triad (frostyard/core ADR-0043, ADR-0044). verify is credential-free
# and leaves a clean checkout clean; check formats first; ci adds the race
# detector and the app checks, which need the nsl machine.
verify: ## Tidy, generated files, vet, format, lint and unit tests
	go mod tidy -diff
	$(MAKE) --no-print-directory gen-check
	go vet ./...
	test -z "$$(gofmt -l $(GO_FILES))"
	$(MAKE) --no-print-directory lint
	go test ./...
	$(MAKE) --no-print-directory test-fork

check: fmt verify ## Format, then verify (the developer gate)

ci: verify ui-check app-test app-build ## verify plus race tests and the app checks
	go test -race ./...

ui-check: ## Lint (Biome), typecheck and test the app UI (nsl)
	$(IN_NSL) 'cd app && pnpm install --frozen-lockfile --silent && pnpm run check'

ui-test: ui-check ## Alias for ui-check

ui-vitest: ## Run app tests matching F (a path or pattern) in the nsl machine
	$(IN_NSL) 'cd app && pnpm install --frozen-lockfile --silent && pnpm exec vitest run $(F)'

ui-e2e: app-build ## Drive the built app in WebKitGTK against fixture maild data, headless (nsl); F= picks tests
	CGO_ENABLED=0 go build -o build/ ./cmd/maild ./tools/uifixture ./tools/smtpsink
	$(IN_NSL) 'cd app && pnpm install --frozen-lockfile --silent && env -u WAYLAND_DISPLAY GDK_BACKEND=x11 xvfb-run -a -s "-screen 0 1280x800x24" pnpm exec vitest run -c vitest.e2e.config.ts $(F)'

ui-fmt: ## Format the app's sources and organize imports with Biome (nsl)
	$(IN_NSL) 'cd app && pnpm install --frozen-lockfile --silent && pnpm run fmt'

app-build: ## Build the release app binary into build/frostmail-app (nsl)
	$(IN_NSL) 'cd app && pnpm install --frozen-lockfile --silent && pnpm tauri build && mkdir -p ../build && cp "$$CARGO_TARGET_DIR/release/frostmail" ../build/frostmail-app'

app-test: ## Run the app shell's Rust tests (nsl)
	$(IN_NSL) 'cd app/src-tauri && cargo test --release'

app-dev: build ## Run maild and tauri dev in the nsl machine
	$(IN_NSL) 'scripts/dev-app.sh'

app-run: build app-build ## Run maild and the built app in the nsl machine
	$(IN_NSL) 'APP_BINARY=build/frostmail-app scripts/dev-app.sh'

mailtest-up: ## Create the incus test mail server
	dev/incus/mailtest.sh up

mailtest-reset: ## Restore the test mail server's clean snapshot
	dev/incus/mailtest.sh reset

mailtest-seed: ## Load mailgen mail for make e2e (test4: 5,000; test5: 50,000) as snapshot seeded
	dev/incus/mailtest.sh reset
	dev/incus/mailtest.sh seed test4 5000
	dev/incus/mailtest.sh seed test5 50000
	dev/incus/mailtest.sh snapshot seeded

e2e: ## Run the real binaries against the seeded mail server (needs make mailtest-seed)
	dev/incus/mailtest.sh reset seeded
	FROSTMAIL_E2E=1 FROSTMAIL_IT_HOST=$$(dev/incus/mailtest.sh ip) go test -tags integration -count=1 -timeout 30m -v ./tests/e2e/

tasks: ## List task cards
	@go run ./tools/taskrun list

accept: ## Run a task card's acceptance command: make accept T=0001
	@test -n "$(T)" || { echo "usage: make accept T=NNNN"; exit 2; }
	go run ./tools/taskrun accept $(T)

task: ## Run a task card through the executor model: make task T=0001
	@test -n "$(T)" || { echo "usage: make task T=NNNN"; exit 2; }
	go run ./tools/taskrun -model '$(EXECUTOR_MODEL)' run $(T)

task-verify: ## Check a task branch's scope, given files and gates
	go run ./tools/taskrun verify $(T)

task-finish: ## Verify, move the card to done/ and commit the task's work
	go run ./tools/taskrun finish $(T)

clean: ## Remove build output
	rm -rf build dist coverage.out coverage.html

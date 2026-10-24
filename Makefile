.PHONY: help build build-windows install install-app uninstall fmt lint lint-version-check gen gen-check test test-fork engine-it e2e mailtest-seed \
	verify check ci vulncheck web-build lodge-package ui-check ui-test ui-vitest ui-e2e ui-fmt screenshots app-test app-check-windows app-build app-dev app-run mailtest-up mailtest-reset \
	tasks accept task task-verify task-finish clean

VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT)

# govulncheck's release; the Go checksum database verifies the module.
GOVULNCHECK_VERSION := v1.9.0

# golangci-lint's release, read from mise.toml (frostyard/core ADR-0043).
GOLANGCI_LINT_VERSION := $(strip $(shell sed -n 's/^golangci-lint = "\(.*\)"/\1/p' mise.toml))
# The Go release the project builds with: go.mod's toolchain line (its go line is
# only the minimum).
GO_VERSION := $(strip $(shell sed -n 's/^toolchain go\([0-9.]*\)$$/\1/p' go.mod))
# Tracked or new Go files outside third_party (the fork keeps upstream style)
# and outside docs/tasks (given files are checked when copied into place),
# skipping files deleted but not yet staged.
GO_FILES = $(shell git ls-files --cached --others --exclude-standard '*.go' | grep -v -e '^third_party/' -e '^docs/tasks/' | while read -r f; do [ -e "$$f" ] && echo "$$f"; done)

# The app toolchain runs in the nsl machine (docs/adr/0006-development-environment.md).
# CI, whose runner has the toolchain, runs the same targets with IN_NSL='sh -c'.
NSL_MACHINE ?= frostmail
IN_NSL := nsl run -m $(NSL_MACHINE) sh -lc
# The executor for task cards (ADR-0021): codex, with the user's Codex model
# unless EXECUTOR_MODEL names one; or opencode with EXECUTOR_MODEL as its
# provider/model (EXECUTOR=opencode EXECUTOR_MODEL=PROVIDER/MODEL).
EXECUTOR ?= codex
EXECUTOR_MODEL ?=

help: ## List targets
	@grep -E '^[a-z0-9-]+:.*## ' $(MAKEFILE_LIST) | sed 's/:.*## /\t/' | expand -t 18

build: ## Build maild and mailctl into build/
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o build/ ./cmd/maild ./cmd/mailctl

build-windows: ## Cross-compile maild.exe (a GUI program: no console at sign-in) and mailctl.exe into build/windows/
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS) -H=windowsgui' -o build/windows/maild.exe ./cmd/maild
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o build/windows/mailctl.exe ./cmd/mailctl

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

lint: lint-version-check ## Run the pinned golangci-lint, on the Linux and the Windows build
	golangci-lint run
	GOOS=windows golangci-lint run

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
verify: ## Tidy, generated files, vet (Linux and Windows), format, lint and unit tests
	go mod tidy -diff
	$(MAKE) --no-print-directory gen-check
	go vet ./...
	GOOS=windows go vet ./...
	test -z "$$(gofmt -l $(GO_FILES))"
	$(MAKE) --no-print-directory lint
	go test ./...
	$(MAKE) --no-print-directory test-fork

check: fmt verify ## Format, then verify (the developer gate)

ci: verify ui-check app-test app-build ## verify plus race tests and the app checks
	go test -race ./...

vulncheck: ## Report reachable vulnerabilities (govulncheck; needs the network)
	go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

ui-check: ## Lint (Biome), typecheck and test the app UI (nsl)
	$(IN_NSL) 'cd app && pnpm install --frozen-lockfile --silent && pnpm run check'

ui-test: ui-check ## Alias for ui-check

ui-vitest: ## Run app tests matching F (a path or pattern) in the nsl machine
	$(IN_NSL) 'cd app && pnpm install --frozen-lockfile --silent && pnpm exec vitest run $(F)'

ui-e2e: app-build ## Drive the built app in WebKitGTK against fixture maild data, headless (nsl); F= picks tests
	CGO_ENABLED=0 go build -o build/ ./cmd/maild ./tools/uifixture ./tools/smtpsink
	$(IN_NSL) 'cd app && pnpm install --frozen-lockfile --silent && env -u WAYLAND_DISPLAY GDK_BACKEND=x11 xvfb-run -a -s "-screen 0 1280x800x24" pnpm exec vitest run -c vitest.e2e.config.ts $(F)'

screenshots: app-build ## Retake the README's screenshots (docs/images) of the app on made-up mail, headless (nsl)
	CGO_ENABLED=0 go build -o build/ ./cmd/maild ./tools/uifixture ./tools/smtpsink
	$(IN_NSL) 'cd app && pnpm install --frozen-lockfile --silent && env -u WAYLAND_DISPLAY GDK_BACKEND=x11 FROSTMAIL_SCREENSHOTS=docs/images xvfb-run -a -s "-screen 0 1280x800x24" pnpm exec vitest run -c vitest.e2e.config.ts screenshots'

zones: ## Regenerate internal/calendar/zones_gen.go from CLDR's pinned windowsZones.xml (network)
	go run ./tools/zonesgen

ui-fmt: ## Format the app's sources and organize imports with Biome (nsl)
	$(IN_NSL) 'cd app && pnpm install --frozen-lockfile --silent && pnpm run fmt'

app-build: ## Build the release app binary into build/frostmail-app (nsl)
	$(IN_NSL) 'cd app && pnpm install --frozen-lockfile --silent && pnpm tauri build && mkdir -p ../build && cp "$$CARGO_TARGET_DIR/release/frostmail" ../build/frostmail-app'

web-build: ## Build the web app for lodge's /app/mail/ into build/web (nsl; docs/specs/lodge-app.md)
	$(IN_NSL) 'cd app && pnpm install --frozen-lockfile --silent && pnpm exec vite build --mode lodge --base /app/mail/ --outDir ../build/web --emptyOutDir && cp src-tauri/icons/icon.svg ../build/web/icon.svg'

# lodge-package stages maild as lodge's mail app under LODGE_DESTDIR, in
# lodge's install layout (docs/specs/lodge-app.md), with
# packaging/lodge/stage.sh: the manifest with this build's version, the web
# build when make web-build has made one, maild and mailctl, the units and
# the sysusers.d entry. It replaces only mail's own files, so LODGE_DESTDIR
# may be a lodge root other apps share; it refuses an empty one, or /.
LODGE_DESTDIR ?= build/lodge-root
lodge-package: build ## Stage maild, its manifest, units and web build for lodge under LODGE_DESTDIR
	packaging/lodge/stage.sh "$(LODGE_DESTDIR)" "$(VERSION)" build

# The shell's notifications test needs a session bus: dbus-run-session gives
# it one where the machine has it, and it skips elsewhere.
app-test: ## Run the app shell's and the Windows notifier's Rust tests (nsl)
	$(IN_NSL) 'cd app/src-tauri && $$(command -v dbus-run-session >/dev/null && echo dbus-run-session --) cargo test --release && cd ../notify && cargo test --release'

# Windows CI builds the Windows app (ADR-0038); here it is only type-checked,
# without the sidecars its bundle takes.
app-check-windows: ## Type-check the app shell and the notifier for Windows (nsl)
	$(IN_NSL) 'export CARGO_TARGET_DIR="$$CARGO_TARGET_DIR-windows" && cd app/src-tauri && TAURI_CONFIG="{\"bundle\":{\"externalBin\":null}}" cargo check --release --target x86_64-pc-windows-msvc --all-targets && cd ../notify && cargo check --release --target x86_64-pc-windows-msvc --all-targets'

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

task: ## Run a task card through the executor (Codex by default): make task T=0001
	@test -n "$(T)" || { echo "usage: make task T=NNNN"; exit 2; }
	go run ./tools/taskrun -executor '$(EXECUTOR)' -model '$(EXECUTOR_MODEL)' run $(T)

task-verify: ## Check a task branch's scope, given files and gates
	go run ./tools/taskrun verify $(T)

task-finish: ## Verify, move the card to done/ and commit the task's work
	go run ./tools/taskrun finish $(T)

clean: ## Remove build output
	rm -rf build dist coverage.out coverage.html

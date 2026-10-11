// Command maild is the frostmail engine: a per-user daemon that owns mail
// sync, the local store and the RPC socket the app and mailctl use
// (docs/design/overview.md).
package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
	_ "time/tzdata" // calendars need every zone, whatever the sandbox ships (ADR-0018)

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/blob"
	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/devgw"
	"github.com/frostyard/frostmail/internal/engine"
	"github.com/frostyard/frostmail/internal/events"
	"github.com/frostyard/frostmail/internal/httprec"
	"github.com/frostyard/frostmail/internal/mailsync"
	"github.com/frostyard/frostmail/internal/notify"
	"github.com/frostyard/frostmail/internal/oauth"
	"github.com/frostyard/frostmail/internal/pimsync"
	"github.com/frostyard/frostmail/internal/reminders"
	"github.com/frostyard/frostmail/internal/render"
	"github.com/frostyard/frostmail/internal/rpcserver"
	"github.com/frostyard/frostmail/internal/secrets"
	"github.com/frostyard/frostmail/internal/store"
	"github.com/frostyard/frostmail/internal/view"
)

// Set via ldflags at build time.
var (
	version = "dev"
	commit  = "none"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "maild:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("maild", flag.ContinueOnError)
	showVersion := fs.Bool("version", false, "print the version and exit")
	debug := fs.Bool("debug", false, "log at debug level")
	devgwAddr := fs.String("devgw", "", "also serve the API over a WebSocket on this loopback address, for UI development "+
		"(needs FROSTMAIL_DEVGW_TOKEN; docs/design/app.md)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Printf("maild %s (%s)\n", version, commit)
		return nil
	}
	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	paths, err := config.Resolve(os.Getenv)
	if err != nil {
		return err
	}
	for _, dir := range []string{paths.DataDir, paths.Blobs, paths.CacheDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	db, err := store.Open(ctx, paths.DB)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	broker := events.NewBroker(db)
	views := view.NewManager(db, logger)
	db.OnCommit = func(evs []api.EventEnvelope) {
		broker.Publish(evs)
		views.OnCommit(evs)
	}
	// The Secret Service when the desktop has one; FROSTMAIL_SECRETS=file
	// keeps a development or test maild out of the user's keyring.
	sec, err := secrets.Open(ctx, filepath.Join(paths.DataDir, "secrets.json"), logger)
	if err != nil {
		return err
	}
	blobs := blob.New(paths.Blobs)
	parts := &render.PartsCache{Root: filepath.Join(paths.CacheDir, "parts")}
	renderer := &render.Renderer{Parts: parts, Fetcher: render.NewFetcher(parts, nil)}
	tokens := &oauth.Manager{DB: db, Secrets: sec, Log: logger}
	syncCfg := mailsync.Config{
		InsecureSkipVerify: os.Getenv("FROSTMAIL_INSECURE_TLS") == "1",
		Parts:              parts,
		Tokens:             tokens,
	}
	if dir := os.Getenv("MAILD_IMAP_TRACE"); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("trace directory: %w", err)
		}
		logger.Warn("tracing IMAP sessions; the traces hold mail", "dir", dir)
		syncCfg.Trace = traceFiles(dir, logger)
	}
	var davTrace io.Writer
	if dir := os.Getenv("MAILD_DAV_TRACE"); dir != "" {
		f, err := davTraceFile(dir)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		logger.Warn("tracing DAV and Tasks sessions; the trace holds contacts, calendars and tasks", "file", f.Name())
		davTrace = f
	}
	open := openInApp(logger)
	desktop, err := notify.OpenDesktop(ctx, logger, open)
	if err != nil {
		logger.Info("no desktop notifications", "err", err)
		desktop = nil
	} else {
		defer func() { _ = desktop.Close() }()
		syncCfg.Announce = desktop.Announce
	}
	syncer := mailsync.New(db, sec, blobs, logger, syncCfg, broker.Publish)
	pim := pimsync.New(db, sec, logger, pimConfig(syncCfg.InsecureSkipVerify, tokens, davTrace, logger))
	tokens.SignedIn = func(id int64) {
		if err := syncer.Restart(ctx, id); err != nil {
			logger.Warn("restart a signed-in account", "account", id, "err", err)
		}
		if err := pim.Reload(ctx); err != nil {
			logger.Warn("restart a signed-in account's services", "account", id, "err", err)
		}
		pim.Poll(id, true)
	}
	var undo time.Duration // the undoDelay setting, unless FROSTMAIL_UNDO_DELAY overrides it
	if v := os.Getenv("FROSTMAIL_UNDO_DELAY"); v != "" {
		if undo, err = time.ParseDuration(v); err != nil {
			return fmt.Errorf("FROSTMAIL_UNDO_DELAY: %w", err)
		}
	}

	ln, err := rpcserver.Listen(paths.Socket)
	if err != nil {
		return err
	}
	var sched *reminders.Scheduler // the app coming or going checks reminders
	srv := rpcserver.New(rpcserver.Options{Name: "maild " + version, Broker: broker, Logger: logger,
		OnAttended: func(bool) { sched.Kick() }})
	eng := engine.New(engine.Deps{
		DB: db, Secrets: sec, Log: logger, Sync: syncer, Blobs: blobs, Views: views, Render: renderer, UndoDelay: undo,
		OAuth: tokens, PIM: pim,
	})
	sched = newReminders(ctx, db, eng.Calendar(), srv, desktop, open, syncer.FireReminders, logger)
	router, err := api.NewRouter(api.Services{
		RPC: srv, Events: srv, Account: eng.Accounts(), Mailbox: eng.Mailboxes(),
		Message: eng.Messages(), Sync: eng.Sync(), Thread: eng.Threads(), View: eng.Views(),
		Draft: eng.Drafts(), Outbox: eng.Outbox(), Address: eng.Addresses(), Identity: eng.Identities(),
		Oauth: eng.Oauth(), People: eng.People(), Calendar: eng.Calendar(), Tasks: eng.Tasks(),
		Settings: eng.Settings(), Vip: eng.Vips(), Smart: eng.Smart(), Rule: eng.Rules(),
	})
	if err != nil {
		return err
	}
	go views.Run(ctx)
	go sched.Run(ctx)
	// FROSTMAIL_SYNC=off serves the store without connecting to any server,
	// for the README's screenshots, whose accounts have none (make
	// screenshots). Mail already stored still reads.
	if os.Getenv("FROSTMAIL_SYNC") == "off" {
		logger.Warn("sync is off (FROSTMAIL_SYNC=off)")
	} else {
		if err := syncer.Start(ctx); err != nil {
			return err
		}
		if err := pim.Start(ctx); err != nil {
			return err
		}
	}
	defer syncer.Wait()
	defer pim.Wait()
	if *devgwAddr != "" {
		serve := func(nc net.Conn) { srv.ServeConn(ctx, nc, router) }
		go func() {
			if err := devgw.ListenAndServe(ctx, *devgwAddr, os.Getenv("FROSTMAIL_DEVGW_TOKEN"), serve, logger); err != nil {
				logger.Error("dev gateway stopped", "err", err)
			}
		}()
	}
	logger.Info("maild started", "version", version, "socket", paths.Socket, "db", paths.DB)
	err = srv.Serve(ctx, ln, router)
	logger.Info("maild stopped")
	return err
}

// newReminders fires calendar reminders (docs/design/pim.md, Reminders):
// to the app while it is connected, else as desktop notifications, whose
// Snooze (10 minutes) and Dismiss act here and whose click opens the app.
// Each check also fires the messages' Remind Me reminders.
func newReminders(ctx context.Context, db *store.DB, cal api.CalendarService, srv *rpcserver.Server,
	desktop *notify.Desktop, open func(int64), messages func(context.Context, time.Time) error, log *slog.Logger,
) *reminders.Scheduler {
	cfg := reminders.Config{
		DB:       db,
		Messages: messages,
		List: func(ctx context.Context) ([]api.Reminder, error) {
			return cal.Reminders(ctx, &api.CalendarRemindersParams{})
		},
		Attended: srv.Attended,
		Log:      log,
	}
	if desktop != nil {
		cfg.Notifier = desktop
	}
	sched := reminders.New(cfg)
	if desktop != nil {
		desktop.OnReminder(func(id, action string) {
			var err error
			switch action {
			case notify.ActionSnooze:
				until := time.Now().Add(10 * time.Minute).Truncate(time.Minute)
				err = cal.Snooze(ctx, &api.CalendarSnoozeParams{IDs: []string{id}, Until: until})
			case notify.ActionDismiss:
				err = cal.Dismiss(ctx, &api.CalendarDismissParams{IDs: []string{id}})
			default:
				open(0) // the app raises its reminder window
			}
			if err != nil {
				log.Warn("reminder action", "action", action, "err", err)
			}
			sched.Kick()
		})
	}
	return sched
}

// pimConfig configures contacts, calendar and tasks sync: test servers'
// certificates and plain HTTP only with FROSTMAIL_INSECURE_TLS=1, and every
// exchange recorded to trace when it is set (MAILD_DAV_TRACE).
func pimConfig(insecure bool, tokens pimsync.TokenSource, trace io.Writer, logger *slog.Logger) pimsync.Config {
	cfg := pimsync.Config{Tokens: tokens, UserAgent: "Frostmail/" + version, AllowHTTP: insecure}
	var transport http.RoundTripper
	if insecure {
		transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test servers only
		}
	}
	if trace != nil {
		transport = &httprec.Transport{Base: transport, W: trace, OnError: func(err error) {
			logger.Warn("dav trace", "err", err)
		}}
	}
	if transport != nil {
		cfg.HTTP = &http.Client{Timeout: time.Minute, Transport: transport}
	}
	return cfg
}

// davTraceFile creates this run's DAV and Tasks trace in dir
// (MAILD_DAV_TRACE, docs/design/testing.md), readable only by the user.
func davTraceFile(dir string) (*os.File, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("trace directory: %w", err)
	}
	name := fmt.Sprintf("dav-%s.trace", time.Now().UTC().Format("20060102T150405.000000"))
	f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("trace file: %w", err)
	}
	return f, nil
}

// openInApp runs the app on a clicked notification's message (or just the
// app, for a group): frostmail --open-message <id>, or $FROSTMAIL_APP.
func openInApp(logger *slog.Logger) func(messageID int64) {
	app := os.Getenv("FROSTMAIL_APP")
	if app == "" {
		app = "frostmail"
	}
	return func(messageID int64) {
		var args []string
		if messageID != 0 {
			args = []string{"--open-message", strconv.FormatInt(messageID, 10)}
		}
		cmd := exec.Command(app, args...)
		if err := cmd.Start(); err != nil {
			logger.Warn("open the app", "app", app, "err", err)
			return
		}
		go func() { _ = cmd.Wait() }()
	}
}

// traceFiles gives each IMAP connection its own trace file in dir
// (MAILD_IMAP_TRACE, docs/design/testing.md), readable only by the user.
func traceFiles(dir string, logger *slog.Logger) func(accountID int64) io.Writer {
	return func(accountID int64) io.Writer {
		name := fmt.Sprintf("account-%d-%s.trace", accountID, time.Now().UTC().Format("20060102T150405.000000"))
		f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			logger.Warn("trace", "account", accountID, "err", err)
			return nil
		}
		return f
	}
}

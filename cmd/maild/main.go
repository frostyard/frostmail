// Command maild is the frostmail engine: a per-user daemon that owns mail
// sync, the local store and the RPC socket the app and mailctl use
// (docs/design/overview.md).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/frostyard/frostmail/api"
	"github.com/frostyard/frostmail/internal/blob"
	"github.com/frostyard/frostmail/internal/config"
	"github.com/frostyard/frostmail/internal/engine"
	"github.com/frostyard/frostmail/internal/events"
	"github.com/frostyard/frostmail/internal/mailsync"
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
	sec := secrets.NewFile(filepath.Join(paths.DataDir, "secrets.json"))
	blobs := blob.New(paths.Blobs)
	parts := &render.PartsCache{Root: filepath.Join(paths.CacheDir, "parts")}
	renderer := &render.Renderer{Parts: parts, Fetcher: render.NewFetcher(parts, nil)}
	syncer := mailsync.New(db, sec, blobs, logger, mailsync.Config{
		InsecureSkipVerify: os.Getenv("FROSTMAIL_INSECURE_TLS") == "1",
	}, broker.Publish)

	ln, err := rpcserver.Listen(paths.Socket)
	if err != nil {
		return err
	}
	srv := rpcserver.New(rpcserver.Options{Name: "maild " + version, Broker: broker, Logger: logger})
	eng := engine.New(engine.Deps{DB: db, Secrets: sec, Log: logger, Sync: syncer, Blobs: blobs, Views: views, Render: renderer})
	router, err := api.NewRouter(api.Services{
		RPC: srv, Events: srv, Account: eng.Accounts(), Mailbox: eng.Mailboxes(),
		Message: eng.Messages(), Sync: eng.Sync(), Thread: eng.Threads(), View: eng.Views(),
	})
	if err != nil {
		return err
	}
	go views.Run(ctx)
	if err := syncer.Start(ctx); err != nil {
		return err
	}
	defer syncer.Wait()
	logger.Info("maild started", "version", version, "socket", paths.Socket, "db", paths.DB)
	err = srv.Serve(ctx, ln, router)
	logger.Info("maild stopped")
	return err
}

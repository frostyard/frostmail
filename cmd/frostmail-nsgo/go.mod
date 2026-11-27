module github.com/frostyard/frostmail/cmd/frostmail-nsgo

go 1.27.4

require (
	github.com/emersion/go-imap/v2 v2.0.0-beta.8
	github.com/frostyard/frostmail v0.0.0
	github.com/frostyard/frostui/workspaceui v0.5.1
	github.com/frostyard/std v0.5.1
	github.com/nsgo-dev/nsgo v0.3.0
)

require (
	github.com/Microsoft/go-winio v0.6.4 // indirect
	github.com/andybalholm/brotli v1.3.0 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/coder/websocket v1.8.15 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/ebitengine/purego v0.11.3 // indirect
	github.com/emersion/go-message v0.18.2 // indirect
	github.com/emersion/go-sasl v0.0.0-20241020182733-b788ff22d5a6 // indirect
	github.com/emersion/go-smtp v0.26.0 // indirect
	github.com/evanw/esbuild v0.28.2 // indirect
	github.com/frostyard/lodge/kit v0.6.0 // indirect
	github.com/gdamore/encoding v1.0.1 // indirect
	github.com/gdamore/tcell/v3 v3.6.0 // indirect
	github.com/go-sourcemap/sourcemap v2.1.4+incompatible // indirect
	github.com/godbus/dbus/v5 v5.2.4 // indirect
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/google/licenseclassifier/v2 v2.0.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.1 // indirect
	github.com/makiuchi-d/gozxing v0.1.1 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/modelcontextprotocol/go-sdk v1.8.0 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/nsgo-dev/goja v0.1.0-nsgo.2.0.20261122032045-fee2b34506fc // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/sergi/go-diff v1.1.0 // indirect
	github.com/tdewolff/parse/v2 v2.25.0 // indirect
	github.com/teambition/rrule-go v1.8.2 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	github.com/yuin/goldmark v1.8.7 // indirect
	go.yaml.in/yaml/v3 v3.0.6 // indirect
	golang.org/x/crypto v0.59.0 // indirect
	golang.org/x/image v0.48.0 // indirect
	golang.org/x/net v0.63.0 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/term v0.48.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	modernc.org/libc v1.80.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
	modernc.org/sqlite v1.63.0 // indirect
	rsc.io/qr v0.2.0 // indirect
)

replace github.com/frostyard/frostmail => ../..

replace github.com/emersion/go-imap/v2 => ../../third_party/go-imap

tool github.com/nsgo-dev/nsgo/cmd/nsgo

module github.com/coredev-uk/sona-go

go 1.27.3

require (
	github.com/ebitengine/purego v0.11.3
	github.com/egoist/mygo v0.8.3
	github.com/godbus/dbus/v5 v5.2.4
	github.com/mattn/go-sqlite3 v1.14.53
	github.com/pressly/goose/v3 v3.30.0
	github.com/zalando/go-keyring v0.2.8
	golang.org/x/sync v0.25.0
	golang.org/x/sys v0.50.0
	golang.org/x/term v0.48.0
	golang.org/x/text v0.44.0
	golang.org/x/tools v0.52.0
)

require (
	github.com/bitfield/gotestdox v0.2.2 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/dnephin/pflag v1.0.7 // indirect
	github.com/fatih/color v1.18.0 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mfridman/interpolate v0.0.2 // indirect
	github.com/sethvargo/go-retry v0.5.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/image v0.46.0 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	gotest.tools/gotestsum v1.13.0 // indirect
)

tool (
	github.com/egoist/mygo/cmd/mygo
	gotest.tools/gotestsum
)

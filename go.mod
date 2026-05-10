module github.com/KiriKirby/phytozome-go

go 1.25.0

require (
	github.com/alitto/pond/v2 v2.7.1
	github.com/cavaliergopher/grab/v3 v3.0.1
	github.com/cenkalti/backoff/v4 v4.3.0
	github.com/cespare/xxhash/v2 v2.3.0
	github.com/dgraph-io/ristretto/v2 v2.4.0
	github.com/felixge/fgprof v0.9.5
	github.com/gdamore/tcell/v2 v2.7.4
	github.com/goccy/go-json v0.10.6
	github.com/google/go-cmp v0.7.0
	github.com/hashicorp/go-retryablehttp v0.7.8
	github.com/hashicorp/golang-lru/v2 v2.0.7
	github.com/jarcoal/httpmock v1.4.1
	github.com/klauspost/compress v1.18.6
	github.com/phpdave11/gofpdf v1.4.3
	github.com/pkg/profile v1.7.0
	github.com/rivo/tview v0.0.0-20241030223020-e34b54cd4c27
	github.com/rs/zerolog v1.35.1
	github.com/sony/gobreaker/v2 v2.4.0
	github.com/spf13/cobra v1.10.2
	github.com/spf13/viper v1.21.0
	github.com/stretchr/testify v1.11.1
	github.com/valyala/bytebufferpool v1.0.0
	github.com/vmihailenco/msgpack/v5 v5.4.1
	github.com/xuri/excelize/v2 v2.10.1
	go.etcd.io/bbolt v1.4.3
	go.uber.org/automaxprocs v1.6.0
	go.uber.org/goleak v1.3.0
	golang.org/x/perf v0.0.0-20260409210113-8e83ce0f7b1c
	golang.org/x/sync v0.20.0
	golang.org/x/time v0.15.0
)

replace github.com/gdamore/tcell/v2 => github.com/kivattt/tcell-naively-faster/v2 v2.0.1

replace github.com/rivo/tview => github.com/kivattt/tview v1.0.6

require (
	github.com/aclements/go-moremath v0.0.0-20210112150236-f10218a38794 // indirect
	github.com/arl/statsviz v0.8.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/gdamore/encoding v1.0.1 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/gofrs/flock v0.13.0 // indirect
	github.com/google/pprof v0.0.0-20240227163752-401108e1b7e7 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jszwec/csvutil v1.10.0 // indirect
	github.com/karrick/godirwalk v1.17.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.3.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.15 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/richardlehane/mscfb v1.0.6 // indirect
	github.com/richardlehane/msoleps v1.0.6 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/sagikazarmark/locafero v0.11.0 // indirect
	github.com/shirou/gopsutil/v4 v4.26.4 // indirect
	github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/tiendc/go-deepcopy v1.7.2 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/xuri/efp v0.0.1 // indirect
	github.com/xuri/nfp v0.0.2-0.20250530014748-2ddeb826f9a9 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.50.0 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/term v0.42.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

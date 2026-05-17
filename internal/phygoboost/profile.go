package phygoboost

import (
	"context"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime/pprof"
	"strings"

	"github.com/arl/statsviz"
	"github.com/felixge/fgprof"
	"github.com/pkg/profile"
	"github.com/rs/zerolog"
)

var log = zerolog.New(os.Stderr).With().Timestamp().Str("component", "phygoboost").Logger()

func StartDiagnostics(ctx context.Context) func() {
	var stops []func()
	if enabledEnv("PHYTOZOME_GO_PPROF") {
		stops = append(stops, startPProfServer(ctx))
	}
	if mode := strings.ToLower(strings.TrimSpace(os.Getenv("PHYTOZOME_GO_PROFILE"))); mode != "" {
		stops = append(stops, startLocalProfile(mode))
	}
	return func() {
		for i := len(stops) - 1; i >= 0; i-- {
			stops[i]()
		}
	}
}

func startPProfServer(ctx context.Context) func() {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/fgprof", fgprof.Handler().ServeHTTP)
	mux.Handle("/debug/pprof/", http.DefaultServeMux)
	if enabledEnv("PHYTOZOME_GO_STATSVIZ") {
		if err := statsviz.Register(mux); err != nil {
			log.Warn().Err(err).Msg("statsviz unavailable")
		}
	}
	mux.HandleFunc("/debug/heap", func(w http.ResponseWriter, r *http.Request) {
		_ = pprof.Lookup("heap").WriteTo(w, 1)
	})
	addr := strings.TrimSpace(os.Getenv("PHYTOZOME_GO_PPROF_ADDR"))
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Warn().Err(err).Msg("pprof listener unavailable")
		return func() {}
	}
	server := &http.Server{Handler: mux}
	log.Info().Str("addr", listener.Addr().String()).Msg("pprof server listening")
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Warn().Err(err).Msg("pprof server stopped")
		}
	}()
	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()
	return func() {
		_ = server.Shutdown(context.Background())
	}
}

func startLocalProfile(mode string) func() {
	options := []func(*profile.Profile){
		profile.ProfilePath("."),
		profile.NoShutdownHook,
	}
	switch mode {
	case "cpu":
		options = append(options, profile.CPUProfile)
	case "mem", "memory", "heap":
		options = append(options, profile.MemProfile)
	case "block":
		options = append(options, profile.BlockProfile)
	case "mutex":
		options = append(options, profile.MutexProfile)
	default:
		log.Warn().Str("mode", mode).Msg("unknown profile mode")
		return func() {}
	}
	p := profile.Start(options...)
	return func() {
		if p != nil {
			p.Stop()
		}
	}
}

func enabledEnv(name string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func LogRuntimeSummary() {
	state := RuntimeState()
	state.sampleIfStale()
	state.mu.RLock()
	defer state.mu.RUnlock()
	profile := Current()
	log.Debug().
		Uint64("alloc", state.stats.Alloc).
		Uint64("process_rss", state.processRSS).
		Uint64("system_used", state.systemUsed).
		Uint64("system_total", state.systemTotal).
		Uint64("sys", state.stats.Sys).
		Uint32("gc", state.stats.NumGC).
		Int64("active_workers", state.activeWorkers.Load()).
		Int("child_processes", state.ChildProcesses()).
		Int("managed_total", profile.ManagedTotal).
		Str("pressure", fmt.Sprint(state.pressure)).
		Msg("runtime summary")
}

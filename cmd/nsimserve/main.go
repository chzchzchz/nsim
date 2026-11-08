// Command nsimserve runs the simulator continuously and exposes a
// Prometheus-style metrics endpoint plus control endpoints over
// HTTP. It is the long-lived counterpart of the batch nsimrun CLI;
// plant construction is shared via internal/plantinit.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/chzchzchz/nsim/core"
)

// SetLogLevel sets the slog handler level from a flag value.
func SetLogLevel(level string) {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		fmt.Fprintf(os.Stderr, "invalid log level %q; want debug|info|warn|error\n", level)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})))
}

// writeAtomicFile writes data to a temporary file in path's
// directory and renames it over path, so a failed write never
// leaves a partial target file behind. kind names the payload
// in errors so callers do not need a second write path.
func writeAtomicFile(path string, data []byte, kind string) error {
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	tmp, err := os.CreateTemp(dir, ".nsimserve-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp %s file: %w", kind, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s file: %w", kind, err)
	}
	if _, err := tmp.Write([]byte("\n")); err != nil {
		tmp.Close()
		return fmt.Errorf("write %s file: %w", kind, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp %s file: %w", kind, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename %s file: %w", kind, err)
	}
	return nil
}

// saveOnShutdown writes the full core.Plant JSON atomically.
func saveOnShutdown(server *Server, path string) error {
	// Write lock, not read lock: a step or control mutation must
	// not land between the marshal and the rename, or the saved
	// file would describe a plant that never actually existed.
	server.mu.Lock()
	b, err := json.Marshal(server.plant)
	server.mu.Unlock()
	if err != nil {
		return fmt.Errorf("marshal plant JSON: %w", err)
	}
	return writeAtomicFile(path, b, "plant")
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "listen address (loopback default; control endpoints mutate state)")
	webdir := flag.String("webdir", "", "optional directory holding the built web/ UI; served statically at /")
	siteName := flag.String("name", "nsimserve", "Site Name shown in /status and /metrics")
	plantPath := flag.String("plant", "", "plant JSON file (same semantics as nsimrun -plant)")
	scenarioID := flag.String("scenario", "", "starting scenario id from the catalog; mutually exclusive with -plant")
	cmdPath := flag.String("cmd", "", "startup command file: TimeOffset Part.Label Value lines, or web-exported *.json")
	stepsPerSecond := flag.Int("stepsPerSecond", 1000, "steps executed per wall-clock second (clamped to 1e6)")
	timestep := flag.Duration("timestep", core.DefaultTimeStep, "per-step simulated duration")
	paused := flag.Bool("paused", false, "start paused for /sim/step-driven use")
	savePath := flag.String("saveOnShutdown", "", "optional: write full plant JSON to this file on SIGINT/SIGTERM")
	resumePath := flag.String("resume", "", "state file for resume: load an existing file on startup, otherwise start fresh; overwrite on SIGINT/SIGTERM or POST /sim/shutdown")
	logLevel := flag.String("logLevel", "info", "log verbosity: debug|info|warn|error")
	flag.Parse()

	SetLogLevel(*logLevel)

	// NewServer performs the canonical flag validation (steps-per-
	// second and timestep bounds); main only surfaces its error.
	server, err := NewServer(*siteName, *scenarioID, *plantPath, *cmdPath, *paused, *stepsPerSecond, *timestep)
	if err != nil {
		slog.Error("failed to create server", "error", err)
		os.Exit(1)
	}
	if *resumePath != "" {
		snap, err := readResumeState(*resumePath)
		if err == nil {
			if err := server.restoreResumeState(snap); err != nil {
				slog.Error("failed to restore resume state", "path", *resumePath, "error", err)
				os.Exit(1)
			}
			slog.Info("resumed server state", "path", *resumePath, "simTimeMs", snap.SimTimeNs/int64(time.Millisecond))
		} else if !os.IsNotExist(err) {
			slog.Error("failed to load resume state", "path", *resumePath, "error", err)
			os.Exit(1)
		} else {
			slog.Info("starting fresh server state", "path", *resumePath)
		}
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	go server.loop(stop, done)

	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if *webdir != "" {
		httpServer.Handler = webdirHandler(*webdir, server)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.ListenAndServe() }()

	// One teardown path serves signals, /sim/shutdown, and server exit.
	// It saves state exactly once; a returned error is stopped by
	// the caller unless the server already failed, in which case the
	// original error takes precedence.
	teardown := func() error {
		// Shut HTTP down first so in-flight handlers drain before
		// the simulated plant is frozen for the state save.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		shutdownErr := httpServer.Shutdown(ctx)
		cancel()
		if shutdownErr != nil {
			slog.Error("shutdown error", "error", shutdownErr)
		}
		close(stop)
		<-done
		if *savePath != "" {
			if err := saveOnShutdown(server, *savePath); err != nil {
				return fmt.Errorf("failed to save plant: %w", err)
			}
			slog.Info("saved plant state", "path", *savePath)
		}
		if *resumePath != "" {
			if err := saveResumeState(server, *resumePath); err != nil {
				return fmt.Errorf("failed to save resume state: %w", err)
			}
			slog.Info("saved resume state", "path", *resumePath)
		}
		return nil
	}
	exitOnTeardownError := func() {
		if err := teardown(); err != nil {
			slog.Error("failed to persist state", "error", err)
			os.Exit(1)
		}
	}

	select {
	case sigVal := <-sig:
		slog.Info("received signal, shutting down", "signal", sigVal)
		exitOnTeardownError()
		return
	case <-server.shutdown:
		slog.Info("shutdown requested via endpoint")
		exitOnTeardownError()
		return
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			_ = teardown()
			os.Exit(1)
		}
		exitOnTeardownError()
	}
}

// webdirHandler serves static files from dir at / while giving the
// API routes precedence over plain files. The API path test is
// derived from the server's route table so it cannot drift from the
// registered routes.
func webdirHandler(dir string, srv *Server) http.Handler {
	static := http.FileServer(http.Dir(dir))
	api := srv.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAPIPath(srv, r.URL.Path) {
			api.ServeHTTP(w, r)
			return
		}
		static.ServeHTTP(w, r)
	})
}

// isAPIPath reports whether a request path collides with an API
// route (exact or prefix match) so static serving never shadows it.
func isAPIPath(srv *Server, clean string) bool {
	for _, rt := range srv.routes {
		if isAPIPattern(rt.pattern, clean) {
			return true
		}
	}
	return false
}

// isAPIPattern matches a request path against a route pattern:
// exact match, prefix match for subpaths, or wildcard match for
// {key...} entries.
func isAPIPattern(pattern, clean string) bool {
	if prefix, ok := strings.CutSuffix(pattern, "{key...}"); ok {
		return strings.HasPrefix(clean, prefix) && len(clean) > len(prefix)
	}
	return clean == pattern || strings.HasPrefix(clean, pattern+"/")
}

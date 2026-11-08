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

// saveOnShutdown writes the full core.Plant JSON atomically:
// a temporary file in the same directory is renamed over the target
// so a failed write never leaves a partial file behind.
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
	dir := filepath.Dir(path)
	if dir == "" {
		dir = "."
	}
	tmp, err := os.CreateTemp(dir, ".nsimserve-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return fmt.Errorf("write plant file: %w", err)
	}
	if _, err := tmp.Write([]byte("\n")); err != nil {
		tmp.Close()
		return fmt.Errorf("write plant file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename plant file: %w", err)
	}
	return nil
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
	paused := flag.Bool("paused", false, "start paused for /step-driven use")
	savePath := flag.String("saveOnShutdown", "", "optional: write full plant JSON to this file on SIGINT/SIGTERM")
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
		httpServer.Handler = webdirHandler(*webdir, server.Handler())
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	errCh := make(chan error, 1)
	go func() { errCh <- httpServer.ListenAndServe() }()

	// Exactly one branch closes stop; the other branch only observes
	// the loop's exit after a ListenAndServe failure.
	select {
	case sigVal := <-sig:
		slog.Info("received signal, shutting down", "signal", sigVal)
		// Shut HTTP down first so in-flight handlers drain before the
		// simulated plant is frozen for the state save.
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
				slog.Error("failed to save plant", "error", err)
				os.Exit(1)
			}
			slog.Info("saved plant state", "path", *savePath)
		}
		return
	case err := <-errCh:
		close(stop)
		<-done
		if err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}
}

// webdirHandler serves static files from dir at / while giving the
// API routes precedence over plain files.
func webdirHandler(dir string, api http.Handler) http.Handler {
	static := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAPIPath(r.URL.Path) {
			api.ServeHTTP(w, r)
			return
		}
		static.ServeHTTP(w, r)
	})
}

// isAPIPath reports whether a request path collides with an API
// route (prefix match) so static serving never shadows it.
func isAPIPath(clean string) bool {
	for _, prefix := range []string{
		"/livez", "/readyz", "/status", "/scenarios", "/sensors",
		"/controls", "/commands", "/logs", "/pause", "/resume", "/step",
		"/reset", "/timestep", "/speed", "/alarms", "/state", "/metrics",
	} {
		if clean == prefix || strings.HasPrefix(clean, prefix+"/") {
			return true
		}
	}
	return false
}

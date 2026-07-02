package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/yc2utc/zs-logger-bridge/internal/config"
	"github.com/yc2utc/zs-logger-bridge/internal/dedupe"
	"github.com/yc2utc/zs-logger-bridge/internal/jtdx"
	"github.com/yc2utc/zs-logger-bridge/internal/n1mm"
	"github.com/yc2utc/zs-logger-bridge/internal/qso"
	"github.com/yc2utc/zs-logger-bridge/internal/uploader"
)

// ActivityEntry is one line in the UI's live activity feed.
type ActivityEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"` // "info" | "error" | "qso"
	Message string `json:"message"`
}

// App is the Wails-bound backend. Every exported method on *App is callable
// from the frontend as window.go.main.App.<Method>(...).
type App struct {
	ctx context.Context

	mu      sync.Mutex
	cfg     config.Config
	running bool
	stop    chan struct{}
	wg      sync.WaitGroup

	dedupe   *dedupe.Cache
	activity []ActivityEntry
}

func NewApp() *App {
	cfg, _ := config.Load()
	return &App{
		cfg:    cfg,
		dedupe: dedupe.New(2 * time.Minute),
	}
}

// startup is a Wails lifecycle hook (wired in main.go), not frontend-bound.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()

	if cfg.ServerURL != "" && cfg.Token != "" && cfg.LogsheetID != "" {
		if err := a.StartBridge(); err != nil {
			a.log("error", "Auto-start failed: "+err.Error())
		}
	}
}

// shutdown is a Wails lifecycle hook (wired in main.go), not frontend-bound.
func (a *App) shutdown(ctx context.Context) {
	a.StopBridge()
}

// ---------------------------------------------------------------------
// Frontend-bound methods
// ---------------------------------------------------------------------

// GetConfig returns the current settings for the frontend form.
func (a *App) GetConfig() config.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg
}

// SaveConfig persists new settings. Call StartBridge/StopBridge separately
// to apply them to a running bridge -- saving alone doesn't restart it.
func (a *App) SaveConfig(cfg config.Config) error {
	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()
	return cfg.Save()
}

// IsRunning reports whether the UDP listeners are currently active.
func (a *App) IsRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

// Quit stops the bridge and exits the app. Closing the window only hides
// it (see OnBeforeClose in main.go) so the bridge keeps running in the
// background; this is the frontend's actual "Quit" action.
func (a *App) Quit() {
	a.StopBridge()
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
}

// GetActivity returns the recent activity feed (most recent last), for the
// frontend to render on load. Live updates after that arrive via the
// "activity" Wails event.
func (a *App) GetActivity() []ActivityEntry {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]ActivityEntry, len(a.activity))
	copy(out, a.activity)
	return out
}

// StartBridge starts the configured UDP listeners (N1MM, JTDX, or both) and
// begins forwarding QSOs to the logger.
func (a *App) StartBridge() error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return fmt.Errorf("already running")
	}
	cfg := a.cfg
	a.mu.Unlock()

	if cfg.ServerURL == "" || cfg.Token == "" || cfg.LogsheetID == "" {
		return fmt.Errorf("server URL, logsheet ID, and token are required")
	}
	if !cfg.N1MMEnabled && !cfg.JTDXEnabled {
		return fmt.Errorf("enable at least one of N1MM or JTDX")
	}

	stop := make(chan struct{})
	a.mu.Lock()
	a.stop = stop
	a.running = true
	a.mu.Unlock()

	up := &uploader.Client{ServerURL: cfg.ServerURL, LogsheetID: cfg.LogsheetID, Token: cfg.Token}

	if cfg.N1MMEnabled {
		l := &n1mm.Listener{
			Port:       cfg.N1MMPort,
			OnQSO:      func(q qso.QSO) { go a.handleQSO(up, q) },
			OnError:    func(err error) { a.log("error", "N1MM: "+err.Error()) },
			OnRawEvent: func(kind string) { a.log("info", "N1MM: "+kind+" (ignored in v1)") },
		}
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			if err := l.Start(stop); err != nil {
				a.log("error", err.Error())
			}
		}()
		a.log("info", fmt.Sprintf("N1MM listener started on UDP :%d", cfg.N1MMPort))
	}

	if cfg.JTDXEnabled {
		l := &jtdx.Listener{
			Port:    cfg.JTDXPort,
			OnQSO:   func(q qso.QSO) { go a.handleQSO(up, q) },
			OnError: func(err error) { a.log("error", "JTDX: "+err.Error()) },
			// Heartbeat/Status/Decode messages are frequent and not
			// actionable in v1 -- deliberately not logged to keep the
			// activity feed readable.
		}
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			if err := l.Start(stop); err != nil {
				a.log("error", err.Error())
			}
		}()
		a.log("info", fmt.Sprintf("JTDX listener started on UDP :%d", cfg.JTDXPort))
	}

	a.emitStatus()
	return nil
}

// StopBridge stops all listeners. Safe to call when not running.
func (a *App) StopBridge() {
	a.mu.Lock()
	if !a.running {
		a.mu.Unlock()
		return
	}
	close(a.stop)
	a.running = false
	a.mu.Unlock()

	a.wg.Wait()
	a.log("info", "Bridge stopped")
	a.emitStatus()
}

// handleQSO applies the dedupe window and uploads. Run in its own goroutine
// by the caller so a slow/retrying upload never blocks the UDP read loop.
func (a *App) handleQSO(up *uploader.Client, q qso.QSO) {
	if a.dedupe.SeenRecently(q.Key()) {
		a.log("info", fmt.Sprintf("Skipped duplicate: %s %s %s", q.Callsign, q.Mode, q.LoggedAt.Format("15:04:05")))
		return
	}

	a.log("qso", fmt.Sprintf("%s  %s  %.4f MHz (%s) -> uploading...", q.Callsign, q.Mode, q.FrequencyMHz, string(q.Source)))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	result, err := up.Send(ctx, q)
	if err != nil {
		a.log("error", fmt.Sprintf("Upload failed for %s: %v", q.Callsign, err))
		return
	}
	if result.Duplicate {
		a.log("info", fmt.Sprintf("%s already logged (server-side dedupe)", q.Callsign))
		return
	}
	a.log("qso", fmt.Sprintf("Logged %s (log #%d)", q.Callsign, result.LogID))
}

func (a *App) log(level, message string) {
	entry := ActivityEntry{Time: time.Now().Format("15:04:05"), Level: level, Message: message}

	a.mu.Lock()
	a.activity = append(a.activity, entry)
	if len(a.activity) > 200 {
		a.activity = a.activity[len(a.activity)-200:]
	}
	a.mu.Unlock()

	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "activity", entry)
	}
}

func (a *App) emitStatus() {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "status", a.IsRunning())
	}
}

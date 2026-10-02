package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type LogLine struct {
	Time string `json:"time"`
	Text string `json:"text"`
}

type State struct {
	Rotor   RotorState   `json:"rotor"`
	Steppir SteppirState `json:"steppir"`
	AG      AGState      `json:"ag"`
	N1MM    N1MMState    `json:"n1mm"`
}

type App struct {
	ctx    context.Context
	cancel context.CancelFunc

	cfgMu sync.RWMutex
	cfg   Config

	logMu   sync.Mutex
	logs    []LogLine
	logFile *os.File

	emitMu      sync.Mutex
	emitPending bool

	rotor   *Rotator
	steppir *Steppir
	ag      *AntennaGenius
	n1mm    *N1MM
}

func NewApp() *App {
	a := &App{cfg: loadConfig()}
	a.rotor = newRotator(a)
	a.steppir = newSteppir(a)
	a.ag = newAntennaGenius(a)
	a.n1mm = newN1MM(a)
	return a
}

func (a *App) startup(ctx context.Context) {
	a.ctx, a.cancel = context.WithCancel(ctx)
	a.openLogFile()
	if c := a.config(); c.Window.Saved {
		runtime.WindowSetPosition(ctx, c.Window.X, c.Window.Y)
	}
	a.logf("AntennaDeck v%s started. Settings file: %s", Version, configPath())
	go a.rotor.link.run(a.ctx)
	go a.steppir.link.run(a.ctx)
	go a.ag.link.run(a.ctx)
	a.n1mm.start(a.ctx)
}

func (a *App) beforeClose(ctx context.Context) bool {
	x, y := runtime.WindowGetPosition(ctx)
	w, h := runtime.WindowGetSize(ctx)
	a.cfgMu.Lock()
	a.cfg.Window.Saved, a.cfg.Window.X, a.cfg.Window.Y = true, x, y
	if a.cfg.Layout == "vertical" {
		a.cfg.Window.VW, a.cfg.Window.VH = w, h
	} else {
		a.cfg.Window.HW, a.cfg.Window.HH = w, h
	}
	c := a.cfg
	a.cfgMu.Unlock()
	_ = saveConfig(c)
	return false
}

func (a *App) shutdown(ctx context.Context) {
	if a.cancel != nil {
		a.cancel()
	}
	a.n1mm.stop()
}

// openLogFile keeps a copy of the log next to the settings file;
// it is started fresh when it grows past 1 MB.
func (a *App) openLogFile() {
	p := filepath.Join(filepath.Dir(configPath()), "antennadeck.log")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if fi, err := os.Stat(p); err == nil && fi.Size() > 1<<20 {
		_ = os.Remove(p)
	}
	f, err := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	a.logMu.Lock()
	a.logFile = f
	a.logMu.Unlock()
}

func (a *App) config() Config {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return a.cfg
}

func (a *App) mutateConfig(f func(*Config)) {
	a.cfgMu.Lock()
	f(&a.cfg)
	a.cfg.normalize()
	c := a.cfg
	a.cfgMu.Unlock()
	if err := saveConfig(c); err != nil {
		a.logf("Could not save settings: %v", err)
	}
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "config", c)
	}
}

func (a *App) logf(format string, args ...any) {
	line := LogLine{Time: time.Now().Format("15:04:05"), Text: fmt.Sprintf(format, args...)}
	a.logMu.Lock()
	a.logs = append(a.logs, line)
	if len(a.logs) > 500 {
		a.logs = a.logs[len(a.logs)-500:]
	}
	if a.logFile != nil {
		fmt.Fprintf(a.logFile, "%s %s  %s\r\n", time.Now().Format("2006-01-02"), line.Time, line.Text)
	}
	a.logMu.Unlock()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "log", line)
	}
}

// changed schedules a "state" event; bursts of changes are merged.
func (a *App) changed() {
	a.emitMu.Lock()
	if a.emitPending {
		a.emitMu.Unlock()
		return
	}
	a.emitPending = true
	a.emitMu.Unlock()
	time.AfterFunc(50*time.Millisecond, func() {
		a.emitMu.Lock()
		a.emitPending = false
		a.emitMu.Unlock()
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "state", a.GetState())
		}
	})
}

// steppirRadio returns the N1MM radio the SteppIR follows.
func (a *App) steppirRadio() (RadioState, bool) {
	cfg := a.config().Steppir
	n := a.n1mm.snapshot()
	nr := 1
	switch cfg.FollowRadio {
	case "2":
		nr = 2
	case "active":
		if n.ActiveRadio == 2 {
			nr = 2
		}
	case "ag":
		if cfg.AGAntenna > 0 {
			for _, p := range a.ag.snapshot().Ports {
				if p.TxAnt == cfg.AGAntenna || p.RxAnt == cfg.AGAntenna {
					nr = p.ID
					break
				}
			}
		}
	}
	r := n.Radios[nr-1]
	return r, r.Seen
}

func (a *App) steppirTX() bool {
	r, ok := a.steppirRadio()
	return ok && r.TX
}

func (a *App) steppirFollow() {
	cfg := a.config().Steppir
	if !cfg.Enabled {
		return
	}
	if !cfg.Tracking {
		// release a manual change that was held during transmit
		if p := a.steppir.pendingFreq(); p > 0 && !a.steppirTX() {
			if err := a.steppir.SetFreq(p); err != nil {
				a.logf("%v", err)
			}
		}
		return
	}
	r, ok := a.steppirRadio()
	if !ok {
		return
	}
	mhz := r.TXMHz
	if mhz <= 0 {
		mhz = r.MHz
	}
	a.steppir.Follow(mhz, cfg.TrackThresholdKHz)
}

func errStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// ---- Methods called from the user interface ----

func (a *App) GetConfig() Config { return a.config() }

func (a *App) UIReady() { a.logf("User interface ready") }

func (a *App) GetVersion() string { return Version }

func (a *App) GetState() State {
	return State{
		Rotor:   a.rotor.snapshot(),
		Steppir: a.steppir.snapshot(),
		AG:      a.ag.snapshot(),
		N1MM:    a.n1mm.snapshot(),
	}
}

func (a *App) GetLog() []LogLine {
	a.logMu.Lock()
	defer a.logMu.Unlock()
	return append([]LogLine(nil), a.logs...)
}

func (a *App) SaveConfig(c Config) string {
	old := a.config()
	c.Window = old.Window
	c.Layout = old.Layout
	a.mutateConfig(func(dst *Config) { *dst = c })
	a.logf("Settings saved")
	a.rotor.link.restart()
	a.steppir.link.restart()
	a.ag.link.restart()
	a.n1mm.stop()
	a.n1mm.start(a.ctx)
	runtime.WindowSetAlwaysOnTop(a.ctx, a.config().AlwaysOnTop)
	a.changed()
	return ""
}

func (a *App) SetLayout(layout string) {
	w, h := runtime.WindowGetSize(a.ctx)
	a.mutateConfig(func(c *Config) {
		if c.Layout == "vertical" {
			c.Window.VW, c.Window.VH = w, h
		} else {
			c.Window.HW, c.Window.HH = w, h
		}
		c.Layout = layout
	})
	nw, nh := a.config().windowSize()
	runtime.WindowSetSize(a.ctx, nw, nh)
}

func (a *App) SetAlwaysOnTop(on bool) {
	a.mutateConfig(func(c *Config) { c.AlwaysOnTop = on })
	runtime.WindowSetAlwaysOnTop(a.ctx, on)
}

func (a *App) RotorGoTo(az int) string { return errStr(a.rotor.GoTo(az)) }
func (a *App) RotorStop() string       { return errStr(a.rotor.Stop()) }

func (a *App) SteppirBand(name string) string {
	for _, b := range a.config().Steppir.Bands {
		if b.Name == name {
			return errStr(a.steppir.SetFreq(b.MHz))
		}
	}
	return "Unknown band " + name
}

func (a *App) SteppirStep(sign int) string          { return errStr(a.steppir.Step(sign)) }
func (a *App) SteppirDirection(d string) string     { return errStr(a.steppir.SetDirection(d)) }
func (a *App) SteppirRetract() string               { return errStr(a.steppir.Retract()) }
func (a *App) SteppirCalibrate() string             { return errStr(a.steppir.Calibrate()) }
func (a *App) SteppirAutotrack(on bool) string      { return errStr(a.steppir.Autotrack(on)) }
func (a *App) AGSelect(port int, antenna int) string { return errStr(a.ag.Select(port, antenna)) }

func (a *App) SetTracking(on bool) {
	a.mutateConfig(func(c *Config) { c.Steppir.Tracking = on })
	if on {
		a.logf("SteppIR: tracking N1MM frequency")
	} else {
		a.logf("SteppIR: tracking off")
	}
	a.steppirFollow()
}

func (a *App) SetTrackThreshold(khz int) {
	a.mutateConfig(func(c *Config) { c.Steppir.TrackThresholdKHz = khz })
}

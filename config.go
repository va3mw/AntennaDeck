package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type Preset struct {
	Label   string `json:"label"`
	Azimuth int    `json:"azimuth"`
}

type Band struct {
	Name    string  `json:"name"`
	MHz     float64 `json:"mhz"` // frequency the band button tunes to
	Min     float64 `json:"min"`
	Max     float64 `json:"max"`
	Enabled bool    `json:"enabled"`
}

type RotorConfig struct {
	Enabled   bool     `json:"enabled"`
	Host      string   `json:"host"`
	Port      int      `json:"port"`
	Number    int      `json:"number"` // Rotator Genius rotor 1 or 2
	PollMs    int      `json:"pollMs"`
	BeamWidth int      `json:"beamWidth"`
	Presets   []Preset `json:"presets"`
}

type SteppirConfig struct {
	Enabled           bool    `json:"enabled"`
	Host              string  `json:"host"`
	Port              int     `json:"port"`
	PollMs            int     `json:"pollMs"`
	StepKHz           int     `json:"stepKHz"`
	TrackThresholdKHz int     `json:"trackThresholdKHz"`
	FollowRadio       string  `json:"followRadio"` // "1", "2", "active" or "ag"
	AGAntenna         int     `json:"agAntenna"`   // AG antenna port the SteppIR is wired to (for "ag")
	Tracking          bool    `json:"tracking"`
	ShowThreeQuarter  bool    `json:"showThreeQuarter"`
	Bands             []Band  `json:"bands"`
}

type AGConfig struct {
	Enabled  bool   `json:"enabled"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	AuthCode string `json:"authCode"` // remote access code, needed when not on the AG's subnet
}

type N1MMConfig struct {
	RotorEnabled bool   `json:"rotorEnabled"`
	RotorPort    int    `json:"rotorPort"`
	RotorName    string `json:"rotorName"` // blank = accept any rotor name
	RadioEnabled bool   `json:"radioEnabled"`
	RadioPort    int    `json:"radioPort"`
}

type WindowConfig struct {
	Saved bool              `json:"saved"`
	X     int               `json:"x"`
	Y     int               `json:"y"`
	Sizes map[string][2]int `json:"sizes"` // window size per layout key, see layoutKey
	// sizes saved by v1.0 (horizontal / vertical only)
	HW int `json:"hw,omitempty"`
	HH int `json:"hh,omitempty"`
	VW int `json:"vw,omitempty"`
	VH int `json:"vh,omitempty"`
}

type Config struct {
	UIVersion   int           `json:"uiVersion"` // bumped when panel sizes change, resets saved window sizes
	Layout      string        `json:"layout"`    // "horizontal" or "vertical"
	Mini        bool          `json:"mini"`      // compact version of either layout (no compass, small text)
	AlwaysOnTop bool          `json:"alwaysOnTop"`
	Window      WindowConfig  `json:"window"`
	Rotor       RotorConfig   `json:"rotor"`
	Steppir     SteppirConfig `json:"steppir"`
	AG          AGConfig      `json:"ag"`
	N1MM        N1MMConfig    `json:"n1mm"`
	API         APIConfig     `json:"api"`
}

// APIConfig is the HTTP interface used by Stream Deck buttons and other remote tools.
type APIConfig struct {
	Enabled bool   `json:"enabled"`
	Port    int    `json:"port"`
	Key     string `json:"key"` // optional; when set, requests must carry ?key=<key>
}

const uiVersion = 2

func defaultBands() []Band {
	return []Band{
		{"40m", 7.100, 7.000, 7.300, false},
		{"30m", 10.120, 10.100, 10.150, false},
		{"20m", 14.150, 14.000, 14.350, true},
		{"17m", 18.110, 18.068, 18.168, true},
		{"15m", 21.200, 21.000, 21.450, true},
		{"12m", 24.940, 24.890, 24.990, true},
		{"10m", 28.400, 28.000, 29.700, true},
		{"6m", 50.150, 50.000, 54.000, true},
	}
}

func defaultConfig() Config {
	return Config{
		Layout: "horizontal",
		Rotor: RotorConfig{
			Enabled: true, Host: "192.168.1.250", Port: 9006, // 4O3A factory default address
			Number: 1, PollMs: 500, BeamWidth: 60,
			Presets: []Preset{{"EU", 50}, {"SA", 160}, {"US", 220}, {"VK", 300}, {"JA", 330}, {"AS", 355}},
		},
		Steppir: SteppirConfig{
			Enabled: true, Port: 4001, PollMs: 1000, StepKHz: 25, TrackThresholdKHz: 50,
			FollowRadio: "1", Bands: defaultBands(),
		},
		AG:   AGConfig{Enabled: true, Port: 9007},
		N1MM: N1MMConfig{RotorEnabled: true, RotorPort: 12040, RadioEnabled: true, RadioPort: 12060},
		API:  APIConfig{Port: 8090},
	}
}

func (c *Config) normalize() {
	d := defaultConfig()
	if c.Layout != "vertical" {
		c.Layout = "horizontal"
	}
	c.Rotor.Host = strings.TrimSpace(c.Rotor.Host)
	c.Steppir.Host = strings.TrimSpace(c.Steppir.Host)
	c.AG.Host = strings.TrimSpace(c.AG.Host)
	c.AG.AuthCode = strings.TrimSpace(c.AG.AuthCode)
	c.N1MM.RotorName = strings.TrimSpace(c.N1MM.RotorName)
	c.API.Key = strings.TrimSpace(c.API.Key)
	if c.API.Port <= 0 || c.API.Port > 65535 {
		c.API.Port = d.API.Port
	}
	if c.Rotor.Port <= 0 {
		c.Rotor.Port = d.Rotor.Port
	}
	if c.Rotor.Number != 2 {
		c.Rotor.Number = 1
	}
	if c.Rotor.PollMs < 200 {
		c.Rotor.PollMs = d.Rotor.PollMs
	}
	if c.Rotor.BeamWidth <= 0 || c.Rotor.BeamWidth > 180 {
		c.Rotor.BeamWidth = d.Rotor.BeamWidth
	}
	if len(c.Rotor.Presets) == 0 {
		c.Rotor.Presets = d.Rotor.Presets
	}
	if c.Steppir.Port <= 0 {
		c.Steppir.Port = d.Steppir.Port
	}
	if c.Steppir.PollMs < 300 {
		c.Steppir.PollMs = d.Steppir.PollMs
	}
	if c.Steppir.StepKHz <= 0 {
		c.Steppir.StepKHz = d.Steppir.StepKHz
	}
	if c.Steppir.TrackThresholdKHz <= 0 {
		c.Steppir.TrackThresholdKHz = d.Steppir.TrackThresholdKHz
	}
	switch c.Steppir.FollowRadio {
	case "1", "2", "active", "ag":
	default:
		c.Steppir.FollowRadio = "1"
	}
	if len(c.Steppir.Bands) == 0 {
		c.Steppir.Bands = defaultBands()
	}
	if c.AG.Port <= 0 {
		c.AG.Port = d.AG.Port
	}
	if c.N1MM.RotorPort <= 0 {
		c.N1MM.RotorPort = d.N1MM.RotorPort
	}
	if c.N1MM.RadioPort <= 0 {
		c.N1MM.RadioPort = d.N1MM.RadioPort
	}
}

// The four layouts and the window size each opens at until the user resizes it.
var defaultSizes = map[string][2]int{
	"horizontal":      {850, 320},
	"vertical":        {370, 1000},
	"mini-horizontal": {500, 250},
	"mini-vertical":   {270, 585},
}

// layoutKey names the current layout: horizontal, vertical, mini-horizontal or mini-vertical.
func (c Config) layoutKey() string {
	if c.Mini {
		return "mini-" + c.Layout
	}
	return c.Layout
}

// windowSize returns the window size to use for the current layout.
func (c Config) windowSize() (int, int) {
	key := c.layoutKey()
	if s, ok := c.Window.Sizes[key]; ok && s[0] > 0 && s[1] > 0 {
		return s[0], s[1]
	}
	switch {
	case key == "horizontal" && c.Window.HW > 0 && c.Window.HH > 0:
		return c.Window.HW, c.Window.HH
	case key == "vertical" && c.Window.VW > 0 && c.Window.VH > 0:
		return c.Window.VW, c.Window.VH
	}
	s := defaultSizes[key]
	return s[0], s[1]
}

// rememberSize stores the window size for the current layout.
func (c *Config) rememberSize(w, h int) {
	if w <= 0 || h <= 0 {
		return
	}
	if c.Window.Sizes == nil {
		c.Window.Sizes = map[string][2]int{}
	}
	c.Window.Sizes[c.layoutKey()] = [2]int{w, h}
}

// bandFor returns the enabled SteppIR band containing mhz, or "".
func (c Config) bandFor(mhz float64) string {
	for _, b := range c.Steppir.Bands {
		if b.Enabled && mhz >= b.Min && mhz <= b.Max {
			return b.Name
		}
	}
	return ""
}

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "AntennaDeck", "config.json")
}

func loadConfig() Config {
	c := defaultConfig()
	b, err := os.ReadFile(configPath())
	if err != nil {
		// first run after the rename from "BeamController": keep the old settings
		old := filepath.Join(filepath.Dir(filepath.Dir(configPath())), "BeamController", "config.json")
		b, err = os.ReadFile(old)
	}
	if err == nil {
		_ = json.Unmarshal(b, &c)
	}
	if c.UIVersion < uiVersion {
		c.UIVersion = uiVersion
		c.Window.HW, c.Window.HH, c.Window.VW, c.Window.VH = 0, 0, 0, 0
		c.Window.Sizes = nil
	}
	c.normalize()
	return c
}

func saveConfig(c Config) error {
	p := configPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

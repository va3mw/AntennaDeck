package main

import (
	"encoding/json"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// HTTP interface for Stream Deck buttons (or any tool that can fetch a URL).
// Every command is a simple GET, e.g. http://shack-pc:8090/api/rotor/220.
// The page at / lists all commands for this station, ready to copy.
type apiServer struct {
	app *App
	mu  sync.Mutex
	srv *http.Server
}

func newAPIServer(a *App) *apiServer { return &apiServer{app: a} }

func (s *apiServer) start() {
	c := s.app.config().API
	if !c.Enabled {
		return
	}
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(c.Port))
	if err != nil {
		s.app.logf("Stream Deck API: cannot listen on TCP %d (%v)", c.Port, err)
		return
	}
	srv := &http.Server{Handler: http.HandlerFunc(s.serve), ReadHeaderTimeout: 5 * time.Second}
	s.mu.Lock()
	s.srv = srv
	s.mu.Unlock()
	s.app.logf("Stream Deck API: listening on port %d (%s)", c.Port, strings.Join(apiURLs(c.Port), ", "))
	go func() { _ = srv.Serve(ln) }()
}

func (s *apiServer) stop() {
	s.mu.Lock()
	srv := s.srv
	s.srv = nil
	s.mu.Unlock()
	if srv != nil {
		_ = srv.Close()
	}
}

// apiURLs lists http://<ip>:<port>/ for this PC's IPv4 addresses.
func apiURLs(port int) []string {
	var out []string
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ip, ok := a.(*net.IPNet); ok && ip.IP.To4() != nil && !ip.IP.IsLoopback() && !ip.IP.IsLinkLocalUnicast() {
			out = append(out, fmt.Sprintf("http://%s:%d/", ip.IP, port))
		}
	}
	if len(out) == 0 {
		out = append(out, fmt.Sprintf("http://127.0.0.1:%d/", port))
	}
	return out
}

func (s *apiServer) serve(w http.ResponseWriter, r *http.Request) {
	key := s.app.config().API.Key
	if key != "" && r.URL.Query().Get("key") != key && r.Header.Get("X-Api-Key") != key {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "missing or wrong key"})
		return
	}
	path := strings.Trim(r.URL.Path, "/")
	switch {
	case path == "":
		s.help(w)
		return
	case path == "api/status":
		writeJSON(w, http.StatusOK, s.app.GetState())
		return
	case strings.HasPrefix(path, "api/"):
	default:
		http.NotFound(w, r)
		return
	}
	parts := strings.Split(strings.TrimPrefix(path, "api/"), "/")
	err := s.command(parts)
	if err != nil {
		s.app.logf("Stream Deck: /%s from %s failed: %v", path, remoteHost(r), err)
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.app.logf("Stream Deck: /%s from %s", path, remoteHost(r))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func remoteHost(r *http.Request) string {
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return h
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *apiServer) command(p []string) error {
	a := s.app
	arg := func(i int) string {
		if i < len(p) {
			return strings.ToLower(p[i])
		}
		return ""
	}
	switch arg(0) {
	case "rotor":
		return s.rotorCommand(p[1:])
	case "steppir":
		switch arg(1) {
		case "band":
			if r := a.SteppirBand(bandName(a.config(), arg(2))); r != "" {
				return fmt.Errorf("%s", r)
			}
			return nil
		case "dir":
			d := arg(2)
			if d == "n" {
				d = "normal"
			}
			return a.steppir.SetDirection(d)
		case "up":
			return a.steppir.Step(1)
		case "down":
			return a.steppir.Step(-1)
		case "retract":
			return a.steppir.Retract()
		case "calibrate":
			return a.steppir.Calibrate()
		case "autotrack":
			on, err := onOff(arg(2), a.steppir.snapshot().Autotrack)
			if err != nil {
				return err
			}
			return a.steppir.Autotrack(on)
		case "tracking":
			on, err := onOff(arg(2), a.config().Steppir.Tracking)
			if err != nil {
				return err
			}
			a.SetTracking(on)
			return nil
		}
	case "ag":
		port := map[string]int{"a": 1, "1": 1, "b": 2, "2": 2}[arg(1)]
		if port == 0 {
			return fmt.Errorf("port must be A or B")
		}
		ant, err := s.antenna(p[2:])
		if err != nil {
			return err
		}
		return a.ag.Select(port, ant)
	}
	return fmt.Errorf("unknown command")
}

func (s *apiServer) rotorCommand(p []string) error {
	a := s.app
	if len(p) == 0 {
		return fmt.Errorf("missing heading")
	}
	switch strings.ToLower(p[0]) {
	case "stop":
		return a.rotor.Stop()
	case "preset":
		if len(p) > 1 {
			for _, ps := range a.config().Rotor.Presets {
				if strings.EqualFold(ps.Label, p[1]) {
					return a.rotor.GoTo(ps.Azimuth)
				}
			}
		}
		return fmt.Errorf("no such preset")
	case "sp", "lp":
		az := a.n1mm.snapshot().LastAz
		if az < 0 {
			return fmt.Errorf("no heading from N1MM yet")
		}
		if strings.EqualFold(p[0], "lp") {
			az += 180
		}
		return a.rotor.GoTo(az)
	case "cw", "ccw":
		deg := 10
		if len(p) > 1 {
			n, err := strconv.Atoi(p[1])
			if err != nil || n <= 0 || n > 359 {
				return fmt.Errorf("bad step %q", p[1])
			}
			deg = n
		}
		az := a.rotor.snapshot().Azimuth
		if az < 0 {
			return fmt.Errorf("rotor heading not known")
		}
		if strings.EqualFold(p[0], "ccw") {
			deg = -deg
		}
		return a.rotor.GoTo(az + deg)
	}
	az, err := strconv.Atoi(p[0])
	if err != nil || az < 0 || az > 360 {
		return fmt.Errorf("bad heading %q", p[0])
	}
	return a.rotor.GoTo(az)
}

// antenna accepts "/3" or "/name/40M V".
func (s *apiServer) antenna(p []string) (int, error) {
	if len(p) == 1 {
		if n, err := strconv.Atoi(p[0]); err == nil && n > 0 {
			return n, nil
		}
	}
	if len(p) >= 2 && strings.EqualFold(p[0], "name") {
		want := strings.ReplaceAll(strings.Join(p[1:], "/"), "_", " ")
		for _, ant := range s.app.ag.snapshot().Antennas {
			if strings.EqualFold(ant.Name, want) {
				return ant.ID, nil
			}
		}
		return 0, fmt.Errorf("no antenna named %q", want)
	}
	return 0, fmt.Errorf("give an antenna number, or name/<antenna name>")
}

func bandName(c Config, s string) string {
	for _, b := range c.Steppir.Bands {
		if strings.EqualFold(b.Name, s) || strings.EqualFold(strings.TrimSuffix(b.Name, "m"), s) {
			return b.Name
		}
	}
	return s
}

func onOff(s string, current bool) (bool, error) {
	switch s {
	case "on", "1", "true":
		return true, nil
	case "off", "0", "false":
		return false, nil
	case "", "toggle":
		return !current, nil
	}
	return false, fmt.Errorf("use on, off or toggle")
}

// help serves a page listing every command for this station.
func (s *apiServer) help(w http.ResponseWriter) {
	c := s.app.config()
	base := apiURLs(c.API.Port)[0]
	q := ""
	if c.API.Key != "" {
		q = "?key=" + url.QueryEscape(c.API.Key)
	}
	type row struct{ path, what string }
	var rows []row
	add := func(path, what string) { rows = append(rows, row{path, what}) }

	add("api/rotor/220", "Turn to 220° (any heading 0-359)")
	for _, p := range c.Rotor.Presets {
		add("api/rotor/preset/"+p.Label, fmt.Sprintf("Preset %s (%d°)", p.Label, p.Azimuth))
	}
	add("api/rotor/cw/10", "Turn 10° clockwise from the current heading")
	add("api/rotor/ccw/10", "Turn 10° counter-clockwise")
	add("api/rotor/sp", "Short path to the last N1MM heading")
	add("api/rotor/lp", "Long path to the last N1MM heading")
	add("api/rotor/stop", "Stop the rotor")
	for _, b := range c.Steppir.Bands {
		if b.Enabled {
			add("api/steppir/band/"+b.Name, fmt.Sprintf("SteppIR to %s (%.3f MHz)", b.Name, b.MHz))
		}
	}
	add("api/steppir/dir/normal", "SteppIR normal direction")
	add("api/steppir/dir/180", "SteppIR 180°")
	add("api/steppir/dir/bi", "SteppIR bi-directional")
	add("api/steppir/up", fmt.Sprintf("SteppIR up %d kHz", c.Steppir.StepKHz))
	add("api/steppir/down", fmt.Sprintf("SteppIR down %d kHz", c.Steppir.StepKHz))
	add("api/steppir/tracking/toggle", "N1MM frequency tracking on/off (also /on, /off)")
	add("api/steppir/autotrack/toggle", "Controller autotrack on/off (also /on, /off)")
	add("api/steppir/retract", "Retract elements (no confirmation)")
	add("api/steppir/calibrate", "Calibrate elements (no confirmation)")
	ants := s.app.ag.snapshot().Antennas
	sort.Slice(ants, func(i, j int) bool { return ants[i].ID < ants[j].ID })
	for _, port := range []string{"A", "B"} {
		for _, ant := range ants {
			add(fmt.Sprintf("api/ag/%s/%d", port, ant.ID), fmt.Sprintf("Antenna Genius radio %s → %s", port, ant.Name))
		}
	}
	add("api/status", "Current state of everything, as JSON")

	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>AntennaDeck commands</title>
<style>body{font:14px Segoe UI,sans-serif;background:#141210;color:#ece6dd;margin:24px}
h1{color:#e8762b;font-size:20px}table{border-collapse:collapse}td{padding:4px 12px;border-bottom:1px solid #3b342d}
a{color:#22c27a;font-family:Consolas,monospace;text-decoration:none}a:hover{text-decoration:underline}
p{color:#988f84;max-width:760px}</style></head><body>`)
	fmt.Fprintf(&b, "<h1>AntennaDeck v%s: Stream Deck commands</h1>", Version)
	b.WriteString("<p>Each line is a URL for a Stream Deck button. Use a plugin that sends an HTTP GET request. " +
		"Clicking a link here runs the command for real, so it will move the antenna.</p><table>")
	for _, r := range rows {
		u := base + r.path + q
		fmt.Fprintf(&b, `<tr><td><a href="%s">%s</a></td><td>%s</td></tr>`, html.EscapeString(u), html.EscapeString(u), html.EscapeString(r.what))
	}
	b.WriteString("</table></body></html>")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

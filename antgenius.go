package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 4O3A Antenna Genius TCP/IP API (default port 9007).
// Commands are "C<seq>|<command>\n", replies "R<seq>|<hex code>|<message>",
// asynchronous status "S0|<message>". See github.com/4o3a/genius-api-docs/wiki.
type AGAntenna struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	TX   uint16 `json:"tx"` // band masks, bit n = band n
	RX   uint16 `json:"rx"`
}

type AGPort struct {
	ID       int    `json:"id"` // 1 = A, 2 = B
	Band     int    `json:"band"`
	BandName string `json:"bandName"`
	RxAnt    int    `json:"rxant"`
	TxAnt    int    `json:"txant"`
	TX       bool   `json:"tx"`
	Inhibit  bool   `json:"inhibit"`
	Source   string `json:"source"`
}

type AGState struct {
	Connected bool        `json:"connected"`
	Version   string      `json:"version"`
	AuthMsg   string      `json:"authMsg"` // remote-access problem to show, "" if none
	Antennas  []AGAntenna `json:"antennas"`
	Ports     []AGPort    `json:"ports"`
}

type AntennaGenius struct {
	app       *App
	link      *link
	mu        sync.Mutex
	connected bool
	version   string
	ants      map[int]AGAntenna
	bands     map[int]string
	ports     [2]AGPort
	seq       int
	pending   map[int]string
	line      []byte
	inited    bool
	authWait  bool // sent the remote access code, waiting for the reply
	authMsg   string
	codeNagged bool // "code required" already logged (only touched by the reader goroutine)
	connTime  time.Time
}

func newAntennaGenius(a *App) *AntennaGenius {
	g := &AntennaGenius{app: a}
	g.reset()
	l := newLink("Antenna Genius", a)
	l.addr = func() (string, bool) {
		c := a.config().AG
		return hostPort(c.Host, c.Port), c.Enabled && c.Host != ""
	}
	l.interval = func() time.Duration { return 2 * time.Second }
	l.staleAfter = 10 * time.Second
	l.onConnect = func() {
		g.mu.Lock()
		g.reset()
		g.connected = true
		g.connTime = time.Now()
		g.mu.Unlock()
		a.changed()
	}
	l.onDisconnect = func() {
		g.mu.Lock()
		g.connected = false
		g.mu.Unlock()
		a.changed()
	}
	l.onTick = func() {
		g.mu.Lock()
		inited, authWait, since := g.inited, g.authWait, time.Since(g.connTime)
		g.mu.Unlock()
		if authWait {
			return
		}
		if !inited && since > 1500*time.Millisecond {
			g.initialise() // no version banner seen; start anyway
		} else if inited {
			g.cmd("ping")
		}
	}
	l.onData = g.onData
	g.link = l
	return g
}

// reset clears device state; caller holds mu (or is the constructor).
func (g *AntennaGenius) reset() {
	g.ants = map[int]AGAntenna{}
	g.bands = map[int]string{}
	g.ports = [2]AGPort{{ID: 1}, {ID: 2}}
	g.pending = map[int]string{}
	g.line = nil
	g.inited = false
	g.authWait = false
	g.authMsg = ""
	g.version = ""
}

func (g *AntennaGenius) snapshot() AGState {
	g.mu.Lock()
	defer g.mu.Unlock()
	st := AGState{Connected: g.connected, Version: g.version, AuthMsg: g.authMsg}
	for _, a := range g.ants {
		st.Antennas = append(st.Antennas, a)
	}
	sort.Slice(st.Antennas, func(i, j int) bool { return st.Antennas[i].ID < st.Antennas[j].ID })
	for _, p := range g.ports {
		p.BandName = g.bands[p.Band]
		st.Ports = append(st.Ports, p)
	}
	return st
}

func (g *AntennaGenius) cmd(c string) error {
	g.mu.Lock()
	g.seq++
	if g.seq > 255 {
		g.seq = 1
	}
	seq := g.seq
	g.pending[seq] = c
	g.mu.Unlock()
	// Firmware 4.1.x ignores commands ending in a bare CR (as the API docs
	// show); it needs LF.
	if err := g.link.send([]byte(fmt.Sprintf("C%d|%s\n", seq, c))); err != nil {
		return fmt.Errorf("Antenna Genius: %w", err)
	}
	return nil
}

func (g *AntennaGenius) initialise() {
	g.mu.Lock()
	if g.inited {
		g.mu.Unlock()
		return
	}
	g.inited = true
	g.mu.Unlock()
	for _, c := range []string{"band list", "antenna list", "sub port all", "sub antenna", "port get 1", "port get 2"} {
		_ = g.cmd(c)
	}
}

func (g *AntennaGenius) onData(b []byte) {
	g.mu.Lock()
	g.line = append(g.line, b...)
	var lines []string
	for {
		i := strings.IndexAny(string(g.line), "\r\n")
		if i < 0 {
			break
		}
		if i > 0 {
			lines = append(lines, string(g.line[:i]))
		}
		g.line = g.line[i+1:]
	}
	if len(g.line) > 8192 {
		g.line = nil
	}
	g.mu.Unlock()
	for _, ln := range lines {
		g.handleLine(strings.TrimSpace(ln))
	}
}

func (g *AntennaGenius) handleLine(ln string) {
	if ln == "" {
		return
	}
	switch ln[0] {
	case 'V':
		g.mu.Lock()
		g.version = strings.Fields(ln)[0][1:]
		g.mu.Unlock()
		g.app.logf("Antenna Genius: firmware %s", ln[1:])
		g.app.changed()
		// "V4.1.2 AG AUTH" = connection is from outside the AG's subnet
		if strings.HasSuffix(ln, " AUTH") {
			code := g.app.config().AG.AuthCode
			if code == "" {
				if !g.codeNagged {
					g.codeNagged = true
					g.app.logf("Antenna Genius: remote access code required - enter it in Settings")
				}
				g.mu.Lock()
				g.authWait = true // nothing more can be done until a code is saved
				g.authMsg = "Remote access code needed (Settings)"
				g.mu.Unlock()
				g.app.changed()
				return
			}
			g.mu.Lock()
			g.authWait = true
			g.authMsg = "Sending access code…"
			g.mu.Unlock()
			_ = g.cmd("auth code=" + code)
			return
		}
		g.initialise()
	case 'R':
		parts := strings.SplitN(ln[1:], "|", 3)
		if len(parts) < 2 {
			return
		}
		seq, _ := strconv.Atoi(parts[0])
		g.mu.Lock()
		cmd := g.pending[seq]
		if len(parts) == 3 && parts[2] == "" || cmd == "ping" {
			delete(g.pending, seq) // final (empty) reply of a command
		}
		g.mu.Unlock()
		code, _ := strconv.ParseUint(parts[1], 16, 32)
		if strings.HasPrefix(cmd, "auth ") {
			g.mu.Lock()
			g.authWait = code != 0
			g.authMsg = ""
			if code != 0 {
				g.authMsg = "Access code rejected (Settings)"
			}
			g.mu.Unlock()
			g.app.changed()
			if code != 0 {
				g.app.logf("Antenna Genius: remote access code was rejected - check it in Settings")
				return
			}
			g.app.logf("Antenna Genius: remote access code accepted")
			g.initialise()
			return
		}
		if code != 0 {
			if cmd != "sub antenna" { // not supported by all firmware
				g.app.logf("Antenna Genius: '%s' failed (code %s)", cmd, parts[1])
			}
			return
		}
		if len(parts) == 3 {
			g.handleMessage(parts[2])
		}
	case 'S':
		if parts := strings.SplitN(ln, "|", 2); len(parts) == 2 {
			g.handleMessage(parts[1])
		}
	}
}

func agFields(fields []string) map[string]string {
	kv := map[string]string{}
	for _, f := range fields {
		if k, v, ok := strings.Cut(f, "="); ok {
			kv[k] = v
		}
	}
	return kv
}

func (g *AntennaGenius) handleMessage(msg string) {
	f := strings.Fields(msg)
	if len(f) < 2 {
		return
	}
	if f[0] == "antenna" && f[1] == "reload" {
		_ = g.cmd("antenna list")
		return
	}
	id, err := strconv.Atoi(f[1])
	if err != nil {
		return
	}
	kv := agFields(f[2:])
	portChanged := false
	g.mu.Lock()
	switch f[0] {
	case "antenna":
		tx, _ := strconv.ParseUint(kv["tx"], 16, 16)
		rx, _ := strconv.ParseUint(kv["rx"], 16, 16)
		name := strings.ReplaceAll(kv["name"], "_", " ")
		if name == "" {
			name = fmt.Sprintf("Antenna %d", id)
		}
		g.ants[id] = AGAntenna{ID: id, Name: name, TX: uint16(tx), RX: uint16(rx)}
	case "band":
		g.bands[id] = strings.ReplaceAll(kv["name"], "_", " ")
	case "port":
		if id < 1 || id > 2 {
			break
		}
		p := &g.ports[id-1]
		before := *p
		atoi := func(k string, dst *int) {
			if v, ok := kv[k]; ok {
				*dst, _ = strconv.Atoi(v)
			}
		}
		atoi("band", &p.Band)
		atoi("rxant", &p.RxAnt)
		atoi("txant", &p.TxAnt)
		if v, ok := kv["tx"]; ok {
			p.TX = v == "1"
		}
		if v, ok := kv["inhibit"]; ok {
			p.Inhibit = v == "1"
		}
		if v, ok := kv["source"]; ok {
			p.Source = v
		}
		portChanged = before.RxAnt != p.RxAnt || before.TxAnt != p.TxAnt
	default:
		g.mu.Unlock()
		return
	}
	g.mu.Unlock()
	g.app.changed()
	if portChanged {
		g.app.steppirFollow()
	}
}

// Select puts antenna ant on radio port (1 = A, 2 = B). The AG keeps
// automatic band selection; a manual pick holds until the next band change.
func (g *AntennaGenius) Select(port, ant int) error {
	if port < 1 || port > 2 {
		return fmt.Errorf("Antenna Genius: bad port %d", port)
	}
	if err := g.cmd(fmt.Sprintf("port set %d rxant=%d", port, ant)); err != nil {
		return err
	}
	g.app.logf("Antenna Genius: port %s -> antenna %d", map[int]string{1: "A", 2: "B"}[port], ant)
	return nil
}

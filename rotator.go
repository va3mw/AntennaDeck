package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 4O3A Rotator Genius over raw TCP (default port 9006).
//
//	|h        status request
//	|A<n><az> turn rotor n to azimuth (e.g. |A1173)
//	|S        stop
//
// Status reply: "|h2" + 0x00, then one 34-byte block per rotor:
//
//	0-2 azimuth (999 = no sensor)   10 moving (0 stop, 1 CW, 2 CCW)
//	15-17 target (999 = none)       21 0x00, 22-33 rotor name
//	e.g. "173359  0A0  10999999\x00Steppir     "
type RotorState struct {
	Connected bool   `json:"connected"`
	Azimuth   int    `json:"azimuth"` // -1 = unknown
	Target    int    `json:"target"`  // -1 = none
	Moving    int    `json:"moving"`  // 0 stopped, 1 CW, 2 CCW
	Name      string `json:"name"`
}

type Rotator struct {
	app  *App
	link *link
	mu   sync.Mutex
	st       RotorState
	buf      []byte
	reported bool // first heading after connect has been logged
}

const rgBlockLen = 34

func newRotator(a *App) *Rotator {
	r := &Rotator{app: a, st: RotorState{Azimuth: -1, Target: -1}}
	l := newLink("Rotator Genius", a)
	l.addr = func() (string, bool) {
		c := a.config().Rotor
		return hostPort(c.Host, c.Port), c.Enabled && c.Host != ""
	}
	l.interval = func() time.Duration { return time.Duration(a.config().Rotor.PollMs) * time.Millisecond }
	l.staleAfter = 8 * time.Second
	l.onConnect = func() {
		r.mu.Lock()
		r.buf = nil
		r.reported = false
		r.st.Connected = true
		r.mu.Unlock()
		a.changed()
		_ = l.send([]byte("|h"))
	}
	l.onDisconnect = func() {
		r.mu.Lock()
		r.st = RotorState{Azimuth: -1, Target: -1, Name: r.st.Name}
		r.mu.Unlock()
		a.changed()
	}
	l.onTick = func() { _ = l.send([]byte("|h")) }
	l.onData = r.onData
	r.link = l
	return r
}

func (r *Rotator) snapshot() RotorState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.st
}

func (r *Rotator) onData(b []byte) {
	rotorNo := r.app.config().Rotor.Number
	r.mu.Lock()
	r.buf = append(r.buf, b...)
	if len(r.buf) > 4096 {
		r.buf = r.buf[len(r.buf)-1024:]
	}
	st, cut, ok := parseRG(string(r.buf), rotorNo)
	changed := false
	if ok {
		r.buf = append([]byte(nil), r.buf[cut:]...)
		next := r.st
		next.Azimuth, next.Target, next.Moving, next.Name = st.Azimuth, st.Target, st.Moving, st.Name
		if next.Moving == 0 {
			next.Target = -1
		}
		changed = next != r.st
		r.st = next
	}
	first := ok && !r.reported
	if first {
		r.reported = true
	}
	r.mu.Unlock()
	if first {
		r.app.logf("Rotator Genius: rotor %d '%s' at %d°", rotorNo, st.Name, st.Azimuth)
	}
	if changed {
		r.app.changed()
	}
}

// parseRG finds the newest complete status reply in s. It returns the parsed
// rotor block and the index just past the "|h" marker that was used.
func parseRG(s string, rotor int) (RotorState, int, bool) {
	end := len(s)
	for {
		i := strings.LastIndex(s[:end], "|h")
		if i < 0 {
			return RotorState{}, 0, false
		}
		// The header length after "|h" is found by checking where the
		// first block's fixed fields line up.
		for k := 2; k <= 5; k++ {
			start := i + k
			if len(s) < start+rgBlockLen {
				break
			}
			if !validRGBlock(s[start : start+rgBlockLen]) {
				continue
			}
			b := start + (rotor-1)*rgBlockLen
			if len(s) < b+rgBlockLen || !validRGBlock(s[b:b+rgBlockLen]) {
				break
			}
			blk := s[b : b+rgBlockLen]
			return RotorState{
				Azimuth: rgNum(blk[0:3]),
				Moving:  int(blk[10] - '0'),
				Target:  rgNum(blk[15:18]),
				Name:    strings.Trim(blk[21:rgBlockLen], " \x00"),
			}, i + 2, true
		}
		end = i
	}
}

func validRGBlock(b string) bool {
	c := b[9]
	return ((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) && b[10] >= '0' && b[10] <= '2'
}

func rgNum(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n >= 999 || n < 0 {
		return -1
	}
	return n
}

func (r *Rotator) GoTo(az int) error {
	az = ((az % 360) + 360) % 360
	n := r.app.config().Rotor.Number
	if err := r.link.send([]byte(fmt.Sprintf("|A%d%d", n, az))); err != nil {
		return fmt.Errorf("Rotator: %w", err)
	}
	r.app.logf("Rotator: turn to %d°", az)
	r.mu.Lock()
	r.st.Target = az
	r.mu.Unlock()
	r.app.changed()
	return nil
}

func (r *Rotator) Stop() error {
	if err := r.link.send([]byte("|S")); err != nil {
		return fmt.Errorf("Rotator: %w", err)
	}
	r.app.logf("Rotator: stop")
	return nil
}

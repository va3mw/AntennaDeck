package main

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"
)

// SteppIR SDA100 transceiver-interface protocol, carried over a TCP-to-RS232
// converter. Every command is one 11-byte packet:
//
//	'@' 'A' 0x00 F2 F1 F0 0x00 dir cmd 0x00 CR
//
// F2..F0 = frequency in 10 Hz units, big-endian.
// dir: 0x00 normal, 0x40 180°, 0x80 bi-directional, 0x20 3/4 wave.
// cmd: 0x00 set freq/dir, 'R' autotrack on, 'U' autotrack off, 'S' home (retract), 'V' calibrate.
//
// "?A" CR requests status; the reply is '@' 'A' 0x00 F2 F1 F0 motors dir vh vl CR.
const (
	stDirNormal = 0x00
	stDir180    = 0x40
	stDirBi     = 0x80
	stDir34     = 0x20

	stCmdSet          = 0x00
	stCmdAutotrackOn  = 'R'
	stCmdAutotrackOff = 'U'
	stCmdHome         = 'S'
	stCmdCalibrate    = 'V'
)

type SteppirState struct {
	Connected bool    `json:"connected"`
	MHz       float64 `json:"mhz"`
	Direction string  `json:"direction"` // normal, 180, bi, 34
	Moving    bool    `json:"moving"`
	Autotrack bool    `json:"autotrack"`
	Version   string  `json:"version"`
	Band      string  `json:"band"`
	Holding   bool    `json:"holding"` // frequency change waiting for TX to end
}

type Steppir struct {
	app      *App
	link     *link
	mu       sync.Mutex
	st       SteppirState
	dirByte  byte
	buf      []byte
	lastSent float64
	pending  float64
}

var errTransmitting = errors.New("SteppIR: radio is transmitting - elements not moved")

func newSteppir(a *App) *Steppir {
	s := &Steppir{app: a, st: SteppirState{Direction: "normal"}}
	l := newLink("SteppIR", a)
	l.addr = func() (string, bool) {
		c := a.config().Steppir
		return hostPort(c.Host, c.Port), c.Enabled && c.Host != ""
	}
	l.interval = func() time.Duration { return time.Duration(a.config().Steppir.PollMs) * time.Millisecond }
	l.staleAfter = 15 * time.Second
	l.onConnect = func() {
		s.mu.Lock()
		s.buf = nil
		s.st.Connected = true
		s.mu.Unlock()
		a.changed()
		_ = l.send([]byte("?A\r"))
	}
	l.onDisconnect = func() {
		s.mu.Lock()
		s.st.Connected = false
		s.st.Moving = false
		s.mu.Unlock()
		a.changed()
	}
	l.onTick = func() { _ = l.send([]byte("?A\r")) }
	l.onData = s.onData
	s.link = l
	return s
}

func (s *Steppir) snapshot() SteppirState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.st
}

func (s *Steppir) onData(b []byte) {
	cfg := s.app.config()
	s.mu.Lock()
	s.buf = append(s.buf, b...)
	prev := s.st
	for {
		i := bytes.Index(s.buf, []byte("@A"))
		if i < 0 {
			if n := len(s.buf); n > 0 && s.buf[n-1] == '@' {
				s.buf = s.buf[n-1:]
			} else {
				s.buf = s.buf[:0]
			}
			break
		}
		if len(s.buf) < i+11 {
			s.buf = s.buf[i:]
			break
		}
		f := s.buf[i : i+11]
		if f[10] != 0x0D {
			s.buf = s.buf[i+1:]
			continue
		}
		freq := uint32(f[2])<<24 | uint32(f[3])<<16 | uint32(f[4])<<8 | uint32(f[5])
		s.st.MHz = float64(freq) / 100000
		s.st.Moving = f[6] != 0
		dir := f[7]
		s.dirByte = dir & 0xE0
		switch {
		case dir&stDirBi != 0:
			s.st.Direction = "bi"
			s.dirByte = stDirBi
		case dir&stDir180 != 0:
			s.st.Direction = "180"
			s.dirByte = stDir180
		case dir&stDir34 != 0:
			s.st.Direction = "34"
			s.dirByte = stDir34
		default:
			s.st.Direction = "normal"
			s.dirByte = stDirNormal
		}
		s.st.Autotrack = dir&0x04 != 0
		if f[8] >= 0x20 && f[8] < 0x7F && f[9] >= 0x20 && f[9] < 0x7F {
			s.st.Version = string(f[8:10])
		} else {
			s.st.Version = fmt.Sprintf("%02X%02X", f[8], f[9])
		}
		s.st.Band = cfg.bandFor(s.st.MHz)
		s.buf = s.buf[i+11:]
	}
	changed := s.st != prev
	s.mu.Unlock()
	if changed {
		s.app.changed()
	}
}

func (s *Steppir) packet(mhz float64, dir, cmd byte) error {
	f := uint32(math.Round(mhz * 100000))
	pkt := []byte{'@', 'A', 0, byte(f >> 16), byte(f >> 8), byte(f), 0, dir, cmd, 0, 0x0D}
	if err := s.link.send(pkt); err != nil {
		return fmt.Errorf("SteppIR: %w", err)
	}
	return nil
}

// current returns the frequency and direction to resend with a command.
func (s *Steppir) current() (float64, byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	mhz := s.st.MHz
	if mhz <= 0 {
		mhz = s.lastSent
	}
	if mhz <= 0 {
		return 0, 0, errors.New("SteppIR: frequency not known yet - pick a band first")
	}
	return mhz, s.dirByte, nil
}

// SetFreq tunes the elements. If the controlling radio is transmitting the
// change is held and sent when transmit ends.
func (s *Steppir) SetFreq(mhz float64) error {
	if mhz < 1 || mhz > 60 {
		return fmt.Errorf("SteppIR: %.3f MHz is out of range", mhz)
	}
	if s.app.steppirTX() {
		s.mu.Lock()
		s.pending = mhz
		s.st.Holding = true
		s.mu.Unlock()
		s.app.changed()
		return nil
	}
	s.mu.Lock()
	dir := s.dirByte
	s.mu.Unlock()
	if err := s.packet(mhz, dir, stCmdSet); err != nil {
		return err
	}
	s.mu.Lock()
	s.lastSent = mhz
	s.pending = 0
	s.st.Holding = false
	s.mu.Unlock()
	s.app.logf("SteppIR: tune to %.3f MHz", mhz)
	s.app.changed()
	return nil
}

// Follow is called with the controlling radio's frequency when tracking is on.
func (s *Steppir) Follow(mhz float64, thresholdKHz int) {
	cfg := s.app.config()
	band := cfg.bandFor(mhz)
	if band == "" {
		return
	}
	s.mu.Lock()
	base := s.lastSent
	if base <= 0 {
		base = s.st.MHz
	}
	pending := s.pending
	s.mu.Unlock()
	if pending > 0 || base <= 0 || cfg.bandFor(base) != band || math.Abs(mhz-base)*1000 >= float64(thresholdKHz) {
		if err := s.SetFreq(mhz); err != nil {
			s.app.logf("%v", err)
		}
	}
}

func (s *Steppir) Step(sign int) error {
	mhz, _, err := s.current()
	if err != nil {
		return err
	}
	step := float64(s.app.config().Steppir.StepKHz) / 1000
	if sign < 0 {
		step = -step
	}
	return s.SetFreq(mhz + step)
}

func (s *Steppir) command(dir, cmd byte, what string) error {
	if s.app.steppirTX() {
		return errTransmitting
	}
	mhz, _, err := s.current()
	if err != nil && cmd != stCmdHome && cmd != stCmdCalibrate {
		return err
	}
	if err := s.packet(mhz, dir, cmd); err != nil {
		return err
	}
	s.app.logf("SteppIR: %s", what)
	return nil
}

func (s *Steppir) SetDirection(d string) error {
	var b byte
	switch d {
	case "normal":
		b = stDirNormal
	case "180":
		b = stDir180
	case "bi":
		b = stDirBi
	case "34":
		b = stDir34
	default:
		return fmt.Errorf("SteppIR: unknown direction %q", d)
	}
	if err := s.command(b, stCmdSet, "direction "+d); err != nil {
		return err
	}
	s.mu.Lock()
	s.dirByte = b
	s.mu.Unlock()
	return nil
}

func (s *Steppir) pendingFreq() float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pending
}

func (s *Steppir) dir() byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dirByte
}

func (s *Steppir) Retract() error   { return s.command(s.dir(), stCmdHome, "retract elements (home)") }
func (s *Steppir) Calibrate() error { return s.command(s.dir(), stCmdCalibrate, "calibrate") }

func (s *Steppir) Autotrack(on bool) error {
	if on {
		return s.command(s.dir(), stCmdAutotrackOn, "controller autotrack on")
	}
	return s.command(s.dir(), stCmdAutotrackOff, "controller autotrack off")
}

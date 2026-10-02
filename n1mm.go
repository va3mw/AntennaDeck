package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"math"
	"net"
	"strconv"
	"strings"
	"sync"
)

// N1MM Logger+ UDP broadcasts:
//   - rotor commands (default port 12040):
//     <N1MMRotor><rotor>name</rotor><goazi>173.0</goazi>...</N1MMRotor>
//     <N1MMRotor><stop>name</stop></N1MMRotor>
//   - radio info (default port 12060): <RadioInfo> with Freq/TXFreq in 10 Hz units.
type RadioState struct {
	Nr    int     `json:"nr"`
	MHz   float64 `json:"mhz"`
	TXMHz float64 `json:"txMhz"`
	Mode  string  `json:"mode"`
	TX    bool    `json:"tx"`
	Seen  bool    `json:"seen"`
}

type N1MMState struct {
	Radios         [2]RadioState `json:"radios"`
	ActiveRadio    int           `json:"activeRadio"`
	LastAz         int           `json:"lastAz"` // last heading requested by N1MM, -1 = none
	RotorListening bool          `json:"rotorListening"`
	RadioListening bool          `json:"radioListening"`
}

type N1MM struct {
	app   *App
	mu    sync.Mutex
	st    N1MMState
	conns []net.PacketConn
}

func newN1MM(a *App) *N1MM {
	return &N1MM{app: a, st: N1MMState{LastAz: -1, Radios: [2]RadioState{{Nr: 1}, {Nr: 2}}}}
}

func (n *N1MM) snapshot() N1MMState {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.st
}

func (n *N1MM) start(ctx context.Context) {
	c := n.app.config().N1MM
	rotorOK, radioOK := false, false
	if c.RotorEnabled {
		rotorOK = n.listen(c.RotorPort, "rotor", n.handleRotor)
	}
	if c.RadioEnabled {
		radioOK = n.listen(c.RadioPort, "radio info", n.handleRadio)
	}
	n.mu.Lock()
	n.st.RotorListening, n.st.RadioListening = rotorOK, radioOK
	n.mu.Unlock()
	n.app.changed()
}

func (n *N1MM) stop() {
	n.mu.Lock()
	conns := n.conns
	n.conns = nil
	n.mu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
}

func (n *N1MM) listen(port int, what string, handle func([]byte)) bool {
	pc, err := net.ListenPacket("udp4", ":"+strconv.Itoa(port))
	if err != nil {
		n.app.logf("N1MM: cannot listen for %s on UDP %d (%v). Is another program using that port?", what, port, err)
		return false
	}
	n.app.logf("N1MM: listening for %s on UDP %d", what, port)
	n.mu.Lock()
	n.conns = append(n.conns, pc)
	n.mu.Unlock()
	go func() {
		buf := make([]byte, 65536)
		for {
			k, _, err := pc.ReadFrom(buf)
			if err != nil {
				return // closed
			}
			handle(append([]byte(nil), buf[:k]...))
		}
	}()
	return true
}

func decodeXML(b []byte, v any) error {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	d := xml.NewDecoder(bytes.NewReader(b))
	d.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	return d.Decode(v)
}

func (n *N1MM) handleRotor(b []byte) {
	var m struct {
		XMLName xml.Name `xml:"N1MMRotor"`
		Rotor   string   `xml:"rotor"`
		GoAzi   string   `xml:"goazi"`
		Stop    *string  `xml:"stop"`
	}
	if err := decodeXML(b, &m); err != nil {
		return
	}
	name := strings.TrimSpace(m.Rotor)
	if m.Stop != nil && name == "" {
		name = strings.TrimSpace(*m.Stop)
	}
	if want := n.app.config().N1MM.RotorName; want != "" && !strings.EqualFold(name, want) {
		n.app.logf("N1MM: ignoring command for rotor '%s'", name)
		return
	}
	if m.Stop != nil {
		n.app.logf("N1MM: stop rotor")
		if err := n.app.rotor.Stop(); err != nil {
			n.app.logf("%v", err)
		}
		return
	}
	az, err := strconv.ParseFloat(strings.TrimSpace(m.GoAzi), 64)
	if err != nil {
		return
	}
	iaz := ((int(math.Round(az)) % 360) + 360) % 360
	n.mu.Lock()
	n.st.LastAz = iaz
	n.mu.Unlock()
	n.app.logf("N1MM: turn rotor to %d°", iaz)
	if err := n.app.rotor.GoTo(iaz); err != nil {
		n.app.logf("%v", err)
	}
	n.app.changed()
}

func (n *N1MM) handleRadio(b []byte) {
	var m struct {
		XMLName        xml.Name `xml:"RadioInfo"`
		RadioNr        string   `xml:"RadioNr"`
		Freq           string   `xml:"Freq"`
		TXFreq         string   `xml:"TXFreq"`
		Mode           string   `xml:"Mode"`
		IsTransmitting string   `xml:"IsTransmitting"`
		ActiveRadioNr  string   `xml:"ActiveRadioNr"`
	}
	if err := decodeXML(b, &m); err != nil {
		return
	}
	nr, _ := strconv.Atoi(strings.TrimSpace(m.RadioNr))
	if nr < 1 || nr > 2 {
		return
	}
	mhz := func(s string) float64 {
		v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
		return v / 100000 // N1MM sends 10 Hz units
	}
	r := RadioState{
		Nr:    nr,
		MHz:   mhz(m.Freq),
		TXMHz: mhz(m.TXFreq),
		Mode:  strings.TrimSpace(m.Mode),
		TX:    strings.EqualFold(strings.TrimSpace(m.IsTransmitting), "true"),
		Seen:  true,
	}
	n.mu.Lock()
	changed := n.st.Radios[nr-1] != r
	n.st.Radios[nr-1] = r
	if a, _ := strconv.Atoi(strings.TrimSpace(m.ActiveRadioNr)); a >= 1 && a <= 2 && a != n.st.ActiveRadio {
		n.st.ActiveRadio = a
		changed = true
	}
	n.mu.Unlock()
	if changed {
		n.app.changed()
		n.app.steppirFollow()
	}
}

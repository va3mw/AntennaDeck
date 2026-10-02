package main

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"
	"time"
)

// Fake AG: checks that commands end in LF and answers like firmware 4.1.16.
func TestAntennaGeniusSession(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		c.Write([]byte("V4.1.16 AG\n"))
		r := bufio.NewReader(c)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\n")
			seq, cmd, _ := strings.Cut(line[1:], "|")
			reply := func(msg string) { c.Write([]byte("R" + seq + "|0|" + msg + "\n")) }
			switch cmd {
			case "antenna list":
				reply("antenna 1 name=SteppIR tx=0FFF rx=0FFF inband=0000")
				reply("antenna 2 name=40m_array tx=0008 rx=0008 inband=0000")
				reply("")
			case "band list":
				reply("band 5 name=20m freq_start=13.800000 freq_stop=14.550000")
				reply("")
			case "port get 1":
				reply("port 1 auto=1 source=AUTO band=5 rxant=1 txant=1 tx=0 inhibit=0")
			default:
				reply("")
			}
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	cfg := defaultConfig()
	cfg.AG = AGConfig{Enabled: true, Host: "127.0.0.1", Port: addr.Port}
	cfg.Rotor.Enabled, cfg.Steppir.Enabled = false, false
	a := &App{cfg: cfg}
	a.rotor, a.steppir, a.n1mm = newRotator(a), newSteppir(a), newN1MM(a)
	a.ag = newAntennaGenius(a)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	go a.ag.link.run(ctx)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		st := a.ag.snapshot()
		if len(st.Antennas) == 2 && st.Ports[0].RxAnt == 1 && st.Ports[0].BandName == "20m" {
			if st.Antennas[1].Name != "40m array" {
				t.Fatalf("name: %q", st.Antennas[1].Name)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("AG state never filled in: %+v", a.ag.snapshot())
}

package main

import "testing"

// Reply captured from the station's Rotator Genius.
func TestParseRG(t *testing.T) {
	b1 := "173359  0A0  10999999\x00Steppir     "
	b2 := "999359  0A0   0999999\x00Rotator 2 - "
	for _, hdr := range []string{"|h2\x00", "|h2"} {
		s := hdr + b1 + b2
		st, _, ok := parseRG(s, 1)
		if !ok || st.Azimuth != 173 || st.Target != -1 || st.Moving != 0 || st.Name != "Steppir" {
			t.Fatalf("hdr %q rotor1: %+v ok=%v", hdr, st, ok)
		}
		st, _, ok = parseRG(s, 2)
		if !ok || st.Azimuth != -1 || st.Name != "Rotator 2 -" {
			t.Fatalf("hdr %q rotor2: %+v ok=%v", hdr, st, ok)
		}
	}
	// moving CW to 220, followed by the start of the next reply
	s := "|h2\x00" + "045359  0A1  10220999\x00Steppir     " + b2 + "|h2\x00158"
	st, _, ok := parseRG(s, 1)
	if !ok || st.Azimuth != 45 || st.Moving != 1 || st.Target != 220 {
		t.Fatalf("moving: %+v ok=%v", st, ok)
	}
}

func TestSteppirStatus(t *testing.T) {
	a := &App{cfg: defaultConfig()}
	s := newSteppir(a)
	f := uint32(1420000) // 14.200 MHz in 10 Hz units
	s.onData([]byte{'x', '@', 'A', 0, byte(f >> 16), byte(f >> 8), byte(f), 0, 0x40, '1', '0', 0x0D})
	st := s.snapshot()
	if st.MHz != 14.2 || st.Direction != "180" || st.Band != "20m" || st.Moving {
		t.Fatalf("%+v", st)
	}
}

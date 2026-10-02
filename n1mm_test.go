package main

import "testing"

func TestN1MMMessages(t *testing.T) {
	a := &App{cfg: defaultConfig()}
	a.rotor = newRotator(a)
	a.steppir = newSteppir(a)
	a.ag = newAntennaGenius(a)
	a.n1mm = newN1MM(a)

	a.n1mm.handleRadio([]byte(`<?xml version="1.0" encoding="utf-8"?>
<RadioInfo><app>N1MM</app><StationName>SHACK</StationName><RadioNr>2</RadioNr>
<Freq>702500</Freq><TXFreq>702500</TXFreq><Mode>CW</Mode><IsTransmitting>True</IsTransmitting>
<ActiveRadioNr>2</ActiveRadioNr></RadioInfo>`))
	n := a.n1mm.snapshot()
	r := n.Radios[1]
	if !r.Seen || r.MHz != 7.025 || r.Mode != "CW" || !r.TX || n.ActiveRadio != 2 {
		t.Fatalf("radio: %+v active=%d", r, n.ActiveRadio)
	}

	// The rotor is not connected in the test, so this only exercises parsing.
	a.n1mm.handleRotor([]byte(`<?xml version="1.0" encoding="utf-8"?>
<N1MMRotor><rotor>Steppir</rotor><goazi>222.6</goazi><offset>0.0</offset><bidirectional>0</bidirectional><freqband>14</freqband></N1MMRotor>`))
	if got := a.n1mm.snapshot().LastAz; got != 223 {
		t.Fatalf("lastAz = %d", got)
	}
}

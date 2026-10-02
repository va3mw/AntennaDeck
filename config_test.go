package main

import "testing"

// A fresh install must come up with usable values (a zero poll interval
// would crash the ticker).
func TestDefaultConfigIsUsable(t *testing.T) {
	c := defaultConfig()
	c.normalize()
	if c.Rotor.PollMs < 200 || c.Rotor.Number != 1 || c.Rotor.BeamWidth <= 0 {
		t.Fatalf("rotor defaults: %+v", c.Rotor)
	}
	if c.Steppir.PollMs < 300 || c.API.Port != 8090 {
		t.Fatalf("defaults: steppir poll %d, api port %d", c.Steppir.PollMs, c.API.Port)
	}
}

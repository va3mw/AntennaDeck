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

// Each of the four layouts keeps its own size; v1.0 sizes are still honoured.
func TestLayoutSizes(t *testing.T) {
	c := defaultConfig()
	c.Window.HW, c.Window.HH = 900, 330 // saved by v1.0
	if w, h := c.windowSize(); w != 900 || h != 330 {
		t.Fatalf("legacy horizontal: %dx%d", w, h)
	}
	c.Mini = true
	if w, h := c.windowSize(); w != 500 || h != 250 {
		t.Fatalf("mini horizontal default: %dx%d", w, h)
	}
	c.rememberSize(520, 260)
	c.Layout = "vertical"
	if w, h := c.windowSize(); w != 270 || h != 585 {
		t.Fatalf("mini vertical default: %dx%d", w, h)
	}
	c.Layout, c.Mini = "horizontal", true
	if w, h := c.windowSize(); w != 520 || h != 260 {
		t.Fatalf("mini horizontal remembered: %dx%d", w, h)
	}
}

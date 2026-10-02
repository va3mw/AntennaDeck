package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// No devices are connected here, so nothing can actually move.
func newTestAPI(key string) (*App, *apiServer) {
	cfg := defaultConfig()
	cfg.API = APIConfig{Enabled: true, Port: 8090, Key: key}
	a := &App{cfg: cfg}
	a.rotor, a.steppir, a.ag, a.n1mm = newRotator(a), newSteppir(a), newAntennaGenius(a), newN1MM(a)
	return a, newAPIServer(a)
}

func get(s *apiServer, path string) (int, string) {
	rec := httptest.NewRecorder()
	s.serve(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code, rec.Body.String()
}

func TestAPIRouting(t *testing.T) {
	_, s := newTestAPI("")

	if code, body := get(s, "/api/status"); code != 200 || !strings.Contains(body, `"rotor"`) {
		t.Fatalf("status: %d %s", code, body)
	}
	// valid commands reach the device code and fail only because nothing is connected
	for _, p := range []string{"/api/rotor/220", "/api/rotor/preset/eu", "/api/rotor/stop",
		"/api/steppir/band/20m", "/api/steppir/band/20", "/api/ag/A/3"} {
		code, body := get(s, p)
		var r struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		_ = json.Unmarshal([]byte(body), &r)
		if code != 400 || !strings.Contains(r.Error, "not connected") {
			t.Fatalf("%s: %d %s", p, code, body)
		}
	}
	for _, p := range []string{"/api/rotor/400", "/api/rotor/preset/XX", "/api/ag/C/1", "/api/nonsense"} {
		if code, body := get(s, p); code != 400 || strings.Contains(body, "not connected") {
			t.Fatalf("%s should be rejected before reaching a device: %d %s", p, code, body)
		}
	}
	if code, body := get(s, "/"); code != 200 || !strings.Contains(body, "api/rotor/preset/EU") {
		t.Fatalf("help page: %d", code)
	}
}

func TestAPIKey(t *testing.T) {
	_, s := newTestAPI("s3cret")
	if code, _ := get(s, "/api/status"); code != 401 {
		t.Fatalf("no key: %d", code)
	}
	if code, _ := get(s, "/api/status?key=s3cret"); code != 200 {
		t.Fatalf("with key: %d", code)
	}
}

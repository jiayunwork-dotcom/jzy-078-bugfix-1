package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"cycloneservice/cyclone"
)

func postJSON(t *testing.T, r http.Handler, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var decoded map[string]any
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &decoded); err != nil {
			t.Fatalf("response not JSON (%d): %s", w.Code, w.Body.String())
		}
	}
	return w, decoded
}

func getJSON(t *testing.T, r http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func baseParams() map[string]any {
	return map[string]any{
		"cylinder_diameter": 1.0,
		"inlet_velocity":    20.0,
		"gas_density":       1.2,
		"solid_density":     2000.0,
		"gas_viscosity":     1.8e-5,
	}
}

func TestHealth(t *testing.T) {
	r := NewRouter()
	w := getJSON(t, r, "/healthz")
	if w.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200", w.Code)
	}
}

func TestGeometryEndpoint(t *testing.T) {
	r := NewRouter()
	w, body := postJSON(t, r, "/api/v1/cyclone/geometry", map[string]any{"cylinder_diameter": 2.0})
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	g := body["geometry"].(map[string]any)
	if g["inlet_width"].(float64) != 0.4 {
		t.Errorf("inlet_width = %v, want 0.4", g["inlet_width"])
	}
	if g["inlet_height"].(float64) != 1.0 {
		t.Errorf("inlet_height = %v, want 1.0", g["inlet_height"])
	}
	ratios := g["ratios"].(map[string]any)
	if ratios["inlet_width"].(float64) != 0.2 {
		t.Errorf("ratio a/D = %v, want 0.2", ratios["inlet_width"])
	}
}

func TestCutPointEndpoint(t *testing.T) {
	r := NewRouter()
	w, body := postJSON(t, r, "/api/v1/cyclone/cut-point", baseParams())
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	d50, ok := body["cut_diameter_d50_m"].(float64)
	if !ok || d50 <= 0 {
		t.Fatalf("missing positive d50: %v", body)
	}
	if body["stokes_assumption_valid"] != true {
		t.Errorf("expected stokes valid, body = %v", body)
	}
	if _, ok := body["particle_reynolds"].(float64); !ok {
		t.Error("missing particle_reynolds")
	}
}

func TestGradeEndpoint(t *testing.T) {
	r := NewRouter()
	payload := baseParams()
	payload["particle_diameter_m"] = 8.031e-6
	w, body := postJSON(t, r, "/api/v1/cyclone/grade-efficiency", payload)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	eta := body["collection_efficiency"].(float64)
	if eta < 0.45 || eta > 0.55 {
		t.Errorf("efficiency at d50 = %.4f, want ~0.5", eta)
	}
	p := body["penetration"].(float64)
	if absF(p-(1-eta)) > 1e-12 {
		t.Errorf("penetration %.6f inconsistent", p)
	}
}

func TestDistributionEndpoint(t *testing.T) {
	r := NewRouter()
	payload := baseParams()
	payload["bins"] = []map[string]any{
		{"diameter_m": 1e-6, "weight": 0.25},
		{"diameter_m": 8e-6, "weight": 0.5},
		{"diameter_m": 40e-6, "weight": 0.25},
	}
	w, body := postJSON(t, r, "/api/v1/cyclone/distribution-efficiency", payload)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	eta := body["overall_efficiency"].(float64)
	if eta <= 0 || eta >= 1 {
		t.Errorf("overall eta = %.4f outside (0,1)", eta)
	}
	bins := body["bins"].([]any)
	if len(bins) != 3 {
		t.Errorf("got %d bins, want 3", len(bins))
	}
}

// Many clients posting distributions with different weight totals at the same
// time must each get a response that describes only their own request, and
// every concurrent response must be byte-identical to the serial one.
// Regression test for warnings leaking across requests through shared state.
func TestDistributionEndpointConcurrentClientsMatchSerial(t *testing.T) {
	srv := httptest.NewServer(NewRouter())
	defer srv.Close()
	url := srv.URL + "/api/v1/cyclone/distribution-efficiency"
	client := &http.Client{}

	sums := []float64{20, 30, 40, 50, 60, 70, 80, 90}
	allSums := append(append([]float64{}, sums...), 1) // 1 = pre-normalized feed

	payloadFor := func(sum float64) map[string]any {
		payload := baseParams()
		diameters := []float64{1e-6, 3e-6, 8e-6, 20e-6, 60e-6}
		bins := make([]map[string]any, len(diameters))
		for i, d := range diameters {
			// sum/5 is exact for these totals, so weight_sum is exactly sum.
			bins[i] = map[string]any{"diameter_m": d, "weight": sum / 5}
		}
		payload["bins"] = bins
		return payload
	}

	post := func(payload map[string]any) (string, error) {
		raw, err := json.Marshal(payload)
		if err != nil {
			return "", err
		}
		resp, err := client.Post(url, "application/json", bytes.NewReader(raw))
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("status %d: %s", resp.StatusCode, body)
		}
		return string(body), nil
	}

	// Serial pass: record the golden response body per distribution and pin
	// the warning contract — the warning must quote this distribution's own
	// weight sum, nobody else's; a unit-sum feed must not warn at all.
	golden := make(map[float64]string, len(allSums))
	for _, sum := range allSums {
		body, err := post(payloadFor(sum))
		if err != nil {
			t.Fatalf("serial request for sum %v: %v", sum, err)
		}
		golden[sum] = body

		if !strings.Contains(body, fmt.Sprintf(`"weight_sum":%v`, sum)) {
			t.Errorf("sum %v: response reports a different weight_sum: %s", sum, body)
		}
		if sum == 1 {
			if strings.Contains(body, "bin weights sum to") {
				t.Errorf("unit-sum distribution must not warn, got: %s", body)
			}
			continue
		}
		if !strings.Contains(body, fmt.Sprintf("bin weights sum to %v, not 1", sum)) {
			t.Errorf("sum %v: warning missing or quotes another sum: %s", sum, body)
		}
		for _, other := range sums {
			if other != sum && strings.Contains(body, fmt.Sprintf("bin weights sum to %v,", other)) {
				t.Errorf("sum %v: response carries sum %v's warning: %s", sum, other, body)
			}
		}
	}

	// Concurrent storm: several clients per distribution, each looping. Every
	// single response must equal the serial golden byte for byte.
	const clientsPerSum = 2
	const iterations = 100
	errs := make(chan string, 4096)
	var wg sync.WaitGroup
	for _, sum := range allSums {
		for c := 0; c < clientsPerSum; c++ {
			wg.Add(1)
			go func(sum float64) {
				defer wg.Done()
				payload := payloadFor(sum)
				for i := 0; i < iterations; i++ {
					body, err := post(payload)
					if err != nil {
						errs <- fmt.Sprintf("sum %v: %v", sum, err)
						return
					}
					if body != golden[sum] {
						errs <- fmt.Sprintf("sum %v: concurrent response diverged from serial:\n got: %s\nwant: %s",
							sum, body, golden[sum])
						return
					}
				}
			}(sum)
		}
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

// The warnings field must always serialize as a JSON array — [] when empty,
// never null — regardless of regime or endpoint.
func TestWarningsSerializeAsEmptyArray(t *testing.T) {
	r := NewRouter()

	// Stokes-regime, unit-sum distribution.
	payload := baseParams()
	payload["bins"] = []map[string]any{{"diameter_m": 8e-6, "weight": 1.0}}
	w, _ := postJSON(t, r, "/api/v1/cyclone/distribution-efficiency", payload)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"warnings":[]`) {
		t.Errorf("distribution warnings should serialize as [], got: %s", w.Body.String())
	}

	w2, _ := postJSON(t, r, "/api/v1/cyclone/cut-point", baseParams())
	if !strings.Contains(w2.Body.String(), `"warnings":[]`) {
		t.Errorf("cut-point warnings should serialize as [], got: %s", w2.Body.String())
	}

	g := baseParams()
	g["particle_diameter_m"] = 8e-6
	w3, _ := postJSON(t, r, "/api/v1/cyclone/grade-efficiency", g)
	if !strings.Contains(w3.Body.String(), `"warnings":[]`) {
		t.Errorf("grade warnings should serialize as [], got: %s", w3.Body.String())
	}
}

func TestInvalidInputsReturnStructuredErrors(t *testing.T) {
	r := NewRouter()

	// Semantic validation -> 422 with field-level structured error.
	bad := baseParams()
	bad["solid_density"] = 0.5 // lighter than carrier gas
	w, body := postJSON(t, r, "/api/v1/cyclone/cut-point", bad)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422; body = %s", w.Code, w.Body.String())
	}
	e := body["error"].(map[string]any)
	if e["code"] != cyclone.ErrInvalidDensity {
		t.Errorf("code = %v, want %s", e["code"], cyclone.ErrInvalidDensity)
	}
	if e["field"] != "solid_density" {
		t.Errorf("field = %v, want solid_density", e["field"])
	}

	// Every endpoint applies the same validation.
	for _, path := range []string{
		"/api/v1/cyclone/geometry",
		"/api/v1/cyclone/grade-efficiency",
		"/api/v1/cyclone/distribution-efficiency",
	} {
		w, _ := postJSON(t, r, path, map[string]any{"cylinder_diameter": -1})
		if w.Code < 400 {
			t.Errorf("%s accepted invalid payload with %d", path, w.Code)
		}
	}

	// Malformed JSON -> 400.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cyclone/cut-point", bytes.NewReader([]byte("{not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("malformed json status = %d, want 400", rec.Code)
	}
}

func TestUnknownRoute(t *testing.T) {
	r := NewRouter()
	w := getJSON(t, r, "/nope")
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func absF(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

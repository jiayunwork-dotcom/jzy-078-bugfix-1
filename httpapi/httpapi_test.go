package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// distributionPayload builds a distribution-efficiency request with five
// size classes whose weights add up to total. Totals that are multiples of
// 5 give binary-exact per-bin weights, so the server-side weight_sum comes
// back as exactly total.
func distributionPayload(total float64) map[string]any {
	payload := baseParams()
	diameters := []float64{1e-6, 3e-6, 8e-6, 20e-6, 60e-6}
	per := total / float64(len(diameters))
	bins := make([]map[string]any, len(diameters))
	for i, d := range diameters {
		bins[i] = map[string]any{"diameter_m": d, "weight": per}
	}
	payload["bins"] = bins
	return payload
}

// postDistribution fires one distribution-efficiency request through the
// full gin stack. It never calls t.Fatal, so it is safe to use from
// concurrent goroutines.
func postDistribution(r http.Handler, total float64) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(distributionPayload(total))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/cyclone/distribution-efficiency", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// Regression test for cross-request warning leakage: with several clients
// posting distributions whose weight sums differ, every response's warning
// must quote its own weight sum — never a neighbour's. The old
// implementation appended to a package-level shared slice, so under
// concurrency a response could carry another request's message (and the
// race detector fired). Weight sums are distinct multiples of 10 so each
// warning marker ("sum to 20," etc.) is unique to one client.
func TestDistributionEndpointConcurrentWarningsMatchOwnWeightSum(t *testing.T) {
	r := NewRouter()
	totals := []float64{20, 30, 40, 50, 60, 70, 80, 90}

	markers := make([]string, len(totals))
	for i, total := range totals {
		markers[i] = "sum to " + strconv.FormatFloat(total, 'g', 4, 64) + ","
	}

	const iterations = 60
	var wg sync.WaitGroup
	for idx, total := range totals {
		wg.Add(1)
		go func(idx int, total float64) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				w := postDistribution(r, total)
				if w.Code != http.StatusOK {
					t.Errorf("total %v: status = %d, body = %s", total, w.Code, w.Body.String())
					return
				}
				var resp struct {
					WeightSum float64  `json:"weight_sum"`
					Warnings  []string `json:"warnings"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Errorf("total %v: response not JSON: %v", total, err)
					return
				}
				if resp.WeightSum != total {
					t.Errorf("total %v: response weight_sum = %v", total, resp.WeightSum)
					return
				}
				if len(resp.Warnings) != 1 {
					t.Errorf("total %v: warnings = %v, want exactly one normalization warning", total, resp.Warnings)
					return
				}
				if !strings.Contains(resp.Warnings[0], markers[idx]) {
					t.Errorf("total %v: warning %q does not mention its own weight sum", total, resp.Warnings[0])
					return
				}
				for j, m := range markers {
					if j != idx && strings.Contains(resp.Warnings[0], m) {
						t.Errorf("total %v: warning %q leaked from the request with weight sum %v", total, resp.Warnings[0], totals[j])
						return
					}
				}
			}
		}(idx, total)
	}
	wg.Wait()
}

// Responses produced under concurrency must be byte-for-byte identical to
// the responses the same requests get when issued serially.
func TestDistributionEndpointConcurrentResponsesMatchSerial(t *testing.T) {
	r := NewRouter()
	totals := []float64{1, 20, 30, 40, 55, 90} // include the no-warning unit sum

	type goldenResponse struct {
		status int
		body   []byte
	}
	golden := make(map[float64]goldenResponse, len(totals))
	for _, total := range totals {
		w := postDistribution(r, total)
		if w.Code != http.StatusOK {
			t.Fatalf("total %v: serial status = %d, body = %s", total, w.Code, w.Body.String())
		}
		golden[total] = goldenResponse{status: w.Code, body: w.Body.Bytes()}
	}

	const iterations = 40
	var wg sync.WaitGroup
	for _, total := range totals {
		wg.Add(1)
		go func(total float64) {
			defer wg.Done()
			want := golden[total]
			for i := 0; i < iterations; i++ {
				w := postDistribution(r, total)
				if w.Code != want.status || !bytes.Equal(w.Body.Bytes(), want.body) {
					t.Errorf("total %v: concurrent response differs from serial\ngot  %d %s\nwant %d %s",
						total, w.Code, w.Body.String(), want.status, want.body)
					return
				}
			}
		}(total)
	}
	wg.Wait()
}

// A distribution whose weights already sum to 1 must come back with an
// empty warnings array — no normalization warning, and [] rather than null.
func TestDistributionEndpointUnitWeightSumHasNoWarning(t *testing.T) {
	r := NewRouter()
	w := postDistribution(r, 1)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	warnings, ok := resp["warnings"].([]any)
	if !ok {
		t.Fatalf("warnings = %v, want a JSON array (not null/missing)", resp["warnings"])
	}
	if len(warnings) != 0 {
		t.Errorf("unit weight sum should carry no warning, got %v", warnings)
	}
	if resp["weights_normalized"] != true {
		t.Errorf("weights_normalized = %v, want true", resp["weights_normalized"])
	}
}

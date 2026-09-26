package cyclone

import (
	"reflect"
	"sync"
	"testing"
)

// binsWithWeightSum builds five size classes (the same diameters as
// testBins) whose weights add up to total. Totals that are multiples of 5
// give binary-exact per-bin weights, so the server-side sum is exactly
// total and trimFloat renders it without decimals.
func binsWithWeightSum(total float64) []DistributionBin {
	diameters := []float64{1e-6, 3e-6, 8e-6, 20e-6, 60e-6}
	per := total / float64(len(diameters))
	bins := make([]DistributionBin, len(diameters))
	for i, d := range diameters {
		bins[i] = DistributionBin{Diameter: d, Weight: per}
	}
	return bins
}

func normalizationWarning(weightSum float64) string {
	return "bin weights sum to " + trimFloat(weightSum) +
		", not 1; results were computed after normalization"
}

// Regression test for cross-request warning leakage: the normalization
// warning used to be appended to a package-level shared slice with spare
// capacity, so concurrent requests overwrote each other's message and a
// response could quote another request's weight sum. Every result must
// describe only its own distribution, no matter the parallelism. Run with
// -race: the old shared slice also tripped the race detector.
func TestDistributionWarningsIsolatedUnderConcurrency(t *testing.T) {
	in := baselineInput()
	totals := []float64{20, 30, 40, 50, 60, 70, 80, 90}

	const iterations = 100
	var wg sync.WaitGroup
	for _, total := range totals {
		wg.Add(1)
		go func(total float64) {
			defer wg.Done()
			bins := binsWithWeightSum(total)
			want := normalizationWarning(total)
			for i := 0; i < iterations; i++ {
				res, err := EvaluateDistribution(in, bins)
				if err != nil {
					t.Errorf("total %v: unexpected error: %v", total, err)
					return
				}
				if res.WeightSum != total {
					t.Errorf("total %v: weight sum = %v", total, res.WeightSum)
					return
				}
				if len(res.Warnings) != 1 || res.Warnings[0] != want {
					t.Errorf("total %v: warnings = %v, want exactly [%q]", total, res.Warnings, want)
					return
				}
			}
		}(total)
	}
	wg.Wait()
}

// A result already handed to a caller must stay stable: later evaluations
// with different weight sums must not reach back into it. This fails
// deterministically against the old shared-slice implementation even
// without goroutines.
func TestDistributionResultNotMutatedByLaterCalls(t *testing.T) {
	in := baselineInput()
	first, err := EvaluateDistribution(in, binsWithWeightSum(20))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(first.Warnings) != 1 {
		t.Fatalf("expected one normalization warning, got %v", first.Warnings)
	}
	want := first.Warnings[0]

	for _, total := range []float64{30, 40, 90, 1, 55} {
		if _, err := EvaluateDistribution(in, binsWithWeightSum(total)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if first.Warnings[0] != want {
		t.Errorf("earlier result mutated by later calls: warning %q became %q", want, first.Warnings[0])
	}
}

// Concurrent evaluations must produce exactly the same result as serial
// ones, field by field.
func TestDistributionConcurrentResultsMatchSerial(t *testing.T) {
	in := baselineInput()
	totals := []float64{1, 20, 30, 40, 55, 90} // include the no-warning unit sum

	golden := make(map[float64]DistributionResult, len(totals))
	for _, total := range totals {
		res, err := EvaluateDistribution(in, binsWithWeightSum(total))
		if err != nil {
			t.Fatalf("total %v: unexpected error: %v", total, err)
		}
		golden[total] = res
	}

	const iterations = 50
	var wg sync.WaitGroup
	for _, total := range totals {
		wg.Add(1)
		go func(total float64) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				res, err := EvaluateDistribution(in, binsWithWeightSum(total))
				if err != nil {
					t.Errorf("total %v: unexpected error: %v", total, err)
					return
				}
				if !reflect.DeepEqual(res, golden[total]) {
					t.Errorf("total %v: concurrent result differs from serial\ngot  %+v\nwant %+v", total, res, golden[total])
					return
				}
			}
		}(total)
	}
	wg.Wait()
}

// A distribution whose weights already sum to 1 must carry no warning, and
// the warnings slice must stay non-nil so the JSON payload renders []
// rather than null.
func TestDistributionUnitWeightSumHasNoWarning(t *testing.T) {
	in := baselineInput()
	res, err := EvaluateDistribution(in, testBins())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.NormalizedWeights {
		t.Error("unit weight sum should be flagged as already normalized")
	}
	if res.Warnings == nil {
		t.Error("warnings must be a non-nil empty slice so JSON renders []")
	}
	if len(res.Warnings) != 0 {
		t.Errorf("unit weight sum should carry no warning, got %v", res.Warnings)
	}
}

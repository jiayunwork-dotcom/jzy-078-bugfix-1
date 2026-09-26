package cyclone

import (
	"math"
	"testing"
)

func TestGradeEfficiencyShape(t *testing.T) {
	// eta(d50) = 0.5.
	if eta := gradeEfficiency(1e-5, 1e-5); math.Abs(eta-0.5) > 1e-12 {
		t.Errorf("eta(d50) = %.6f, want 0.5", eta)
	}
	// Limits: fine dust escapes, coarse dust is captured.
	if eta := gradeEfficiency(1e-9, 1e-5); eta > 1e-6 {
		t.Errorf("very fine particle efficiency %.3e should approach 0", eta)
	}
	if eta := gradeEfficiency(1e-2, 1e-5); eta < 1-1e-6 {
		t.Errorf("very coarse particle efficiency %.6f should approach 1", eta)
	}
	if gradeEfficiency(1e-9, 1e-5) >= gradeEfficiency(1e-2, 1e-5) {
		t.Error("grade curve ordering violated")
	}
}

// Grade efficiency must be strictly monotone in particle size — a larger
// particle may never be collected less efficiently than a smaller one, and the
// service must not saturate at 100% across all sizes.
func TestGradeEndpointMonotonicAndNotSaturated(t *testing.T) {
	in := baselineInput()
	const n = 40
	var prev float64 = -1
	var diameters []float64
	var efficiencies []float64
	for k := 0; k < n; k++ {
		// Log-spaced diameters from 0.1 µm to 100 µm.
		logD := math.Log(1e-7) + float64(k)/float64(n-1)*(math.Log(1e-4)-math.Log(1e-7))
		d := math.Exp(logD)
		diameters = append(diameters, d)

		res, err := EvaluateGrade(in, d)
		if err != nil {
			t.Fatalf("EvaluateGrade(%g): %v", d, err)
		}
		efficiencies = append(efficiencies, res.CollectionEfficiency)
		if !(res.CollectionEfficiency > prev) {
			t.Fatalf("efficiency not strictly increasing at d=%.3e: %.12f <= %.12f", d, res.CollectionEfficiency, prev)
		}
		if !(res.CollectionEfficiency > 0 && res.CollectionEfficiency < 1) {
			t.Fatalf("efficiency %.6f saturated at a boundary (d=%.3e)", res.CollectionEfficiency, d)
		}
		if math.Abs(res.Penetration-(1-res.CollectionEfficiency)) > 1e-12 {
			t.Fatalf("penetration %.6f != 1 - eta %.6f", res.Penetration, 1-res.CollectionEfficiency)
		}
		prev = res.CollectionEfficiency
	}
	if efficiencies[0] > 0.01 {
		t.Errorf("finest grade efficiency %.4f should be near zero", efficiencies[0])
	}
	if efficiencies[n-1] < 0.99 {
		t.Errorf("coarsest grade efficiency %.4f should be near one", efficiencies[n-1])
	}
}

func TestGradeEndpointRejectsBadParticle(t *testing.T) {
	in := baselineInput()
	for _, d := range []float64{0, -1e-6, math.NaN()} {
		if _, err := EvaluateGrade(in, d); err == nil {
			t.Errorf("EvaluateGrade(%v) expected error", d)
		}
	}
	if _, err := EvaluateGrade(in, 1e-6); err != nil {
		t.Errorf("valid particle rejected: %v", err)
	}
}

func testBins() []DistributionBin {
	return []DistributionBin{
		{Diameter: 1e-6, Weight: 0.1}, // 1 µm, much finer than d50
		{Diameter: 3e-6, Weight: 0.2},
		{Diameter: 8e-6, Weight: 0.4}, // near d50
		{Diameter: 20e-6, Weight: 0.2},
		{Diameter: 60e-6, Weight: 0.1}, // coarser than d50
	}
}

// The integrated efficiency must equal the explicit weighted sum of the
// per-bin grade efficiencies.
func TestDistributionIntegrationMatchesWeightedSum(t *testing.T) {
	in := baselineInput()
	res, err := EvaluateDistribution(in, testBins())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(res.WeightSum-1) > 1e-12 {
		t.Errorf("weight sum = %v, want 1", res.WeightSum)
	}
	var manual float64
	for i, bin := range res.Bins {
		manual += bin.NormalizedWeight * bin.CollectionEfficiency
		if math.Abs(bin.NormalizedWeight-testBins()[i].Weight) > 1e-12 {
			t.Errorf("bin %d normalized weight drifted", i)
		}
	}
	if math.Abs(res.OverallEfficiency-manual) > 1e-12 {
		t.Errorf("overall eta = %.8f, weighted sum = %.8f", res.OverallEfficiency, manual)
	}
	if math.Abs(res.OverallPenetration-(1-res.OverallEfficiency)) > 1e-12 {
		t.Errorf("overall penetration inconsistent")
	}
	// Bounds and interior: mixed feed must neither be 0 nor 1.
	if res.OverallEfficiency <= 0 || res.OverallEfficiency >= 1 {
		t.Errorf("overall efficiency %.6f outside open interval (0,1)", res.OverallEfficiency)
	}
}

// Weighted-average of a strictly increasing curve must lie between the finest
// and coarsest bin efficiencies.
func TestDistributionEfficiencyBoundedByGrades(t *testing.T) {
	in := baselineInput()
	res, err := EvaluateDistribution(in, testBins())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	minEta := res.Bins[0].CollectionEfficiency
	maxEta := res.Bins[len(res.Bins)-1].CollectionEfficiency
	if !(res.OverallEfficiency > minEta && res.OverallEfficiency < maxEta) {
		t.Errorf("overall %.6f not within bin range (%.6f, %.6f)", res.OverallEfficiency, minEta, maxEta)
	}
}

// Core inverse relationship: for the same feed distribution, a larger d50 must
// never give a higher overall efficiency.
func TestDistributionEfficiencyDecreasesAsD50Grows(t *testing.T) {
	bins := testBins()
	smallD50 := baselineInput()
	bigD50 := baselineInput()
	// Halve inlet velocity: d50 grows by sqrt(2), geometry and feed unchanged.
	bigD50.InletVelocity /= 2

	cpSmall, _ := EvaluateCutPoint(smallD50)
	cpBig, _ := EvaluateCutPoint(bigD50)
	if !(cpBig.CutDiameter50 > cpSmall.CutDiameter50) {
		t.Fatalf("test setup: d50 did not grow: %v vs %v", cpBig.CutDiameter50, cpSmall.CutDiameter50)
	}

	a, errA := EvaluateDistribution(smallD50, bins)
	b, errB := EvaluateDistribution(bigD50, bins)
	if errA != nil || errB != nil {
		t.Fatalf("unexpected errors: %v %v", errA, errB)
	}
	if b.OverallEfficiency > a.OverallEfficiency {
		t.Errorf("larger d50 raised overall efficiency: %.6f (small d50 %.6f)", b.OverallEfficiency, a.OverallEfficiency)
	}
	if !(b.OverallEfficiency < a.OverallEfficiency) {
		t.Errorf("expected strictly lower efficiency with coarser cut: %.6f vs %.6f", b.OverallEfficiency, a.OverallEfficiency)
	}
	// Every common size class must also be captured no better.
	for i := range bins {
		if b.Bins[i].CollectionEfficiency > a.Bins[i].CollectionEfficiency+1e-15 {
			t.Errorf("bin %d efficiency rose with coarser d50", i)
		}
	}
}

func TestDistributionNormalizesNonUnitWeights(t *testing.T) {
	in := baselineInput()
	bins := []DistributionBin{
		{Diameter: 2e-6, Weight: 3},
		{Diameter: 8e-6, Weight: 7},
	}
	res, err := EvaluateDistribution(in, bins)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(res.WeightSum-10) > 1e-12 {
		t.Errorf("weight sum = %v, want 10", res.WeightSum)
	}
	var sumNorm float64
	for _, bin := range res.Bins {
		sumNorm += bin.NormalizedWeight
	}
	if math.Abs(sumNorm-1) > 1e-12 {
		t.Errorf("normalized weights sum = %.6f, want 1", sumNorm)
	}
	if len(res.Warnings) == 0 {
		t.Error("expected a normalization warning for non-unit weights")
	}

	// Equivalent distribution already normalized must yield the same value.
	binsNorm := []DistributionBin{
		{Diameter: 2e-6, Weight: 0.3},
		{Diameter: 8e-6, Weight: 0.7},
	}
	resNorm, err := EvaluateDistribution(in, binsNorm)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(res.OverallEfficiency-resNorm.OverallEfficiency) > 1e-12 {
		t.Errorf("normalization changed result: %.10f vs %.10f", res.OverallEfficiency, resNorm.OverallEfficiency)
	}
}

func TestDistributionRejectsBadInputs(t *testing.T) {
	in := baselineInput()

	if _, err := EvaluateDistribution(in, nil); err == nil {
		t.Error("empty distribution should be rejected")
	}
	if _, err := EvaluateDistribution(in, []DistributionBin{{Diameter: 1e-6, Weight: 0}}); err == nil {
		t.Error("zero total weight should be rejected")
	}
	if _, err := EvaluateDistribution(in, []DistributionBin{{Diameter: 0, Weight: 1}}); err == nil {
		t.Error("zero diameter bin should be rejected")
	}
	if _, err := EvaluateDistribution(in, []DistributionBin{{Diameter: -1e-6, Weight: 1}}); err == nil {
		t.Error("negative diameter bin should be rejected")
	}
	if _, err := EvaluateDistribution(in, []DistributionBin{{Diameter: 1e-6, Weight: -0.5}}); err == nil {
		t.Error("negative weight bin should be rejected")
	}
	// A single valid bin is accepted and equals its grade efficiency.
	res, err := EvaluateDistribution(in, []DistributionBin{{Diameter: 5e-6, Weight: 0.42}})
	if err != nil {
		t.Fatalf("single valid bin rejected: %v", err)
	}
	if math.Abs(res.OverallEfficiency-res.Bins[0].CollectionEfficiency) > 1e-12 {
		t.Errorf("single-bin overall eta %.8f != bin eta %.8f", res.OverallEfficiency, res.Bins[0].CollectionEfficiency)
	}
}

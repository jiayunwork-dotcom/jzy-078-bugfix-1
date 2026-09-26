package cyclone

import (
	"math"
	"testing"
)

// A representative operating point well inside the Stokes regime.
func baselineInput() BaseInput {
	return BaseInput{
		CylinderDiameter: 1.0,  // m
		InletVelocity:    20.0, // m/s
		GasDensity:       1.2,  // kg/m^3
		SolidDensity:     2000, // kg/m^3
		GasViscosity:     1.8e-5,
	}
}

func relErr(got, want float64) float64 {
	return math.Abs(got-want) / want
}

// cutDiameterFor is a thin wrapper so tests never touch package internals.
func cutDiameterFor(in BaseInput) float64 {
	r, err := EvaluateCutPoint(in)
	if err != nil {
		panic("unexpected cut point error: " + err.Error())
	}
	return r.CutDiameter50
}

func TestCutDiameterReferenceValue(t *testing.T) {
	r, err := EvaluateCutPoint(baselineInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// d50 = sqrt(9*mu*b/(2*pi*N*(rho_p-rho_g)*Vi)) ≈ 8.031 µm for this point.
	const want = 8.031e-6
	if e := relErr(r.CutDiameter50, want); e > 0.01 {
		t.Errorf("d50 = %.4e m, want %.4e (rel err %.2f%%)", r.CutDiameter50, want, 100*e)
	}
	if !r.StokesValid || r.Regime != RegimeStokes {
		t.Errorf("baseline should be Stokes regime, got regime=%s warnings=%v", r.Regime, r.Warnings)
	}
	if len(r.Warnings) != 0 {
		t.Errorf("baseline should carry no warnings, got %v", r.Warnings)
	}
	if math.Abs(r.CutDiameter50Um-r.CutDiameter50*1e6) > 1e-9 {
		t.Errorf("micron conversion inconsistent: %v vs %v", r.CutDiameter50Um, r.CutDiameter50*1e6)
	}
}

// Doubling inlet velocity alone must give d50' = d50 / sqrt(2) — an inverse
// square-root law, never a linear one.
func TestCutDiameterVelocityInverseSquareRoot(t *testing.T) {
	base := baselineInput()
	fast := base
	fast.InletVelocity *= 2

	ratio := cutDiameterFor(fast) / cutDiameterFor(base)
	const invSqrt2 = 1.0 / math.Sqrt2
	if e := relErr(ratio, invSqrt2); e > 0.02 {
		t.Errorf("2x velocity: d50 ratio = %.4f, want %.4f ±2%% (rel err %.2f%%)", ratio, invSqrt2, 100*e)
	}
	// A linear inverse law would give 0.5; the sqrt law gives ~0.707.
	if math.Abs(ratio-0.5) < 0.05 {
		t.Errorf("d50 scales linearly with velocity (ratio %.4f); expected inverse square-root", ratio)
	}

	// Halving velocity must coarsen the cut, again by sqrt(2).
	slow := base
	slow.InletVelocity /= 2
	slowRatio := cutDiameterFor(slow) / cutDiameterFor(base)
	if e := relErr(slowRatio, math.Sqrt2); e > 0.02 {
		t.Errorf("0.5x velocity: d50 ratio = %.4f, want %.4f ±2%%", slowRatio, math.Sqrt2)
	}
}

// Doubling the gas-solid density difference alone must give d50' = d50/sqrt(2).
func TestCutDiameterDensityDifferenceInverseSquareRoot(t *testing.T) {
	base := baselineInput()
	heavy := base
	// baseline diff = 2000-1.2 = 1998.8; double it to 3997.6.
	heavy.SolidDensity = base.GasDensity + 2*(base.SolidDensity-base.GasDensity)

	ratio := cutDiameterFor(heavy) / cutDiameterFor(base)
	const invSqrt2 = 1.0 / math.Sqrt2
	if e := relErr(ratio, invSqrt2); e > 0.02 {
		t.Errorf("2x density difference: d50 ratio = %.4f, want %.4f ±2%% (rel err %.2f%%)", ratio, invSqrt2, 100*e)
	}
	if math.Abs(ratio-0.5) < 0.05 {
		t.Errorf("d50 scales linearly with density difference (ratio %.4f)", ratio)
	}
}

// Doubling cylinder diameter with unchanged Stairmand proportions and inlet
// velocity must coarsen d50 (b scales with D, so d50 ∝ sqrt(D)).
func TestCutDiameterCoarsensWithDiameter(t *testing.T) {
	base := baselineInput()
	big := base
	big.CylinderDiameter *= 2

	small, large := cutDiameterFor(base), cutDiameterFor(big)
	if !(large > small) {
		t.Errorf("doubling diameter: d50 went from %.3e to %.3e (must coarsen)", small, large)
	}
	if e := relErr(large/small, math.Sqrt2); e > 0.02 {
		t.Errorf("2x diameter: d50 ratio = %.4f, want sqrt(2) ±2%%", large/small)
	}
}

// Doubling gas viscosity must coarsen d50 (d50 ∝ sqrt(mu)).
func TestCutDiameterCoarsensWithViscosity(t *testing.T) {
	base := baselineInput()
	thick := base
	thick.GasViscosity *= 2

	thin, thickD50 := cutDiameterFor(base), cutDiameterFor(thick)
	if !(thickD50 > thin) {
		t.Errorf("doubling viscosity: d50 went from %.3e to %.3e (must coarsen)", thin, thickD50)
	}
	if e := relErr(thickD50/thin, math.Sqrt2); e > 0.02 {
		t.Errorf("2x viscosity: d50 ratio = %.4f, want sqrt(2) ±2%%", thickD50/thin)
	}
}

func TestCutDiameterAtD50IsFiftyPercent(t *testing.T) {
	r, err := EvaluateCutPoint(baselineInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	eta := gradeEfficiency(r.CutDiameter50, r.CutDiameter50)
	if math.Abs(eta-0.5) > 1e-12 {
		t.Errorf("grade efficiency at d50 = %.6f, want 0.5", eta)
	}
}

// Outside the Stokes regime the service must warn instead of pretending the
// formula is accurate. Re_p at d50 grows as D^1.5, so a very large barrel
// drives the point into the transitional regime.
func TestReynoldsWarningOutsideStokes(t *testing.T) {
	in := baselineInput()
	in.CylinderDiameter = 100.0

	r, err := EvaluateCutPoint(in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.StokesValid {
		t.Errorf("expected Stokes assumption invalid at Re=%.3g, got valid", r.ParticleReynolds)
	}
	if r.Regime != RegimeTransitional {
		t.Errorf("expected transitional regime, got %q", r.Regime)
	}
	if r.ParticleReynolds <= reynoldsStokesLimit || r.ParticleReynolds > reynoldsNewtonLimit {
		t.Errorf("Re=%.3g outside intended transitional band", r.ParticleReynolds)
	}
	if len(r.Warnings) == 0 {
		t.Error("expected a regime warning, got none")
	}

	in.CylinderDiameter = 10000.0
	r, _ = EvaluateCutPoint(in)
	if r.Regime != RegimeNewton {
		t.Errorf("expected newton regime for D=10000m, got %q (Re=%.3g)", r.Regime, r.ParticleReynolds)
	}
}

func TestCutPointRejectsInvalidInputs(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*BaseInput)
		field  string
	}{
		{"zero diameter", func(i *BaseInput) { i.CylinderDiameter = 0 }, "cylinder_diameter"},
		{"negative diameter", func(i *BaseInput) { i.CylinderDiameter = -0.5 }, "cylinder_diameter"},
		{"zero velocity", func(i *BaseInput) { i.InletVelocity = 0 }, "inlet_velocity"},
		{"negative velocity", func(i *BaseInput) { i.InletVelocity = -3 }, "inlet_velocity"},
		{"solid equal to gas", func(i *BaseInput) { i.SolidDensity = i.GasDensity }, "solid_density"},
		{"solid lighter than gas", func(i *BaseInput) { i.SolidDensity = 0.5 }, "solid_density"},
		{"zero viscosity", func(i *BaseInput) { i.GasViscosity = 0 }, "gas_viscosity"},
		{"negative viscosity", func(i *BaseInput) { i.GasViscosity = -1e-5 }, "gas_viscosity"},
		{"zero gas density", func(i *BaseInput) { i.GasDensity = 0 }, "gas_density"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := baselineInput()
			tc.mutate(&in)
			_, err := EvaluateCutPoint(in)
			if err == nil {
				t.Fatalf("expected rejection for %s", tc.name)
			}
			if err.Field != tc.field {
				t.Errorf("error field = %q, want %q", err.Field, tc.field)
			}
		})
	}
}

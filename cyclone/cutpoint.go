package cyclone

import (
	"math"
	"strconv"
)

// This file defines the cut diameter correlation once. Both the single-point
// grade efficiency and the distribution integration obtain d50 exclusively
// through EvaluateCutPoint, so the formula can never drift between callers.
//
// Stairmand cut diameter (Stokes regime):
//
//	d50 = sqrt( 9 * mu * b / (2 * pi * N * (rho_p - rho_g) * Vi) )
//
// where b is the inlet height and N is the effective number of vortex turns.
// Both b and N come from the Stairmand geometry module.
const (
	// Stokes law is strictly valid for particle Reynolds numbers up to ~1;
	// between 1 and 1000 the transitional (Allen) regime applies and beyond
	// ~1000 the Newton (turbulent) regime.
	reynoldsStokesLimit = 1.0
	reynoldsNewtonLimit = 1000.0

	gravitationalAcceleration = 9.81 // m/s^2, used for the terminal-settling regime check
)

// Regime labels reported to clients.
const (
	RegimeStokes       = "stokes"
	RegimeTransitional = "transitional"
	RegimeNewton       = "newton"
)

// CutPointResult reports the cut diameter together with the regime check.
type CutPointResult struct {
	CutDiameter50    float64  `json:"cut_diameter_d50_m"`
	CutDiameter50Um  float64  `json:"cut_diameter_d50_um"`
	ParticleReynolds float64  `json:"particle_reynolds"`
	Regime           string   `json:"regime"`
	StokesValid      bool     `json:"stokes_assumption_valid"`
	Warnings         []string `json:"warnings"`
	Geometry         Geometry `json:"geometry"`
}

// EvaluateCutPoint fixes the Stairmand geometry from the cylinder diameter and
// computes d50 plus the particle Reynolds number at d50. Warnings are returned
// (not silently swallowed) whenever the operating point leaves the Stokes
// regime, because the cut formula then loses accuracy.
func EvaluateCutPoint(in BaseInput) (CutPointResult, *APIError) {
	if e := in.Validate(); e != nil {
		return CutPointResult{}, e
	}
	geom, e := CalculateGeometry(in.CylinderDiameter)
	if e != nil {
		return CutPointResult{}, e
	}

	d50 := cutDiameter(in, geom)
	re := particleReynolds(in, d50)
	regime, valid, warnings := classifyReynolds(re)

	return CutPointResult{
		CutDiameter50:    d50,
		CutDiameter50Um:  d50 * 1e6,
		ParticleReynolds: re,
		Regime:           regime,
		StokesValid:      valid,
		Warnings:         warnings,
		Geometry:         geom,
	}, nil
}

// cutDiameter is the sole implementation of the Stairmand cut correlation.
func cutDiameter(in BaseInput, g Geometry) float64 {
	densityDiff := in.SolidDensity - in.GasDensity
	num := 9.0 * in.GasViscosity * g.InletHeight
	den := 2.0 * math.Pi * stairmandEffectiveTurns * densityDiff * in.InletVelocity
	return math.Sqrt(num / den)
}

// particleReynolds evaluates Re_p = rho_g * v_t * d / mu using the Stokes
// gravitational terminal velocity of the particle, v_t = d^2 (rho_p-rho_g) g /
// (18 mu). It is the standard regime check attached to the cut correlation:
// Re_p > 1 means Stokes drag (and hence the d50 formula) is an approximation.
func particleReynolds(in BaseInput, d float64) float64 {
	densityDiff := in.SolidDensity - in.GasDensity
	terminal := d * d * densityDiff * gravitationalAcceleration / (18.0 * in.GasViscosity)
	return in.GasDensity * terminal * d / in.GasViscosity
}

// classifyReynolds always returns a freshly allocated warning list: callers
// may append their own warnings to it, so no slice (or its backing array) may
// ever be shared between calls — that would leak one request's warnings into
// another concurrent request. The empty Stokes list is non-nil so clients
// always receive [] rather than null.
func classifyReynolds(re float64) (regime string, stokesValid bool, warnings []string) {
	switch {
	case re <= reynoldsStokesLimit:
		return RegimeStokes, true, make([]string, 0)
	case re <= reynoldsNewtonLimit:
		warnings = []string{
			"particle Reynolds number at d50 is " + trimFloat(re) +
				" (>1); flow is in the transitional (Allen) regime and the Stokes-based d50 is approximate",
		}
		return RegimeTransitional, false, warnings
	default:
		warnings = []string{
			"particle Reynolds number at d50 is " + trimFloat(re) +
				" (>1000); flow is in the Newton regime and the Stokes-based d50 is unreliable",
		}
		return RegimeNewton, false, warnings
	}
}

func trimFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', 4, 64)
}

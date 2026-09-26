// Package cyclone implements cyclone separator sizing calculations using
// Stairmand's standard geometric proportions and the Stairmand cut diameter
// correlation in the Stokes flow regime.
package cyclone

import "math"

// BaseInput holds the basic process parameters for every cyclone calculation.
// All quantities are SI units:
//
//	CylinderDiameter m
//	InletVelocity    m/s
//	GasDensity       kg/m^3
//	SolidDensity     kg/m^3
//	GasViscosity     Pa·s (kg/(m·s))
type BaseInput struct {
	CylinderDiameter float64 `json:"cylinder_diameter"`
	InletVelocity    float64 `json:"inlet_velocity"`
	GasDensity       float64 `json:"gas_density"`
	SolidDensity     float64 `json:"solid_density"`
	GasViscosity     float64 `json:"gas_viscosity"`
}

// Validate checks the physical validity of the basic parameters.
func (in BaseInput) Validate() *APIError {
	switch {
	case !isFinite(in.CylinderDiameter):
		return newError(ErrInvalidNumber, "cylinder_diameter", "cylinder diameter must be a finite number")
	case in.CylinderDiameter <= 0:
		return newError(ErrInvalidDiameter, "cylinder_diameter", "cylinder diameter must be greater than zero")
	case !isFinite(in.InletVelocity):
		return newError(ErrInvalidNumber, "inlet_velocity", "inlet velocity must be a finite number")
	case in.InletVelocity <= 0:
		return newError(ErrInvalidVelocity, "inlet_velocity", "inlet velocity must be greater than zero")
	case !isFinite(in.GasDensity) || !isFinite(in.SolidDensity):
		return newError(ErrInvalidNumber, "gas_density", "densities must be finite numbers")
	case in.GasDensity <= 0:
		return newError(ErrInvalidDensity, "gas_density", "gas density must be greater than zero")
	// Reject inverted density order: a solid denser than the carrier gas is
	// the only configuration a centrifugal separator can collect.
	case in.SolidDensity <= in.GasDensity:
		return newError(ErrInvalidDensity, "solid_density", "solid density must be greater than gas density")
	case !isFinite(in.GasViscosity):
		return newError(ErrInvalidNumber, "gas_viscosity", "gas viscosity must be a finite number")
	case in.GasViscosity <= 0:
		return newError(ErrInvalidViscosity, "gas_viscosity", "gas viscosity must be greater than zero")
	}
	return nil
}

// Error codes returned by the calculation layer.
const (
	ErrInvalidRequest   = "INVALID_REQUEST"
	ErrInvalidNumber    = "INVALID_NUMBER"
	ErrInvalidDiameter  = "INVALID_DIAMETER"
	ErrInvalidVelocity  = "INVALID_VELOCITY"
	ErrInvalidDensity   = "INVALID_DENSITY"
	ErrInvalidViscosity = "INVALID_VISCOSITY"
	ErrInvalidParticle  = "INVALID_PARTICLE"
	ErrInvalidDist      = "INVALID_DISTRIBUTION"
)

// APIError is the structured error description returned for every rejected
// request. Code is machine-readable, Field identifies the offending input and
// Message carries a human-readable explanation.
type APIError struct {
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	if e.Field != "" {
		return e.Code + ": " + e.Field + ": " + e.Message
	}
	return e.Code + ": " + e.Message
}

func newError(code, field, message string) *APIError {
	return &APIError{Code: code, Field: field, Message: message}
}

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

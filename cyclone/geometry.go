package cyclone

// This file is the single source of truth for Stairmand's standard (high
// efficiency) cyclone geometry. Every ratio is expressed relative to the
// cylinder diameter D. The cut diameter and efficiency calculations must not
// re-declare any of these proportions.
//
// Stairmand (1951) high-efficiency proportions:
//
//	inlet width  a/D = 0.20
//	inlet height b/D = 0.50
//	vortex finder (outlet) diameter De/D = 0.50
//	outlet duct length (protrusion) S/D = 0.50
//	barrel (cylindrical section) height h/D = 1.50
//	total height (barrel + cone) H/D = 4.00
//	dust outlet (apex) diameter B/D = 0.375
//
// The effective number of outer-vortex turns for Stairmand geometry is N=5.
const (
	stairmandInletWidthRatio  = 0.20  // a/D
	stairmandInletHeightRatio = 0.50  // b/D
	stairmandOutletRatio      = 0.50  // De/D (vortex finder)
	stairmandOutletLength     = 0.50  // S/D
	stairmandBarrelRatio      = 1.50  // h/D
	stairmandTotalHeightRatio = 4.00  // H/D
	stairmandDustOutletRatio  = 0.375 // B/D
	stairmandEffectiveTurns   = 5.0   // N
)

// Ratios reports the dimensionless Stairmand proportions (all relative to D).
type Ratios struct {
	InletWidth  float64 `json:"inlet_width"`  // a/D
	InletHeight float64 `json:"inlet_height"` // b/D
	Outlet      float64 `json:"outlet"`       // De/D
	OutletLen   float64 `json:"outlet_len"`   // S/D
	Barrel      float64 `json:"barrel"`       // h/D
	TotalHeight float64 `json:"total_height"` // H/D
	DustOutlet  float64 `json:"dust_outlet"`  // B/D
	Cone        float64 `json:"cone"`         // (H-h)/D
	Turns       float64 `json:"turns"`        // N
}

// Geometry holds the absolute dimensions of a Stairmand-proportioned cyclone.
// Lengths are in metres, the inlet area in m^2.
type Geometry struct {
	CylinderDiameter float64 `json:"cylinder_diameter"`
	InletWidth       float64 `json:"inlet_width"`
	InletHeight      float64 `json:"inlet_height"`
	InletArea        float64 `json:"inlet_area"`
	OutletDiameter   float64 `json:"outlet_diameter"`
	OutletLength     float64 `json:"outlet_length"`
	BarrelLength     float64 `json:"barrel_length"`
	TotalHeight      float64 `json:"total_height"`
	ConeLength       float64 `json:"cone_length"`
	DustOutletDiam   float64 `json:"dust_outlet_diameter"`
	Ratios           Ratios  `json:"ratios"`
}

// GeometryResult is the payload of the geometry endpoint.
type GeometryResult struct {
	Geometry Geometry `json:"geometry"`
}

// CalculateGeometry derives every dimension from the cylinder diameter using
// the Stairmand proportions above.
func CalculateGeometry(diameter float64) (Geometry, *APIError) {
	if !isFinite(diameter) {
		return Geometry{}, newError(ErrInvalidNumber, "cylinder_diameter", "cylinder diameter must be a finite number")
	}
	if diameter <= 0 {
		return Geometry{}, newError(ErrInvalidDiameter, "cylinder_diameter", "cylinder diameter must be greater than zero")
	}

	a := stairmandInletWidthRatio * diameter
	b := stairmandInletHeightRatio * diameter
	g := Geometry{
		CylinderDiameter: diameter,
		InletWidth:       a,
		InletHeight:      b,
		InletArea:        a * b,
		OutletDiameter:   stairmandOutletRatio * diameter,
		OutletLength:     stairmandOutletLength * diameter,
		BarrelLength:     stairmandBarrelRatio * diameter,
		TotalHeight:      stairmandTotalHeightRatio * diameter,
		DustOutletDiam:   stairmandDustOutletRatio * diameter,
		Ratios: Ratios{
			InletWidth:  stairmandInletWidthRatio,
			InletHeight: stairmandInletHeightRatio,
			Outlet:      stairmandOutletRatio,
			OutletLen:   stairmandOutletLength,
			Barrel:      stairmandBarrelRatio,
			TotalHeight: stairmandTotalHeightRatio,
			DustOutlet:  stairmandDustOutletRatio,
			Cone:        stairmandTotalHeightRatio - stairmandBarrelRatio,
			Turns:       stairmandEffectiveTurns,
		},
	}
	g.ConeLength = g.TotalHeight - g.BarrelLength
	return g, nil
}

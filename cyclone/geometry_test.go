package cyclone

import (
	"math"
	"testing"
)

func TestStairmandRatios(t *testing.T) {
	g, err := CalculateGeometry(1.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := []struct {
		name string
		got  float64
		want float64
	}{
		{"inlet width a/D", g.Ratios.InletWidth, 0.20},
		{"inlet height b/D", g.Ratios.InletHeight, 0.50},
		{"outlet De/D", g.Ratios.Outlet, 0.50},
		{"outlet length S/D", g.Ratios.OutletLen, 0.50},
		{"barrel h/D", g.Ratios.Barrel, 1.50},
		{"total height H/D", g.Ratios.TotalHeight, 4.00},
		{"dust outlet B/D", g.Ratios.DustOutlet, 0.375},
		{"cone (H-h)/D", g.Ratios.Cone, 2.50},
		{"effective turns N", g.Ratios.Turns, 5.0},
	}
	for _, tc := range cases {
		if math.Abs(tc.got-tc.want) > 1e-12 {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}

	// Absolute dimensions for D=1 m.
	if g.InletWidth != 0.20 {
		t.Errorf("inlet width = %v, want 0.20 m", g.InletWidth)
	}
	if g.InletHeight != 0.50 {
		t.Errorf("inlet height = %v, want 0.50 m", g.InletHeight)
	}
	if g.InletArea != 0.20*0.50 {
		t.Errorf("inlet area = %v, want %v", g.InletArea, 0.20*0.50)
	}
	if g.ConeLength != g.TotalHeight-g.BarrelLength {
		t.Errorf("cone length %v != total %v - barrel %v", g.ConeLength, g.TotalHeight, g.BarrelLength)
	}
}

func TestGeometryScalesLinearlyWithDiameter(t *testing.T) {
	small, _ := CalculateGeometry(0.5)
	big, _ := CalculateGeometry(2.0)

	linear := []struct {
		name string
		a, b float64
	}{
		{"inlet width", small.InletWidth, big.InletWidth},
		{"inlet height", small.InletHeight, big.InletHeight},
		{"inlet area", small.InletArea, big.InletArea},
		{"outlet diameter", small.OutletDiameter, big.OutletDiameter},
		{"barrel length", small.BarrelLength, big.BarrelLength},
		{"total height", small.TotalHeight, big.TotalHeight},
		{"dust outlet", small.DustOutletDiam, big.DustOutletDiam},
	}
	const diameterRatio = 2.0 / 0.5
	for _, tc := range linear {
		wantRatio := diameterRatio
		if tc.name == "inlet area" {
			wantRatio = diameterRatio * diameterRatio // area scales with D^2
		}
		if got := tc.b / tc.a; math.Abs(got-wantRatio) > 1e-12 {
			t.Errorf("%s ratio = %v, want %v", tc.name, got, wantRatio)
		}
	}

	// Ratios are independent of diameter.
	if big.Ratios != small.Ratios {
		t.Errorf("dimensionless ratios changed with diameter: %+v vs %+v", big.Ratios, small.Ratios)
	}
}

func TestGeometryRejectsInvalidDiameter(t *testing.T) {
	for _, d := range []float64{0, -1.0, math.NaN(), math.Inf(1)} {
		if _, err := CalculateGeometry(d); err == nil {
			t.Errorf("CalculateGeometry(%v) expected error, got nil", d)
		}
	}
}

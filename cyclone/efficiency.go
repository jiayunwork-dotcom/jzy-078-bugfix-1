package cyclone

import "math"

// Grade efficiency follows the empirical Lapple cut-size curve, expressed
// relative to the Stairmand d50:
//
//	eta(d) = 1 / (1 + (d50/d)^2)
//
// Properties the endpoint relies on:
//   - eta is strictly increasing in d, from 0 for vanishingly fine dust to 1
//     for very coarse dust;
//   - eta(d50) = 0.5, which is exactly what a "cut" diameter means;
//   - d50 is obtained solely from the cut point module — no geometry constant
//     is duplicated here.
const gradeEfficiencyExponent = 2.0

// GradeResult is the single-particle grade efficiency answer.
type GradeResult struct {
	ParticleDiameter     float64  `json:"particle_diameter_m"`
	ParticleDiameterUm   float64  `json:"particle_diameter_um"`
	CutDiameter50        float64  `json:"cut_diameter_d50_m"`
	CollectionEfficiency float64  `json:"collection_efficiency"`
	Penetration          float64  `json:"penetration"`
	ParticleReynolds     float64  `json:"particle_reynolds"`
	StokesValid          bool     `json:"stokes_assumption_valid"`
	Warnings             []string `json:"warnings"`
}

// EvaluateGrade computes collection efficiency and penetration for one size.
func EvaluateGrade(in BaseInput, particleDiameter float64) (GradeResult, *APIError) {
	cp, e := EvaluateCutPoint(in)
	if e != nil {
		return GradeResult{}, e
	}
	if !isFinite(particleDiameter) {
		return GradeResult{}, newError(ErrInvalidNumber, "particle_diameter", "particle diameter must be a finite number")
	}
	if particleDiameter <= 0 {
		return GradeResult{}, newError(ErrInvalidParticle, "particle_diameter", "particle diameter must be greater than zero")
	}

	eta := gradeEfficiency(particleDiameter, cp.CutDiameter50)
	re := particleReynolds(in, particleDiameter)
	_, stokesValid, _ := classifyReynolds(re)

	return GradeResult{
		ParticleDiameter:     particleDiameter,
		ParticleDiameterUm:   particleDiameter * 1e6,
		CutDiameter50:        cp.CutDiameter50,
		CollectionEfficiency: eta,
		Penetration:          1.0 - eta,
		ParticleReynolds:     re,
		StokesValid:          stokesValid,
		Warnings:             cp.Warnings,
	}, nil
}

// gradeEfficiency is the sole implementation of the empirical grade curve.
func gradeEfficiency(d, d50 float64) float64 {
	ratio := d50 / d
	return 1.0 / (1.0 + math.Pow(ratio, gradeEfficiencyExponent))
}

// DistributionBin is one size class of the feed: particles of diameter
// Diameter carrying (mass or number) fraction Weight.
type DistributionBin struct {
	Diameter float64 `json:"diameter_m"`
	Weight   float64 `json:"weight"`
}

// BinEfficiency is the per-size result inside a distribution integration.
type BinEfficiency struct {
	Diameter             float64 `json:"diameter_m"`
	DiameterUm           float64 `json:"diameter_um"`
	Weight               float64 `json:"weight"`
	NormalizedWeight     float64 `json:"normalized_weight"`
	CollectionEfficiency float64 `json:"collection_efficiency"`
	Penetration          float64 `json:"penetration"`
}

// DistributionResult is the overall grade efficiency obtained by integrating
// (weighted-summing) the grade curve over a particle size distribution.
type DistributionResult struct {
	CutDiameter50      float64         `json:"cut_diameter_d50_m"`
	CutDiameter50Um    float64         `json:"cut_diameter_d50_um"`
	OverallEfficiency  float64         `json:"overall_efficiency"`
	OverallPenetration float64         `json:"overall_penetration"`
	WeightSum          float64         `json:"weight_sum"`
	NormalizedWeights  bool            `json:"weights_normalized"`
	Bins               []BinEfficiency `json:"bins"`
	Warnings           []string        `json:"warnings"`
}

const distributionWeightTolerance = 0.01

// EvaluateDistribution integrates the grade curve over the supplied size
// distribution: eta_overall = sum(w_i * eta(d_i)) / sum(w_i). Weights need not
// be pre-normalized; a non-unit total only triggers a warning, while a
// non-positive or empty total is rejected.
func EvaluateDistribution(in BaseInput, bins []DistributionBin) (DistributionResult, *APIError) {
	cp, e := EvaluateCutPoint(in)
	if e != nil {
		return DistributionResult{}, e
	}
	if len(bins) == 0 {
		return DistributionResult{}, newError(ErrInvalidDist, "bins", "particle size distribution must contain at least one size class")
	}

	var weightSum float64
	for _, bin := range bins {
		if !isFinite(bin.Diameter) {
			return DistributionResult{}, newError(ErrInvalidNumber, "bins.diameter_m", "particle diameter must be a finite number")
		}
		if bin.Diameter <= 0 {
			return DistributionResult{}, newError(ErrInvalidParticle, "bins.diameter_m", "every particle diameter must be greater than zero")
		}
		if !isFinite(bin.Weight) || bin.Weight < 0 {
			return DistributionResult{}, newError(ErrInvalidDist, "bins.weight", "every bin weight must be a finite non-negative number")
		}
		weightSum += bin.Weight
	}
	if weightSum <= 0 {
		return DistributionResult{}, newError(ErrInvalidDist, "bins.weight", "sum of bin weights must be greater than zero")
	}

	// Copy the cut-point warnings into a list this result owns: appending the
	// normalization warning must never write into a slice (or backing array)
	// shared with other in-flight requests.
	warnings := make([]string, 0, len(cp.Warnings)+1)
	warnings = append(warnings, cp.Warnings...)

	result := DistributionResult{
		CutDiameter50:   cp.CutDiameter50,
		CutDiameter50Um: cp.CutDiameter50Um,
		WeightSum:       weightSum,
		Bins:            make([]BinEfficiency, 0, len(bins)),
		Warnings:        warnings,
	}
	if math.Abs(weightSum-1.0) > distributionWeightTolerance {
		result.Warnings = append(result.Warnings,
			"bin weights sum to "+trimFloat(weightSum)+", not 1; results were computed after normalization")
	} else {
		result.NormalizedWeights = true
	}

	var weighted float64
	for _, bin := range bins {
		eta := gradeEfficiency(bin.Diameter, cp.CutDiameter50)
		normalized := bin.Weight / weightSum
		weighted += normalized * eta
		result.Bins = append(result.Bins, BinEfficiency{
			Diameter:             bin.Diameter,
			DiameterUm:           bin.Diameter * 1e6,
			Weight:               bin.Weight,
			NormalizedWeight:     normalized,
			CollectionEfficiency: eta,
			Penetration:          1.0 - eta,
		})
	}
	result.OverallEfficiency = weighted
	result.OverallPenetration = 1.0 - weighted
	return result, nil
}

package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"cycloneservice/cyclone"
)

// Handler exposes the cyclone calculations over HTTP. It is stateless.
type Handler struct{}

// New returns a Handler.
func New() *Handler { return &Handler{} }

// Register mounts every cyclone route on the given engine.
func (h *Handler) Register(r *gin.Engine) {
	r.GET("/healthz", h.health)

	v1 := r.Group("/api/v1")
	{
		v1.POST("/cyclone/geometry", h.geometry)
		v1.POST("/cyclone/cut-point", h.cutPoint)
		v1.POST("/cyclone/grade-efficiency", h.gradeEfficiency)
		v1.POST("/cyclone/distribution-efficiency", h.distributionEfficiency)
	}
}

func (h *Handler) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "cyclone-sizing"})
}

func (h *Handler) geometry(c *gin.Context) {
	var req struct {
		CylinderDiameter float64 `json:"cylinder_diameter"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeBindError(c, err)
		return
	}

	g, apiErr := cyclone.CalculateGeometry(req.CylinderDiameter)
	if apiErr != nil {
		writeAPIError(c, apiErr)
		return
	}
	c.JSON(http.StatusOK, cyclone.GeometryResult{Geometry: g})
}

func (h *Handler) cutPoint(c *gin.Context) {
	var req cyclone.BaseInput
	if err := c.ShouldBindJSON(&req); err != nil {
		writeBindError(c, err)
		return
	}

	result, apiErr := cyclone.EvaluateCutPoint(req)
	if apiErr != nil {
		writeAPIError(c, apiErr)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) gradeEfficiency(c *gin.Context) {
	var req struct {
		cyclone.BaseInput
		ParticleDiameter float64 `json:"particle_diameter_m"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeBindError(c, err)
		return
	}

	result, apiErr := cyclone.EvaluateGrade(req.BaseInput, req.ParticleDiameter)
	if apiErr != nil {
		writeAPIError(c, apiErr)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *Handler) distributionEfficiency(c *gin.Context) {
	var req struct {
		cyclone.BaseInput
		Bins []cyclone.DistributionBin `json:"bins"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		writeBindError(c, err)
		return
	}

	result, apiErr := cyclone.EvaluateDistribution(req.BaseInput, req.Bins)
	if apiErr != nil {
		writeAPIError(c, apiErr)
		return
	}
	c.JSON(http.StatusOK, result)
}

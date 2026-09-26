package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"cycloneservice/cyclone"
)

// NewRouter builds the Gin engine with all cyclone routes registered.
func NewRouter() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	New().Register(r)

	// Structured 404/405 instead of Gin's default HTML.
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"code":    cyclone.ErrInvalidRequest,
				"message": "route not found",
			},
		})
	})
	return r
}

func writeAPIError(c *gin.Context, e *cyclone.APIError) {
	// Every rejected input is a client error: malformed JSON/semantics -> 400,
	// physically invalid parameters -> 422.
	status := http.StatusUnprocessableEntity
	if e.Code == cyclone.ErrInvalidRequest {
		status = http.StatusBadRequest
	}
	c.JSON(status, gin.H{"error": e})
}

func writeBindError(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, gin.H{
		"error": &cyclone.APIError{
			Code:    cyclone.ErrInvalidRequest,
			Message: "request body could not be parsed: " + err.Error(),
		},
	})
}

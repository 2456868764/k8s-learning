package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

)

func Anything(c *gin.Context) {
	response := NewResponseFromContext(c)
	c.JSON(http.StatusOK, response)
}

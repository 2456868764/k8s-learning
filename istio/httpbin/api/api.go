package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func Anything(c *gin.Context) {
	response := NewResponseFromContext(c)
	c.JSON(http.StatusOK, response)
}

func HostName(c *gin.Context) {
	response := getHostName()
	c.JSON(http.StatusOK, response)
}

func Headers(c *gin.Context) {
	headers := c.Request.Header
	response := make(map[string]string, len(headers))
	for hk, hv := range headers {
		response[hk] = strings.Join(hv, ",")
	}
	c.JSON(http.StatusOK, response)
}

func Healthz(c *gin.Context) {
	if FileExisted("./healthz") {
		c.JSON(http.StatusOK, "healthz")
		return
	}

	c.JSON(http.StatusNotFound, "not healthz")
}
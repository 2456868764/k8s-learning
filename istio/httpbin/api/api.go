package api

import (
	"fmt"
	"io"
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

func Bool(c *gin.Context) {
	c.JSON(http.StatusCreated, true)
}

func ReponseAnyDto(c *gin.Context) {
	c.JSON(http.StatusOK, ResponseAny{Code: 1, Data: ConditionRouteDto{}})
}

func ReponseAnyArray(c *gin.Context) {
	c.JSON(http.StatusOK, ResponseAny{Code: 1, Data: []ConditionRouteDto{ConditionRouteDto{}}})
}

func ReponseAnyString(c *gin.Context) {
	c.JSON(http.StatusOK, ResponseAny{Code: 1, Data: "hello"})
}

func Service(c *gin.Context) {
	nextServices := c.Query("services")
	if len(nextServices) == 0 {
		response := NewResponseFromContext(c)
		c.JSON(http.StatusOK, response)
		return
	}
	// Call next service
	// Pass headers
	headers := c.Request.Header
	services := strings.Split(nextServices, ",")
	nextUrl := ""
	if len(services) == 1 {
		nextUrl = "http://" + services[0] + "/"
	} else {
		nextUrl = "http://" + services[0] + "/service?services=" + strings.Join(services[1:], ",")
	}

	fmt.Printf("nextUrl:%s\n", nextUrl)
	req, err := http.NewRequest(c.Request.Method, nextUrl, c.Request.Body)
	if err != nil {
		fmt.Printf("%s", err)
	}
	lowerCaseHeader := make(http.Header)
	for key, value := range headers {
		headK := strings.ToLower(key)
		if headK == "method" || headK == "content-length" || headK == "host" {
			continue
		}
		lowerCaseHeader[strings.ToLower(key)] = value
	}
	// Add service trace header
	traceHeader, ok := lowerCaseHeader["x-service-trace"]
	if !ok {
		lowerCaseHeader["x-service-trace"] = []string{getHostName()}
	} else {
		lowerCaseHeader["x-service-trace"] = []string{traceHeader[0] + "/" + getHostName()}
	}

	req.Header = lowerCaseHeader
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("%s", err)
	}
	var bodyBytes []byte
	bodyBytes, _ = io.ReadAll(resp.Body)
	c.Header("Content-Type", "application/json")
	c.String(http.StatusOK, string(bodyBytes))
}

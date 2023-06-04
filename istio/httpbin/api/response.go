package api

import (
	"strings"

	"github.com/gin-gonic/gin"
	"httpbin/pkg/model"
	"httpbin/pkg/utils"
)

func NewResponseFromContext(c *gin.Context) model.Response {
	query := c.Request.URL.Query()
	headers := c.Request.Header
	form := c.Request.Form
	response := model.Response{
		Args:    make(map[string]string, len(query)),
		Headers: make(map[string]string, len(headers)),
		Envs:    make(map[string]string),
		Form:    make(map[string]string, len(form)),
	}
	response.Method = c.Request.Method
	response.Url = c.Request.URL.Path
	for qk, qv := range query {
		response.Args[qk] = strings.Join(qv, ",")
	}

	for hk, hv := range headers {
		response.Headers[strings.ToLower(hk)] = strings.Join(hv, ",")
	}

	// Add  trace header
	_, ok := response.Headers["x-service-trace"]
	if !ok {
		response.Headers["x-service-trace"] = utils.GetHostName()
	} else {
		response.Headers["x-service-trace"] = response.Headers["x-service-trace"] + "/" + utils.GetHostName()
	}

	for fk, fv := range form {
		response.Form[fk] = strings.Join(fv, ",")
	}

	response.Origin = c.Request.Header.Get("Origin")
	response.Envs = utils.GetAllEnvs()
	response.HostName = utils.GetHostName()

	var bodyBytes []byte // 我们需要的body内容
	// 从原有Request.Body读取
	bodyBytes, _ = c.GetRawData()
	response.Body = string(bodyBytes)
	return response
}

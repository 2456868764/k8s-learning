package api

import (
	"github.com/gin-gonic/gin"
	"os"
	"strings"
)

type Response struct {
	Args     map[string]string `json:"args""`
	Form     map[string]string `json:"form"`
	Headers  map[string]string `json:"headers"`
	Method   string            `json:"method"`
	Origin   string            `json:"origin"`
	Url      string            `json:"url"`
	Envs     map[string]string `json:"envs"`
	HostName string            `json:"host_name"`
}

func NewResponseFromContext(c *gin.Context) Response {
	query := c.Request.URL.Query()
	headers := c.Request.Header
	form := c.Request.Form
	response := Response{
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
		response.Headers[hk] = strings.Join(hv, ",")
	}

	for fk, fv := range form {
		response.Form[fk] = strings.Join(fv, ",")
	}

	response.Origin = c.Request.Header.Get("Origin")
	response.Envs = getAllEnvs()
	response.HostName = getHostName()
	return response
}

func getAllEnvs() map[string]string {
	allEnvs := make(map[string]string, 2)
	envs := os.Environ()
	for _, e := range envs {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			continue
		} else {
			allEnvs[parts[0]] = parts[1]
		}
	}
	return allEnvs
}

func getHostName() string {
	hostname, err := os.Hostname()
	if err != nil {
		return ""
	}
	return hostname
}

func FileExisted(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false
		}
		return true
	}
	return true

}

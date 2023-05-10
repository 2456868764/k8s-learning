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
	Body     string            `json:"body"`
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

	var bodyBytes []byte // 我们需要的body内容
	// 从原有Request.Body读取
	bodyBytes, _ = c.GetRawData()
	response.Body = string(bodyBytes)
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
	return getStringEnv("POD_NAME", getDefaultHostName())
}

func getDefaultHostName() string {
	hostname, err := os.Hostname()
	if err != nil {
		return ""
	}
	return hostname
}
func getStringEnv(name string, defvalue string) string {
	val, ex := os.LookupEnv(name)
	if ex {
		return val
	} else {
		return defvalue
	}
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

type ResponseAny struct {
	Code int `json:"code"`
	Data any `json:"data"`
}

type Base struct {
	Application    string `json:"application" yaml:"application"`
	Service        string `json:"service" yaml:"service"`
	ID             string `json:"id" yaml:"id"`
	ServiceVersion string `json:"serviceVersion" yaml:"serviceVersion"`
	ServiceGroup   string `json:"serviceGroup" yaml:"serviceGroup"`
}

type ConditionRouteDto struct {
	Base

	Conditions []string `json:"conditions" yaml:"conditions" binding:"required"`

	Priority      int    `json:"priority" yaml:"priority"`
	Enabled       bool   `json:"enabled" yaml:"enabled" binding:"required"`
	Force         bool   `json:"force" yaml:"force"`
	Runtime       bool   `json:"runtime" yaml:"runtime"`
	ConfigVersion string `json:"configVersion" yaml:"configVersion" binding:"required"`
}

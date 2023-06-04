package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/SkyAPM/go2sky"

	"k8s.io/klog/v2"

	v3 "skywalking.apache.org/repo/goapi/collect/language/agent/v3"

	"github.com/gin-gonic/gin"
	"httpbin/pkg/model"
	"httpbin/pkg/utils"
)

var defaultTraceHeaders = []string{
	"X-Ot-Span-Context", "X-Request-Id",
	"X-B3-TraceId", "X-B3-SpanId", "X-B3-ParentSpanId", "X-B3-Sampled", "X-B3-Flags",
	"uber-trace-id",
	"jwt", "Authorization",
}

func Anything(c *gin.Context) {
	response := NewResponseFromContext(c)
	c.JSON(http.StatusOK, response)
}

func HostName(c *gin.Context) {
	response := utils.GetHostName()
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
	c.JSON(http.StatusOK, "healthz")
	return
}

func HealthzFile(c *gin.Context) {
	if utils.FileExisted("./healthz.txt") {
		c.JSON(http.StatusOK, "ok")
		return
	}
	c.JSON(http.StatusNotFound, "not healthz")
}

func Readiness(c *gin.Context) {
	c.JSON(http.StatusOK, "readiness")
	return
}

func ReadinessFile(c *gin.Context) {
	if utils.FileExisted("./readiness.txt") {
		c.JSON(http.StatusOK, "ok")
		return
	}
	c.JSON(http.StatusNotFound, "not readiness")
}

func Startup(c *gin.Context) {
	c.JSON(http.StatusOK, "startup")
	return
}

func StartupFile(c *gin.Context) {
	if utils.FileExisted("./startup.txt") {
		c.JSON(http.StatusOK, "ok")
		return
	}
	c.JSON(http.StatusNotFound, "not startup")
}

func Bool(c *gin.Context) {
	c.JSON(http.StatusCreated, true)
}

func ReponseAnyDto(c *gin.Context) {
	c.JSON(http.StatusOK, model.ResponseAny{Code: 1, Data: model.ConditionRouteDto{}})
}

func ReponseAnyArray(c *gin.Context) {
	c.JSON(http.StatusOK, model.ResponseAny{Code: 1, Data: []model.ConditionRouteDto{{}}})
}

func ReponseAnyString(c *gin.Context) {
	c.JSON(http.StatusOK, model.ResponseAny{Code: 1, Data: "hello"})
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
	klog.Infof("service call nexturl:%s", nextUrl)
	req, err := http.NewRequest(c.Request.Method, nextUrl, c.Request.Body)
	if err != nil {
		klog.Error(err)
	}
	lowerCaseHeader := make(http.Header)
	for key, value := range headers {
		headK := strings.ToLower(key)
		for _, traceHeader := range defaultTraceHeaders {
			if headK == strings.ToLower(traceHeader) {
				lowerCaseHeader[strings.ToLower(key)] = value
			}
		}
	}
	// Add service trace header
	traceHeader, ok := lowerCaseHeader["x-service-trace"]
	if !ok {
		lowerCaseHeader["x-service-trace"] = []string{utils.GetHostName()}
	} else {
		lowerCaseHeader["x-service-trace"] = []string{traceHeader[0] + "/" + utils.GetHostName()}
	}

	req.Header = lowerCaseHeader
	// 出去必须用这个携带 header
	reqSpan, err := go2sky.GetGlobalTracer().CreateExitSpan(c.Request.Context(), "invoke", nextUrl, func(headerKey, headerValue string) error {
		req.Header.Set(headerKey, headerValue)
		return nil
	})
	reqSpan.SetComponent(2)
	reqSpan.SetSpanLayer(v3.SpanLayer_RPCFramework) // rpc 调用
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		klog.Error(err)
	}
	var bodyBytes []byte
	bodyBytes, _ = io.ReadAll(resp.Body)
	reqSpan.Tag(go2sky.TagHTTPMethod, http.MethodPost)
	reqSpan.Tag(go2sky.TagURL, nextUrl)
	reqSpan.Log(time.Now(), "[HttpRequest]", fmt.Sprintf("结束请求，响应结果：%s", string(bodyBytes)))
	reqSpan.End()
	c.Header("Content-Type", "application/json")
	c.String(http.StatusOK, string(bodyBytes))
}

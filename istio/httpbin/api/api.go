package api

import (
	"github.com/SkyAPM/go2sky"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

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
	"X-Httpbin-Trace-Host",
	"X-Httpbin-Trace-Service",
}

func Anything(c *gin.Context) {
	// Simulate business call
	r := rand.Intn(45) + 5
	time.Sleep(time.Duration(r) * time.Millisecond)
	// Return
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
		// Simulate business call
		r := rand.Intn(45) + 5
		time.Sleep(time.Duration(r) * time.Millisecond)
		// Return
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
	traceHeader, ok := lowerCaseHeader["x-httpbin-trace-host"]
	if !ok {
		lowerCaseHeader["x-httpbin-trace-host"] = []string{utils.GetHostName()}
	} else {
		lowerCaseHeader["x-httpbin-trace-host"] = []string{traceHeader[0] + "/" + utils.GetHostName()}
	}

	traceHeader2, ok2 := lowerCaseHeader["x-httpbin-trace-service"]
	if !ok2 {
		lowerCaseHeader["x-httpbin-trace-service"] = []string{utils.GetServiceName()}
	} else {
		lowerCaseHeader["x-httpbin-trace-service"] = []string{traceHeader2[0] + "/" + utils.GetServiceName()}
	}

	req.Header = lowerCaseHeader
	fn := func(req *http.Request) (*http.Response, error) {
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			klog.Error(err)
		}
		return resp, err
	}
	resp, err := traceHttpCall(c, req, nextUrl, fn)
	var bodyBytes []byte
	bodyBytes, _ = io.ReadAll(resp.Body)
	c.Header("Content-Type", "application/json")
	c.String(http.StatusOK, string(bodyBytes))
}

func traceHttpCall(c *gin.Context, req *http.Request, url string, fn func(req *http.Request) (*http.Response, error)) (*http.Response, error) {
	tracer := go2sky.GetGlobalTracer()
	if tracer == nil {
		resp, err := fn(req)
		return resp, err
	}

	reqSpan, err := go2sky.GetGlobalTracer().CreateExitSpan(c.Request.Context(), "invoke", url, func(headerKey, headerValue string) error {
		req.Header.Set(headerKey, headerValue)
		return nil
	})
	if err != nil {
	}
	reqSpan.SetComponent(2)
	reqSpan.SetSpanLayer(v3.SpanLayer_RPCFramework) // rpc 调用
	resp, err2 := fn(req)
	reqSpan.Tag(go2sky.TagHTTPMethod, http.MethodPost)
	reqSpan.Tag(go2sky.TagURL, url)
	reqSpan.End()
	return resp, err2

}

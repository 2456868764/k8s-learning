package middleware

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/SkyAPM/go2sky"
	"github.com/SkyAPM/go2sky/reporter"
	"github.com/gin-gonic/gin"
	"httpbin/pkg/options"
	"k8s.io/klog/v2"
	agentv3 "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
)

const (
	componentIDGINHttpServer = 5006
	skipProbPrefix           = "/prob/"
	skipMetricsPrefix        = "/metrics"
)

func StartSkywalkingTracer(g *gin.Engine, option *options.Option) {
	if len(option.SkywalkingGrpcAddress) == 0 {
		return
	}

	reporter, err := reporter.NewGRPCReporter(option.SkywalkingGrpcAddress)
	if err != nil {
		klog.Errorf("create gosky reporter failed! error:%v", err)
	}
	tracer, err := go2sky.NewTracer(option.ServiceName, go2sky.WithReporter(reporter),
		go2sky.WithInstance(option.InstanceName),
		go2sky.WithSampler(option.SamplingRate))
	g.Use(middleware(g, tracer))
	go2sky.SetGlobalTracer(tracer)
}

// Middleware gin middleware return HandlerFunc  with tracing.
func middleware(engine *gin.Engine, tracer *go2sky.Tracer) gin.HandlerFunc {
	if engine == nil || tracer == nil {
		return func(c *gin.Context) {
			c.Next()
		}
	}
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.String(), skipProbPrefix) || strings.HasPrefix(c.Request.URL.String(), skipMetricsPrefix) {
			c.Next()
			return
		}
		span, ctx, err := tracer.CreateEntrySpan(c.Request.Context(), getOperationName(c), func(key string) (string, error) {
			return c.Request.Header.Get(key), nil
		})
		if err != nil {
			c.Next()
			return
		}
		span.SetComponent(componentIDGINHttpServer)
		span.Tag(go2sky.TagHTTPMethod, c.Request.Method)
		span.Tag(go2sky.TagURL, c.Request.Host+c.Request.URL.Path)
		span.SetSpanLayer(agentv3.SpanLayer_Http)

		c.Request = c.Request.WithContext(ctx)

		c.Next()

		if len(c.Errors) > 0 {
			span.Error(time.Now(), c.Errors.String())
		}
		span.Tag(go2sky.TagStatusCode, strconv.Itoa(c.Writer.Status()))
		span.End()
	}
}

func getOperationName(c *gin.Context) string {
	return fmt.Sprintf("/%s%s", c.Request.Method, c.FullPath())
}

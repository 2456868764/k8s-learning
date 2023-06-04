package middleware

import (
	"github.com/SkyAPM/go2sky"
	v3 "github.com/SkyAPM/go2sky-plugins/gin/v3"
	"github.com/SkyAPM/go2sky/reporter"
	"github.com/gin-gonic/gin"
	"httpbin/pkg/options"
	"k8s.io/klog/v2"
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
	g.Use(v3.Middleware(g, tracer))
	go2sky.SetGlobalTracer(tracer)
}

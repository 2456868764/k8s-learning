package app

import (
	"context"
	"flag"
	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	"httpbin/api"
	"httpbin/pkg/logs"
	"httpbin/pkg/middleware"
	"httpbin/pkg/options"
)

func NewAppCommand(ctx context.Context) *cobra.Command {
	option := options.NewOption()
	cmd := &cobra.Command{
		Use:  "httpbin",
		Long: `httpbin for mesh`,
		Run: func(cmd *cobra.Command, args []string) {
			logs.Infof("run with option:%+v", option)
			if err := Run(ctx, option); err != nil {
				logs.Fatal(err)
			}
		},
	}
	cmd.Flags().AddGoFlagSet(flag.CommandLine)
	option.AddFlags(cmd.Flags())
	option.FillEnvs()
	return cmd
}

func Run(ctx context.Context, option *options.Option) error {
	r := gin.New()
	// Start Trace
	middleware.StartSkywalkingTracer(r, option)
	// Start Metric
	middleware.StartMetric(r, option)
	// Start Log
	middleware.StartLogger(r, option)

	r.GET("/", api.Anything)
	r.POST("/", api.Anything)
	r.GET("/hostname", api.HostName)
	r.GET("/headers", api.Headers)

	// liveness, readiness, startup prob
	r.GET("/prob/liveness", api.Healthz)
	r.GET("/prob/livenessfile", api.HealthzFile)
	r.GET("/prob/readiness", api.Readiness)
	r.GET("/prob/readinessfile", api.ReadinessFile)
	r.GET("/prob/startup", api.Startup)
	r.GET("/prob/startupfile", api.StartupFile)

	// Test any data
	r.GET("/data/bool", api.Bool)
	r.GET("/data/dto", api.ReponseAnyDto)
	r.GET("/data/array", api.ReponseAnyArray)
	r.GET("/data/string", api.ReponseAnyString)

	// Service call
	r.GET("/service", api.Service)

	r.Run(option.ServerAddress)

	return nil
}

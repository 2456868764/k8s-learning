package main

import (
	"github.com/2456868764/k8s-learning/istio/httpbin/api"
	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()
	r.GET("/", api.Anything)
	r.POST("/", api.Anything)
	r.POST("/api", api.Anything)
	r.GET("/hostname", api.HostName)
	r.GET("/headers", api.Headers)
	r.GET("/healthz", api.Healthz)
	r.Run(":80")
}

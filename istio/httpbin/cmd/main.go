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
	r.GET("/healthzfile", api.HealthzFile)
	r.GET("/bool", api.Bool)
	r.GET("/dto", api.ReponseAnyDto)
	r.GET("/arraydto", api.ReponseAnyArray)
	r.GET("/string", api.ReponseAnyString)
	r.GET("/service", api.Service)
	r.Run(":80")
}

package main

import (
	"github.com/2456868764/k8s-learning/istio/envoy/httpbin/api"
	"github.com/gin-gonic/gin"
)
func main() {
	r := gin.Default()
	r.GET("/", api.Anything)
	r.Run(":80")
}

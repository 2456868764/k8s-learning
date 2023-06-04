// ref: https://github.com/zsais/go-gin-prometheus/blob/master/middleware.go#L248

package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"httpbin/pkg/options"
	"k8s.io/klog/v2"
)

const (
	defaultMetricPath string = "/metrics"
)

type Metric struct {
	ID              string
	Name            string
	Description     string
	Type            string
	Args            []string
	MetricCollector prometheus.Collector
}

var reqCountMetric = &Metric{
	ID:          "reqCount",
	Name:        "requests_total",
	Description: "How many HTTP requests processed, partitioned by status code and HTTP method.",
	Type:        "counter_vec",
	Args:        []string{"code", "method", "handler", "host", "url"},
}

var reqDurationMetric = &Metric{
	ID:          "reqDuration",
	Name:        "request_duration_seconds",
	Description: "The HTTP request latencies in seconds.",
	Type:        "histogram_vec",
	Args:        []string{"code", "method", "url"},
}

var resSizeMetric = &Metric{
	ID:          "resSize",
	Name:        "response_size_bytes",
	Description: "The HTTP response sizes in bytes.",
	Type:        "summary",
}

var reqSizeMetric = &Metric{
	ID:          "reqSize",
	Name:        "request_size_bytes",
	Description: "The HTTP request sizes in bytes.",
	Type:        "summary",
}

var standardMetrics = []*Metric{
	reqCountMetric,
	reqDurationMetric,
	resSizeMetric,
	reqSizeMetric,
}

type RequestCounterURLLabelMappingFn func(c *gin.Context) string

type metricMiddleWareBuilder struct {
	Subsystem               string
	ConstLabels             map[string]string
	Help                    string
	reqCount                *prometheus.CounterVec
	reqDuration             *prometheus.HistogramVec
	reqSize, resSize        prometheus.Summary
	MetricsList             []*Metric
	ReqCntURLLabelMappingFn RequestCounterURLLabelMappingFn
}

func newMetric(m *Metric, subsystem string, constLables map[string]string) prometheus.Collector {
	var metric prometheus.Collector
	switch m.Type {
	case "counter_vec":
		metric = prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem:   subsystem,
				Name:        m.Name,
				Help:        m.Description,
				ConstLabels: constLables,
			},
			m.Args,
		)
	case "counter":
		metric = prometheus.NewCounter(
			prometheus.CounterOpts{
				Subsystem:   subsystem,
				Name:        m.Name,
				Help:        m.Description,
				ConstLabels: constLables,
			},
		)
	case "gauge_vec":
		metric = prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Subsystem:   subsystem,
				Name:        m.Name,
				Help:        m.Description,
				ConstLabels: constLables,
			},
			m.Args,
		)
	case "gauge":
		metric = prometheus.NewGauge(
			prometheus.GaugeOpts{
				Subsystem:   subsystem,
				Name:        m.Name,
				Help:        m.Description,
				ConstLabels: constLables,
			},
		)
	case "histogram_vec":
		metric = prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Subsystem:   subsystem,
				Name:        m.Name,
				Help:        m.Description,
				ConstLabels: constLables,
			},
			m.Args,
		)
	case "histogram":
		metric = prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Subsystem:   subsystem,
				Name:        m.Name,
				Help:        m.Description,
				ConstLabels: constLables,
			},
		)
	case "summary_vec":
		metric = prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Subsystem:   subsystem,
				Name:        m.Name,
				Help:        m.Description,
				ConstLabels: constLables,
			},
			m.Args,
		)
	case "summary":
		metric = prometheus.NewSummary(
			prometheus.SummaryOpts{
				Subsystem:   subsystem,
				Name:        m.Name,
				Help:        m.Description,
				ConstLabels: constLables,
			},
		)
	}
	return metric
}

func (m *metricMiddleWareBuilder) registerMetrics() {
	for _, metricDef := range m.MetricsList {
		metric := newMetric(metricDef, m.Subsystem, m.ConstLabels)
		if err := prometheus.Register(metric); err != nil {
			klog.Errorf("%s could not be registered in Prometheus", metricDef.Name)
		}
		switch metricDef.ID {
		case "reqCount":
			m.reqCount = metric.(*prometheus.CounterVec)
		case "reqDuration":
			m.reqDuration = metric.(*prometheus.HistogramVec)
		case "resSize":
			m.resSize = metric.(prometheus.Summary)
		case "reqSize":
			m.reqSize = metric.(prometheus.Summary)
		}
		metricDef.MetricCollector = metric
	}
}

func (m *metricMiddleWareBuilder) middleware() gin.HandlerFunc {
	// start init metric and register
	return func(c *gin.Context) {
		if c.Request.URL.String() == defaultMetricPath {
			c.Next()
			return
		}
		start := time.Now()
		reqSz := computeApproximateRequestSize(c.Request)
		c.Next()
		status := strconv.Itoa(c.Writer.Status())
		elapsed := float64(time.Since(start)) / float64(time.Second)
		resSz := float64(c.Writer.Size())
		url := m.ReqCntURLLabelMappingFn(c)
		if m.reqDuration != nil {
			m.reqDuration.WithLabelValues(status, c.Request.Method, url).Observe(elapsed)
		}
		if m.reqCount != nil {
			m.reqCount.WithLabelValues(status, c.Request.Method, c.HandlerName(), c.Request.Host, url).Inc()
		}
		if m.reqSize != nil {
			m.reqSize.Observe(float64(reqSz))
		}
		if m.resSize != nil {
			m.resSize.Observe(resSz)
		}
	}
}

func (m *metricMiddleWareBuilder) prometheusHandler() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

func StartMetric(g *gin.Engine, option *options.Option) {
	labels := make(map[string]string, 2)
	labels["instance"] = option.InstanceName
	labels["service"] = option.ServiceName
	builder := metricMiddleWareBuilder{
		Subsystem:   strings.ToLower(option.SubSystem),
		ConstLabels: labels,
		ReqCntURLLabelMappingFn: func(c *gin.Context) string {
			return c.Request.URL.Path // i.e. by default do nothing, i.e. return URL as is
		},
		MetricsList: standardMetrics,
	}
	builder.registerMetrics()
	g.Use(builder.middleware())
	g.GET(defaultMetricPath, builder.prometheusHandler())
}

// From https://github.com/DanielHeckrath/gin-prometheus/blob/master/gin_prometheus.go
func computeApproximateRequestSize(r *http.Request) int {
	s := 0
	if r.URL != nil {
		s = len(r.URL.Path)
	}

	s += len(r.Method)
	s += len(r.Proto)
	for name, values := range r.Header {
		s += len(name)
		for _, value := range values {
			s += len(value)
		}
	}
	s += len(r.Host)

	// N.B. r.Form and r.MultipartForm are assumed to be included in r.URL.

	if r.ContentLength != -1 {
		s += int(r.ContentLength)
	}
	return s
}

package options

import (
	"strings"

	"github.com/spf13/pflag"
	"httpbin/pkg/utils"
)

type TraceProviderType string

const (
	TraceSkyWalking  TraceProviderType = "skywalking"
	DefaultSubSystem                   = "App"
	DefaultNameSpace                   = "default"
)

type Option struct {
	TraceProvider         TraceProviderType
	SkywalkingGrpcAddress string
	ServerAddress         string
	SamplingRate          float64

	ServiceName  string
	InstanceName string
	SubSystem    string
	NameSpace    string
}

func (o *Option) AddFlags(flags *pflag.FlagSet) {
	traceProvider := ""
	flags.StringVar(&traceProvider, "trace-provider", "skywalking", "Trace provider type")
	if strings.ToLower(traceProvider) == "skywalking" {
		o.TraceProvider = TraceSkyWalking
	}
	flags.StringVar(&o.SkywalkingGrpcAddress, "skywalking-grpc-address", "", "Skywalking grpc address.")
	flags.StringVar(&o.ServerAddress, "server-address", ":80", "The address the server binds to.")
	flags.Float64Var(&o.SamplingRate, "sample-rate", 1.0, "Trace sample rate")
}

func (o *Option) FillEnvs() {
	o.InstanceName = utils.GetHostName()
	o.ServiceName = utils.GetServiceName()
	o.SubSystem = utils.GetSubSystem()
	if len(o.SubSystem) == 0 {
		o.SubSystem = DefaultSubSystem
	}
	o.NameSpace = utils.GetNameSpace()
	if len(o.NameSpace) == 0 {
		o.NameSpace = DefaultNameSpace
	}
}

func NewOption() *Option {
	return &Option{}
}

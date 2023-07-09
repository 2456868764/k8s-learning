# 三种 api Server 链式的初始化

## 1. 核心三个api Server 

apiserver 对启动参数进行合法性校验通过后，就会调用 Run() 启动函数，并传递经过验证的选项配置 completeOptions 以及一个停止信号的通道 stopCh ，函数的定义如下：

```golang
// Run runs the specified APIServer.  This should never exit.
func Run(completeOptions completedServerRunOptions, stopCh <-chan struct{}) error {
	// To help debugging, immediately log version
	klog.Infof("Version: %+v", version.Get())

	klog.InfoS("Golang settings", "GOGC", os.Getenv("GOGC"), "GOMAXPROCS", os.Getenv("GOMAXPROCS"), "GOTRACEBACK", os.Getenv("GOTRACEBACK"))

	server, err := CreateServerChain(completeOptions)
	if err != nil {
		return err
	}

	prepared, err := server.PrepareRun()
	if err != nil {
		return err
	}

	return prepared.Run(stopCh)
}

```

- CreateServerChain ：创建服务调用链。该函数负责创建各种不同 API Server 的配置并初始化，最后构建出完整的 API Server 链式结构
- PrepareRun：服务启动前的准备工作。该函数负责进行健康检查、存活检查和 OpenAPI 路由的注册工作，以便 apiserver 能够顺利地运行
- Run：服务启动。该函数启动 HTTP Server 实例并开始监听和处理来自客户端的请求


首先看服务调用链的创建，在这里会根据不同功能进行解耦，创建出三个不同的 API Server ：
- AggregatorServer：API 聚合服务。用于实现 [Kubernetes API 聚合层](https://kubernetes.io/zh-cn/docs/concepts/extend-kubernetes/api-extension/apiserver-aggregation/) 的功能，当 AggregatorServer 接收到请求之后，如果发现对应的是一个 APIService 的请求，则会直接转发到对应的服务上（自行编写和部署的 API 服务器），否则则委托给 KubeAPIServer 进行处理
- KubeAPIServer：API 核心服务。实现认证、鉴权以及所有 Kubernetes 内置资源的 REST API 接口（诸如 Pod 和 Service 等资源的接口），如果请求未能找到对应的处理，则委托给 APIExtensionsServer 进行处理
- APIExtensionsServer：API 扩展服务。处理 CustomResourceDefinitions（CRD）和 Custom Resource（CR）的 REST 请求（自定义资源的接口），如果请求仍不能被处理则委托给 404 Handler 处理


```golang

func CreateServerChain(completedOptions completedServerRunOptions) (*aggregatorapiserver.APIAggregator, error) {
	kubeAPIServerConfig, serviceResolver, pluginInitializer, err := CreateKubeAPIServerConfig(completedOptions)
	if err != nil {
		return nil, err
	}

	// If additional API servers are added, they should be gated.
	apiExtensionsConfig, err := createAPIExtensionsConfig(*kubeAPIServerConfig.GenericConfig, kubeAPIServerConfig.ExtraConfig.VersionedInformers, pluginInitializer, completedOptions.ServerRunOptions, completedOptions.MasterCount,
		serviceResolver, webhook.NewDefaultAuthenticationInfoResolverWrapper(kubeAPIServerConfig.ExtraConfig.ProxyTransport, kubeAPIServerConfig.GenericConfig.EgressSelector, kubeAPIServerConfig.GenericConfig.LoopbackClientConfig, kubeAPIServerConfig.GenericConfig.TracerProvider))
	if err != nil {
		return nil, err
	}

	notFoundHandler := notfoundhandler.New(kubeAPIServerConfig.GenericConfig.Serializer, genericapifilters.NoMuxAndDiscoveryIncompleteKey)
	apiExtensionsServer, err := createAPIExtensionsServer(apiExtensionsConfig, genericapiserver.NewEmptyDelegateWithCustomHandler(notFoundHandler))
	if err != nil {
		return nil, err
	}

	kubeAPIServer, err := CreateKubeAPIServer(kubeAPIServerConfig, apiExtensionsServer.GenericAPIServer)
	if err != nil {
		return nil, err
	}

	// aggregator comes last in the chain
	aggregatorConfig, err := createAggregatorConfig(*kubeAPIServerConfig.GenericConfig, completedOptions.ServerRunOptions, kubeAPIServerConfig.ExtraConfig.VersionedInformers, serviceResolver, kubeAPIServerConfig.ExtraConfig.ProxyTransport, pluginInitializer)
	if err != nil {
		return nil, err
	}
	aggregatorServer, err := createAggregatorServer(aggregatorConfig, kubeAPIServer.GenericAPIServer, apiExtensionsServer.Informers)
	if err != nil {
		// we don't need special handling for innerStopCh because the aggregator server doesn't create any go routines
		return nil, err
	}

	return aggregatorServer, nil
}

```

这三个服务的类型
- APIExtensionsServer ：*apiextensionsapiserver.CustomResourceDefinitions
- KubeAPIServer ：*controlplane.Instance
- AggregatorServer ：*aggregatorapiserver.APIAggregator

```golang
// APIExtensionsServer 类型
type CustomResourceDefinitions struct {
	GenericAPIServer *genericapiserver.GenericAPIServer

	// provided for easier embedding
	Informers externalinformers.SharedInformerFactory
}

// KubeAPIServer 类型
// Instance contains state for a Kubernetes cluster api server instance.
type Instance struct {
    GenericAPIServer *genericapiserver.GenericAPIServer

    ClusterAuthenticationInfo clusterauthenticationtrust.ClusterAuthenticationInfo
}

// AggregatorServer 类型
// APIAggregator contains state for a Kubernetes cluster master/api server.
type APIAggregator struct {
    GenericAPIServer *genericapiserver.GenericAPIServer

    // provided for easier embedding
    APIRegistrationInformers informers.SharedInformerFactory

	delegateHandler http.Handler
}

```

都有一个共同点，包含了 GenericAPIServer 成员，而该成员实现了 DelegationTarget 接口:

```golang
// DelegationTarget is an interface which allows for composition of API servers with top level handling that works
// as expected.
type DelegationTarget interface {
    ...
	// NextDelegate returns the next delegationTarget in the chain of delegations
	NextDelegate() DelegationTarget
	
	...
}



```

```golang

func (s *GenericAPIServer) NextDelegate() DelegationTarget {
    return s.delegationTarget
}


// PrepareRun does post API installation setup steps. It calls recursively the same function of the delegates.
func (s *GenericAPIServer) PrepareRun() preparedGenericAPIServer {
	s.delegationTarget.PrepareRun()

	if s.openAPIConfig != nil && !s.skipOpenAPIInstallation {
		s.OpenAPIVersionedService, s.StaticOpenAPISpec = routes.OpenAPI{
			Config: s.openAPIConfig,
		}.InstallV2(s.Handler.GoRestfulContainer, s.Handler.NonGoRestfulMux)
	}

	if s.openAPIV3Config != nil && !s.skipOpenAPIInstallation {
		if utilfeature.DefaultFeatureGate.Enabled(features.OpenAPIV3) {
			s.OpenAPIV3VersionedService = routes.OpenAPI{
				Config: s.openAPIV3Config,
			}.InstallV3(s.Handler.GoRestfulContainer, s.Handler.NonGoRestfulMux)
		}
	}

	s.installHealthz()
	s.installLivez()

	// as soon as shutdown is initiated, readiness should start failing
	readinessStopCh := s.lifecycleSignals.ShutdownInitiated.Signaled()
	err := s.addReadyzShutdownCheck(readinessStopCh)
	if err != nil {
		klog.Errorf("Failed to install readyz shutdown check %s", err)
	}
	s.installReadyz()

	return preparedGenericAPIServer{s}
}
```

基于委托模式，重新看 CreateServerChain 函数，从尾节点开始依次创建 API Server 委托对象：

```golang
// 0、初始化 404 Handler Server
notFoundHandler := notfoundhandler.New(kubeAPIServerConfig.GenericConfig.Serializer, genericapifilters.NoMuxAndDiscoveryIncompleteKey)

// 1、初始化 APIExtensionsServer ，传入 404 Handler Server 作为下一个委托对象
apiExtensionsServer, err := createAPIExtensionsServer(apiExtensionsConfig, genericapiserver.NewEmptyDelegateWithCustomHandler(notFoundHandler))

// 2、初始化 KubeAPIServer ，传入 APIExtensionsServer 作为下一个委托对象
kubeAPIServer, err := CreateKubeAPIServer(kubeAPIServerConfig, apiExtensionsServer.GenericAPIServer)

// 3、初始化 AggregatorServer ，传入 KubeAPIServer 作为下一个委托对象
aggregatorServer, err := createAggregatorServer(aggregatorConfig, kubeAPIServer.GenericAPIServer, apiExtensionsServer.Informers)

// 4、返回 AggregatorServer
return aggregatorServer, nil

```

## 2. createAPIExtensionsServer

- 初始化 404 Handler Server

文件位置： /vendor/k8s.io/apiserver/pkg/util/notfoundhandler/not_found_handler.go
```golang

type Handler struct {
	serializer                  runtime.NegotiatedSerializer
	isMuxAndDiscoveryCompleteFn func(ctx context.Context) bool
}

func (h *Handler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if !h.isMuxAndDiscoveryCompleteFn(req.Context()) {
		errMsg := "the request has been made before all known HTTP paths have been installed, please try again"
		err := apierrors.NewServiceUnavailable(errMsg)
		if err.ErrStatus.Details == nil {
			err.ErrStatus.Details = &metav1.StatusDetails{}
		}
		err.ErrStatus.Details.RetryAfterSeconds = int32(5)

		gv := schema.GroupVersion{Group: "unknown", Version: "unknown"}
		requestInfo, ok := apirequest.RequestInfoFrom(req.Context())
		if ok {
			gv.Group = requestInfo.APIGroup
			gv.Version = requestInfo.APIVersion
		}
		responsewriters.ErrorNegotiated(err, h.serializer, gv, rw, req)
		return
	}
	http.NotFound(rw, req)
}

```

- genericapiserver.NewEmptyDelegateWithCustomHandler(notFoundHandler) 创建一个空 genericapiserver, emptyDelegate 实现 DelegationTarget 接口

```golang

type emptyDelegate struct {
	// handler is called at the end of the delegation chain
	// when a request has been made against an unregistered HTTP path the individual servers will simply pass it through until it reaches the handler.
	handler http.Handler
}

func NewEmptyDelegate() DelegationTarget {
	return emptyDelegate{}
}

// NewEmptyDelegateWithCustomHandler allows for registering a custom handler usually for special handling of 404 requests
func NewEmptyDelegateWithCustomHandler(handler http.Handler) DelegationTarget {
	return emptyDelegate{handler}
}

```

- createAPIExtensionsServer, 用传递过来 空 genericapiserver 作为 delegateAPIServer 初始化一个新的 genericServer

```golang
func createAPIExtensionsServer(apiextensionsConfig *apiextensionsapiserver.Config, delegateAPIServer genericapiserver.DelegationTarget) (*apiextensionsapiserver.CustomResourceDefinitions, error) {
	return apiextensionsConfig.Complete().New(delegateAPIServer)
}

// New returns a new instance of CustomResourceDefinitions from the given config.
func (c completedConfig) New(delegationTarget genericapiserver.DelegationTarget) (*CustomResourceDefinitions, error) {
    genericServer, err := c.GenericConfig.New("apiextensions-apiserver", delegationTarget)
...
}

```

## 3. CreateKubeAPIServer

把 apiExtensionsServer.GenericAPIServer 作为 delegationTarget 初始化 新的 GenericAPIServer

```golang

kubeAPIServer, err := CreateKubeAPIServer(kubeAPIServerConfig, apiExtensionsServer.GenericAPIServer)


// CreateKubeAPIServer creates and wires a workable kube-apiserver
func CreateKubeAPIServer(kubeAPIServerConfig *controlplane.Config, delegateAPIServer genericapiserver.DelegationTarget) (*controlplane.Instance, error) {
    return kubeAPIServerConfig.Complete().New(delegateAPIServer)
}


func (c completedConfig) New(delegationTarget genericapiserver.DelegationTarget) (*Instance, error) {
    ...
    s, err := c.GenericConfig.New("kube-apiserver", delegationTarget)
	...
}
```

## 4. createAggregatorServer, 用kubeAPIServer.GenericAPIServer 作为 delegationTarget 初始化 新的 GenericAPIServer

```golang
aggregatorServer, err := createAggregatorServer(aggregatorConfig, kubeAPIServer.GenericAPIServer, apiExtensionsServer.Informers)

func createAggregatorServer(aggregatorConfig *aggregatorapiserver.Config, delegateAPIServer genericapiserver.DelegationTarget, apiExtensionInformers apiextensionsinformers.SharedInformerFactory) (*aggregatorapiserver.APIAggregator, error) {
    aggregatorServer, err := aggregatorConfig.Complete().NewWithDelegate(delegateAPIServer)
	...
}


// NewWithDelegate returns a new instance of APIAggregator from the given config.
func (c completedConfig) NewWithDelegate(delegationTarget genericapiserver.DelegationTarget) (*APIAggregator, error) {
    genericServer, err := c.GenericConfig.New("kube-aggregator", delegationTarget)
	...
}
```

## 5. c.GenericConfig.New 初始化 GenericAPIServer


可以看到三个服务的初始化过程都是一样，调用 c.GenericConfig.New("server name", delegationTarget) 方法

至此，三个服务的链式初始化完成， 后续将继续完成路由注册，服务启动等步骤。





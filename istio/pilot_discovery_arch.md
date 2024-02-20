# Pilot discovery 组件和启动流程

## 启动入口
 
```golang
func main() {
	log.EnableKlogWithCobra()
	rootCmd := app.NewRootCommand()
	if err := rootCmd.Execute(); err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}
```

```golang

// NewRootCommand returns the root cobra command of pilot-discovery.
func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:          "pilot-discovery",
		Short:        "Istio Pilot.",
		Long:         "Istio Pilot provides mesh-wide traffic management, security and policy capabilities in the Istio Service Mesh.",
		SilenceUsage: true,
		FParseErrWhitelist: cobra.FParseErrWhitelist{
			// Allow unknown flags for backward-compatibility.
			UnknownFlags: true,
		},
		PreRunE: func(c *cobra.Command, args []string) error {
			cmd.AddFlags(c)
			return nil
		},
	}

	discoveryCmd := newDiscoveryCommand()
	addFlags(discoveryCmd)
	rootCmd.AddCommand(discoveryCmd)
	rootCmd.AddCommand(version.CobraCommand())
	rootCmd.AddCommand(collateral.CobraCommand(rootCmd, &doc.GenManHeader{
		Title:   "Istio Pilot Discovery",
		Section: "pilot-discovery CLI",
		Manual:  "Istio Pilot Discovery",
	}))
	rootCmd.AddCommand(requestCmd)

	return rootCmd
}

func newDiscoveryCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "discovery",
		Short: "Start Istio proxy discovery service.",
		Args:  cobra.ExactArgs(0),
		FParseErrWhitelist: cobra.FParseErrWhitelist{
			// Allow unknown flags for backward-compatibility.
			UnknownFlags: true,
		},
	    // ...
		RunE: func(c *cobra.Command, args []string) error {
			cmd.PrintFlags(c.Flags())

			// Create the stop channel for all the servers.
			stop := make(chan struct{})

			// Create the server for the discovery service.
			discoveryServer, err := bootstrap.NewServer(serverArgs)
			if err != nil {
				return fmt.Errorf("failed to create discovery service: %v", err)
			}

			// Start the server
			if err := discoveryServer.Start(stop); err != nil {
				return fmt.Errorf("failed to start discovery service: %v", err)
			}

			cmd.WaitSignal(stop)
			// Wait until we shut down. In theory this could block forever; in practice we will get
			// forcibly shut down after 30s in Kubernetes.
			discoveryServer.WaitUntilCompletion()
			return nil
		},
	}
}
```

## 初始化流程

接下来介绍 discoveryServer ，即 pilot-discovery 组件的核心。在这之前先看下 Server 的结构，代码位于 /pilot/pkg/bootstrap/server.go 文件中。

### 1. Server 结构
```golang
// Server contains the runtime configuration for the Pilot discovery service.
type Server struct {
    // Xds 服务
	XDSServer *xds.DiscoveryServer

	clusterID   cluster.ID

   // Pilot 环境所需的 API 集合
	environment *model.Environment

	kubeClient kubelib.Client

    // 处理 Kubernetes 多个集群的注册中心
	multiclusterController *multicluster.Controller
    // 统一处理配置数据（如 VirtualService 等) 的 Controller
	configController       model.ConfigStoreController
     // 不同配置信息的缓存器，提供 Get、List、Create 等方法
	ConfigStores           []model.ConfigStoreController
    // 单独处理 ServiceEntry 的 Controller
	serviceEntryController *serviceentry.Controller

	httpServer  *http.Server // debug, monitoring and readiness Server.
	httpAddr    string
	httpsServer *http.Server // webhooks HTTPS Server.

	grpcServer        *grpc.Server
	grpcAddress       string
	secureGrpcServer  *grpc.Server
	secureGrpcAddress string

	// monitoringMux listens on monitoringAddr(:15014).
	// Currently runs prometheus monitoring and debug (if enabled).
	monitoringMux *http.ServeMux
	// internalDebugMux is a mux for *internal* calls to the debug interface. That is, authentication is disabled.
	internalDebugMux *http.ServeMux

	// httpMux listens on the httpAddr (8080).
	// If a Gateway is used in front and https is off it is also multiplexing
	// the rest of the features if their port is empty.
	// Currently runs readiness and debug (if enabled)
	httpMux *http.ServeMux

	// httpsMux listens on the httpsAddr(15017), handling webhooks
	// If the address os empty, the webhooks will be set on the default httpPort.
	httpsMux *http.ServeMux // webhooks

    // 文件监听器，主要 watch meshconfig 和 networks 配置文件等
	// fileWatcher used to watch mesh config, networks and certificates.
	fileWatcher filewatcher.FileWatcher

    // 证书监听器
	// certWatcher watches the certificates for changes and triggers a notification to Istiod.
	cacertsWatcher *fsnotify.Watcher
	dnsNames       []string

	CA *ca.IstioCA
	RA ra.RegistrationAuthority

	// TrustAnchors for workload to workload mTLS
	workloadTrustBundle     *tb.TrustBundle
	certMu                  sync.RWMutex
	istiodCert              *tls.Certificate
	istiodCertBundleWatcher *keycertbundle.Watcher
	server                  server.Instance

	readinessProbes map[string]readinessProbe
	readinessFlags  *readinessFlags

	// duration used for graceful shutdown.
	shutdownDuration time.Duration

	// internalStop is closed when the server is shutdown. This should be avoided as much as possible, in
	// favor of AddStartFunc. This is only required if we *must* start something outside of this process.
	// For example, everything depends on mesh config, so we use it there rather than trying to sequence everything
	// in AddStartFunc
	internalStop chan struct{}

	// sidecar 注入 webhook
	webhookInfo *webhookInfo

	statusReporter *distribution.Reporter
	statusManager  *status.Manager
	// RWConfigStore is the configstore which allows updates, particularly for status.
	RWConfigStore model.ConfigStoreController
}
```

### 2. NewServer() 方法

再看 NewServer() 方法中的内容，有以下几个关键步骤：
- new 环境
- new xds.NewDiscoveryServer
- s.initReadinessProbes() 初始化就绪探针
- s.initServers() 初始化 Server
- s.initIstiodAdminServer() 初始化 Admin Server
- s.initKubeClient() 初始化 kubeClient
- s.initMeshConfiguration()  初始化 MeshConfig
- s.initMeshNetworks() 初始化 Mesh network
- s.initMeshHandlers()
- s.environment.Init() 初始化 environment
- s.maybeCreateCA() 初始化 CA signing certificate
- s.initControllers() 初始化 controllers
- s.initWorkloadTrustBundle() 初始化 workloadTrustBundle
- s.initIstiodCerts() 初始化 Istiod certs and setup watches.
- s.initSecureDiscoveryService() 初始化 DiscoveryService
- s.initSecureWebhookServer(), s.initSidecarInjector() 初始化 webhook webhooks (e.g. injection, validation)
- s.initRegistryEventHandlers() 
- s.initDiscoveryService()
- s.startCA()
- kubclient.RunAndWait()

```golang
// /pilot/pkg/bootstrap/server.go
// NewServer creates a new Server instance based on the provided arguments.
func NewServer(args *PilotArgs, initFuncs ...func(*Server)) (*Server, error) {
	e := model.NewEnvironment()
	e.DomainSuffix = args.RegistryOptions.KubeOptions.DomainSuffix
	e.SetLedger(buildLedger(args.RegistryOptions))

	ac := aggregate.NewController(aggregate.Options{
		MeshHolder: e,
	})
	e.ServiceDiscovery = ac

	s := &Server{
		clusterID:               getClusterID(args),
		environment:             e,
		fileWatcher:             filewatcher.NewWatcher(),
		httpMux:                 http.NewServeMux(),
		monitoringMux:           http.NewServeMux(),
		readinessProbes:         make(map[string]readinessProbe),
		readinessFlags:          &readinessFlags{},
		workloadTrustBundle:     tb.NewTrustBundle(nil),
		server:                  server.New(),
		shutdownDuration:        args.ShutdownDuration,
		internalStop:            make(chan struct{}),
		istiodCertBundleWatcher: keycertbundle.NewWatcher(),
		webhookInfo:             &webhookInfo{},
	}

	// Apply custom initialization functions.
	for _, fn := range initFuncs {
		fn(s)
	}
	// Initialize workload Trust Bundle before XDS Server
	e.TrustBundle = s.workloadTrustBundle
	s.XDSServer = xds.NewDiscoveryServer(e, args.RegistryOptions.KubeOptions.ClusterAliases)

	grpcprom.EnableHandlingTimeHistogram()

	// make sure we have a readiness probe before serving HTTP to avoid marking ready too soon
	s.initReadinessProbes()

	s.initServers(args)
	if err := s.initIstiodAdminServer(args, s.webhookInfo.GetTemplates); err != nil {
		return nil, fmt.Errorf("error initializing debug server: %v", err)
	}
	if err := s.serveHTTP(); err != nil {
		return nil, fmt.Errorf("error serving http: %v", err)
	}

	// Apply the arguments to the configuration.
	if err := s.initKubeClient(args); err != nil {
		return nil, fmt.Errorf("error initializing kube client: %v", err)
	}

	s.initMeshConfiguration(args, s.fileWatcher)
	spiffe.SetTrustDomain(s.environment.Mesh().GetTrustDomain())

	s.initMeshNetworks(args, s.fileWatcher)
	s.initMeshHandlers()
	s.environment.Init()
	if err := s.environment.InitNetworksManager(s.XDSServer); err != nil {
		return nil, err
	}

	// Options based on the current 'defaults' in istio.
	caOpts := &caOptions{
		TrustDomain:      s.environment.Mesh().TrustDomain,
		Namespace:        args.Namespace,
		DiscoveryFilter:  args.RegistryOptions.KubeOptions.GetFilter(),
		ExternalCAType:   ra.CaExternalType(externalCaType),
		CertSignerDomain: features.CertSignerDomain,
	}

	if caOpts.ExternalCAType == ra.ExtCAK8s {
		// Older environment variable preserved for backward compatibility
		caOpts.ExternalCASigner = k8sSigner
	}
	// CA signing certificate must be created first if needed.
	if err := s.maybeCreateCA(caOpts); err != nil {
		return nil, err
	}

	if err := s.initControllers(args); err != nil {
		return nil, err
	}

	s.XDSServer.InitGenerators(e, args.Namespace, s.clusterID, s.internalDebugMux)

	// Initialize workloadTrustBundle after CA has been initialized
	if err := s.initWorkloadTrustBundle(args); err != nil {
		return nil, err
	}

	// Parse and validate Istiod Address.
	istiodHost, _, err := e.GetDiscoveryAddress()
	if err != nil {
		return nil, err
	}

	// Create Istiod certs and setup watches.
	if err := s.initIstiodCerts(args, string(istiodHost)); err != nil {
		return nil, err
	}

	// Secure gRPC Server must be initialized after CA is created as may use a Citadel generated cert.
	if err := s.initSecureDiscoveryService(args); err != nil {
		return nil, fmt.Errorf("error initializing secure gRPC Listener: %v", err)
	}

	// common https server for webhooks (e.g. injection, validation)
	if s.kubeClient != nil {
		s.initSecureWebhookServer(args)
		wh, err := s.initSidecarInjector(args)
		if err != nil {
			return nil, fmt.Errorf("error initializing sidecar injector: %v", err)
		}
		s.readinessFlags.sidecarInjectorReady.Store(true)
		s.webhookInfo.mu.Lock()
		s.webhookInfo.wh = wh
		s.webhookInfo.mu.Unlock()
		if err := s.initConfigValidation(args); err != nil {
			return nil, fmt.Errorf("error initializing config validator: %v", err)
		}
	}

	// This should be called only after controllers are initialized.
	s.initRegistryEventHandlers()

	s.initDiscoveryService()

	// Notice that the order of authenticators matters, since at runtime
	// authenticators are activated sequentially and the first successful attempt
	// is used as the authentication result.
	authenticators := []security.Authenticator{
		&authenticate.ClientCertAuthenticator{},
	}
	if args.JwtRule != "" {
		jwtAuthn, err := initOIDC(args)
		if err != nil {
			return nil, fmt.Errorf("error initializing OIDC: %v", err)
		}
		if jwtAuthn == nil {
			return nil, fmt.Errorf("JWT authenticator is nil")
		}
		authenticators = append(authenticators, jwtAuthn)
	}
	// The k8s JWT authenticator requires the multicluster registry to be initialized,
	// so we build it later.
	if s.kubeClient != nil {
		authenticators = append(authenticators,
			kubeauth.NewKubeJWTAuthenticator(s.environment.Watcher, s.kubeClient.Kube(), s.clusterID, s.multiclusterController.GetRemoteKubeClient, features.JwtPolicy))
	}
	if len(features.TrustedGatewayCIDR) > 0 {
		authenticators = append(authenticators, &authenticate.XfccAuthenticator{})
	}
	if features.XDSAuth {
		s.XDSServer.Authenticators = authenticators
	}
	caOpts.Authenticators = authenticators

	// Start CA or RA server. This should be called after CA and Istiod certs have been created.
	s.startCA(caOpts)

	// TODO: don't run this if galley is started, one ctlz is enough
	if args.CtrlZOptions != nil {
		_, _ = ctrlz.Run(args.CtrlZOptions, nil)
	}

	// This must be last, otherwise we will not know which informers to register
	if s.kubeClient != nil {
		s.addStartFunc("kube client", func(stop <-chan struct{}) error {
			s.kubeClient.RunAndWait(stop)
			return nil
		})
	}

	return s, nil
}

```

#### a）初始化 Environment

什么是 Environment 呢？根据定义 Environment 为 Pilot 提供了一个汇总的、运行中所需的 API 集合。 Environment 中字段（接口）如下：

```golang
// Environment provides an aggregate environmental API for Pilot
type Environment struct {
	// Discovery interface for listing services and instances.
	// 服务发现的接口模型，主要列出 services 和 instances
	ServiceDiscovery

	// Config interface for listing routing rules
	// Istio 配置文件的存储器，主要列出 ServiceEntry 等配置
	ConfigStore

	// Watcher is the watcher for the mesh config (to be merged into the config store)
	// mesh config 文件的监听器
	mesh.Watcher

	// NetworksWatcher (loaded from a config map) provides information about the
	// set of networks inside a mesh and how to route to endpoints in each
	// network. Each network provides information about the endpoints in a
	// routable L3 network. A single routable L3 network can have one or more
	// service registries.
	// mesh network config 文件的监听器
	NetworksWatcher mesh.NetworksWatcher

	NetworkManager *NetworkManager

	// mutex used for protecting Environment.pushContext
	mutex sync.RWMutex
	// pushContext holds information during push generation. It is reset on config change, at the beginning
	// of the pushAll. It will hold all errors and stats and possibly caches needed during the entire cache computation.
	// DO NOT USE EXCEPT FOR TESTS AND HANDLING OF NEW CONNECTIONS.
	// ALL USE DURING A PUSH SHOULD USE THE ONE CREATED AT THE
	// START OF THE PUSH, THE GLOBAL ONE MAY CHANGE AND REFLECT A DIFFERENT
	// CONFIG AND PUSH
    // 在推送（下发 xDS）生成期间保存信息的上下文
	pushContext *PushContext

	// DomainSuffix provides a default domain for the Istio server.
    // istio server 默认的后缀域名
	DomainSuffix string

	ledger ledger.Ledger

	// TrustBundle: List of Mesh TrustAnchors
	TrustBundle *trustbundle.TrustBundle

	clusterLocalServices ClusterLocalProvider

	CredentialsController credentials.MulticlusterController

	GatewayAPIController GatewayController

	// EndpointShards for a service. This is a global (per-server) list, built from
	// incremental updates. This is keyed by service and namespace
	EndpointIndex *EndpointIndex

	// Cache for XDS resources.
	Cache XdsCache
}

```

其中 PushContext 是 Pilot 在推送 xDS 前，生成配置期间保存相关信息的上下文的地方，在全量推送配置和配置发生改变时重置。它会保存所有的错误和统计信息，并缓存一些配置的计算信息。 
ServiceDiscovery 提供了枚举 Istio 中服务和实例的方法。 mesh.Watcher 和 mesh.NetworksWatcher 负责监听 istiod 启动时挂载的两个配置文件，这两个配置文件是通过 configmap 映射到 Pod 的文件系统中的，监听器将在监听到配置文件变化时运行预先注册的 Handler 。
文件挂载参考 istiod 的配置文件：



Environment 的初始化：

```golang
	e := model.NewEnvironment()
	e.DomainSuffix = args.RegistryOptions.KubeOptions.DomainSuffix
	e.SetLedger(buildLedger(args.RegistryOptions))

	ac := aggregate.NewController(aggregate.Options{
		MeshHolder: e,
	})
	e.ServiceDiscovery = ac
```
首先是初始化了一份 PushContext ，创建 PushContext 所需的各种列表和 Map 。其次是初始化了一个聚合所有注册中心的 Controller 作为 Environment 中的 ServiceDiscovery 。
该 Controller 提供从所有注册中心（如 Kubernetes, Consul, MCP 等）获取服务和实例列表的方法。这里传入了一个参数 MeshHolder 是想利用 Environment 中的 mesh.Watcher 将 mesh 这个配置同步过去。

#### b. 初始化 DiscoveryServer

XDSServer 相关的代码在 istio/pilot/pkg/xds/discovery.go 中，对应为 DiscoveryServer ，该服务为 Envoy xDS APIs 的 gRPC 实现。 

DiscoveryServer 关键定义如下：

```golang
// DiscoveryServer is Pilot's gRPC implementation for Envoy's xds APIs
type DiscoveryServer struct {
	// Env is the model environment.
	// 即上述 pilot server 中的 Environment
	Env *model.Environment

	// ConfigGenerator is responsible for generating data plane configuration using Istio networking
	// APIs and service registry info
	// 控制面 Istio 配置的生成器，如 VirtualService 等
	ConfigGenerator core.ConfigGenerator

	// Generators allow customizing the generated config, based on the client metadata.
	// Key is the generator type - will match the Generator metadata to set the per-connection
	// default generator, or the combination of Generator metadata and TypeUrl to select a
	// different generator for a type.
	// Normal istio clients use the default generator - will not be impacted by this.
	// 针对不同配置类型的定制化生成器
	Generators map[string]model.XdsResourceGenerator

	// ProxyNeedsPush is a function that determines whether a push can be completely skipped. Individual generators
	// may also choose to not send any updates.
	ProxyNeedsPush func(proxy *model.Proxy, req *model.PushRequest) bool

	// concurrentPushLimit is a semaphore that limits the amount of concurrent XDS pushes.
	concurrentPushLimit chan struct{}
	// RequestRateLimit limits the number of new XDS requests allowed. This helps prevent thundering hurd of incoming requests.
	RequestRateLimit *rate.Limiter

	// InboundUpdates describes the number of configuration updates the discovery server has received
	InboundUpdates *atomic.Int64
	// CommittedUpdates describes the number of configuration updates the discovery server has
	// received, process, and stored in the push context. If this number is less than InboundUpdates,
	// there are updates we have not yet processed.
	// Note: This does not mean that all proxies have received these configurations; it is strictly
	// the push context, which means that the next push to a proxy will receive this configuration.
	CommittedUpdates *atomic.Int64

	// pushChannel is the buffer used for debouncing.
	// after debouncing the pushRequest will be sent to pushQueue
    // 接收 push 请求的 channel
	pushChannel chan *model.PushRequest

	// pushQueue is the buffer that used after debounce and before the real xds push.
    // 防抖之后，真正 Push xDS 之前所用的缓冲队列
	pushQueue *PushQueue

	// debugHandlers is the list of all the supported debug handlers.
	debugHandlers map[string]string

	// adsClients reflect active gRPC channels, for both ADS and EDS.
	// ADS 和 EDS 的 gRPC 连接
	adsClients      map[string]*Connection
	adsClientsMutex sync.RWMutex

	// 监听 xDS ACK 和连接断开
	StatusReporter DistributionStatusCache

	// Authenticators for XDS requests. Should be same/subset of the CA authenticators.
	Authenticators []security.Authenticator

	// StatusGen is notified of connect/disconnect/nack on all connections
	StatusGen               *StatusGen
	WorkloadEntryController *autoregistration.Controller

	// serverReady indicates caches have been synced up and server is ready to process requests.
	// 表示缓存已同步，server 可以接受请求
	serverReady atomic.Bool
	// 防抖设置
	debounceOptions debounceOptions

	// Cache for XDS resources
	// xDS 资源的缓存，目前仅适用于 EDS，线程安全
	Cache model.XdsCache

	// JwtKeyResolver holds a reference to the JWT key resolver instance.
	JwtKeyResolver *model.JwksResolver

	// ListRemoteClusters collects debug information about other clusters this istiod reads from.
	ListRemoteClusters func() []cluster.DebugInfo

	// ClusterAliases are alias names for cluster. When a proxy connects with a cluster ID
	// and if it has a different alias we should use that a cluster ID for proxy.
	ClusterAliases map[cluster.ID]cluster.ID

	// pushVersion stores the numeric push version. This should be accessed via NextVersion()
	pushVersion atomic.Uint64

	// discoveryStartTime is the time since the binary started
	discoveryStartTime time.Time
}

```

#### c. 初始化 MeshConfig 、 KubeClient 、 MeshNetworks 和 MeshHandlers

这几个初始化函数比较好理解， initMeshConfiguration 和 initMeshNetworks 都是通过 fileWatcher 对 istiod 从 configmap 中挂载的两个配置文件 mesh 和 meshNetworks 进行监听。当配置文件发生变化时重载配置并触发相应的 Handlers 。

#### d. 初始化 Controllers

这部分比较核心，初始化了三种控制器分别处理证书、配置信息和注册信息，证书及安全相关的内容本篇先暂不讨论。
包括 initMulticluster()，initSDSServer() initConfigController() initServiceControllers()
主要来看 initConfigController 和 initServiceControllers, 代码文件 /pilot/pkg/bootstrap/configcontroller.go

1. ConfigStore 接口
```golang
// # /pilot/pkg/model/config.go
// Object references supplied and returned from this interface should be
// treated as read-only. Modifying them violates thread-safety.
type ConfigStore interface {
	// Schemas exposes the configuration type schema known by the config store.
	// The type schema defines the bidirectional mapping between configuration
	// types and the protobuf encoding schema.
	Schemas() collection.Schemas

	// Get retrieves a configuration element by a type and a key
	Get(typ config.GroupVersionKind, name, namespace string) *config.Config

	// List returns objects by type and namespace.
	// Use "" for the namespace to list across namespaces.
	List(typ config.GroupVersionKind, namespace string) []config.Config

	// Create adds a new configuration object to the store. If an object with the
	// same name and namespace for the type already exists, the operation fails
	// with no side effects.
	Create(config config.Config) (revision string, err error)

	// Update modifies an existing configuration object in the store.  Update
	// requires that the object has been created.  Resource version prevents
	// overriding a value that has been changed between prior _Get_ and _Put_
	// operation to achieve optimistic concurrency. This method returns a new
	// revision if the operation succeeds.
	Update(config config.Config) (newRevision string, err error)
	UpdateStatus(config config.Config) (newRevision string, err error)

	// Patch applies only the modifications made in the PatchFunc rather than doing a full replace. Useful to avoid
	// read-modify-write conflicts when there are many concurrent-writers to the same resource.
	Patch(orig config.Config, patchFn config.PatchFunc) (string, error)

	// Delete removes an object from the store by key
	// For k8s, resourceVersion must be fulfilled before a deletion is carried out.
	// If not possible, a 409 Conflict status will be returned.
	Delete(typ config.GroupVersionKind, name, namespace string, resourceVersion *string) error
}
```

ConfigStore 对支持 GVK GET、List、Create、Update、Delete等操作

GVK包括如下:

```golang
// # /pkg/config/schema/collections/collections.gen.go
	// Pilot contains only collections used by Pilot.
	Pilot = collection.NewSchemasBuilder().
		MustAdd(AuthorizationPolicy).
		MustAdd(DestinationRule).
		MustAdd(EnvoyFilter).
		MustAdd(Gateway).
		MustAdd(PeerAuthentication).
		MustAdd(ProxyConfig).
		MustAdd(RequestAuthentication).
		MustAdd(ServiceEntry).
		MustAdd(Sidecar).
		MustAdd(Telemetry).
		MustAdd(VirtualService).
		MustAdd(WasmPlugin).
		MustAdd(WorkloadEntry).
		MustAdd(WorkloadGroup).
		Build()

```

2. initConfigController 核心流程


```golang
//# /pilot/pkg/bootstrap/configcontroller.go
// initConfigController creates the config controller in the pilotConfig.
func (s *Server) initConfigController(args *PilotArgs) error {
	s.initStatusController(args, features.EnableStatus && features.EnableDistributionTracking)
	meshConfig := s.environment.Mesh()
	if len(meshConfig.ConfigSources) > 0 {
		// Using MCP for config.
		//  MCP 来源配置
		if err := s.initConfigSources(args); err != nil {
			return err
		}
	} else if args.RegistryOptions.FileDir != "" {
		// 文件来源配置
		// Local files - should be added even if other options are specified
		store := memory.Make(collections.Pilot)
		configController := memory.NewController(store)

		err := s.makeFileMonitor(args.RegistryOptions.FileDir, args.RegistryOptions.KubeOptions.DomainSuffix, configController)
		if err != nil {
			return err
		}
		s.ConfigStores = append(s.ConfigStores, configController)
	} else {
		// k8s 来源配置
		err := s.initK8SConfigStore(args)
		if err != nil {
			return err
		}
	}

	// Ingress 来源包装
	// If running in ingress mode (requires k8s), wrap the config controller.
	if hasKubeRegistry(args.RegistryOptions.Registries) && meshConfig.IngressControllerMode != meshconfig.MeshConfig_OFF {
		// Wrap the config controller with a cache.
		// Supporting only Ingress/v1 means we lose support of Kubernetes 1.18
		// Supporting only Ingress/v1beta1 means we lose support of Kubernetes 1.22
		// Since supporting both in a monolith controller is painful due to lack of usable conversion logic between
		// the two versions.
		// As a compromise, we instead just fork the controller. Once 1.18 support is no longer needed, we can drop the old controller
		// ...
	}

	// 所有来源进行聚合
	// Wrap the config controller with a cache.
	aggregateConfigController, err := configaggregate.MakeCache(s.ConfigStores)
	if err != nil {
		return err
	}
	s.configController = aggregateConfigController

	// Create the config store.
	s.environment.ConfigStore = aggregateConfigController

	// Defer starting the controller until after the service is created.
	s.addStartFunc("config controller", func(stop <-chan struct{}) error {
		//  启动 configController
		go s.configController.Run(stop)
		return nil
	})

	return nil
}

```

initConfigController 主要聚合 MCP，文件，k8s, Ingress 来源，同时聚合这些来源生成统一一个 Controller来源赋值给 Server 和 Environment 的 ConfigStore

详细可以看一下 initK8SConfigStore 中的 makeKubeConfigController 方法，这里初始化了一个处理 Istio CRDs 的 Client ，实现 ConfigStoreCache 这个接口中增删改查等方法。

```golang
func (s *Server) makeKubeConfigController(args *PilotArgs) *crdclient.Client {
	opts := crdclient.Option{
		Revision:     args.Revision,
		DomainSuffix: args.RegistryOptions.KubeOptions.DomainSuffix,
		Identifier:   "crd-controller",
	}
	if args.RegistryOptions.KubeOptions.DiscoveryNamespacesFilter != nil {
		opts.NamespacesFilter = args.RegistryOptions.KubeOptions.DiscoveryNamespacesFilter.Filter
	}
	return crdclient.New(s.kubeClient, opts)
}

```

同时在 initK8SConfigStore 初始化 GatewayAPIController, 同时把 GatewayAPIController 添加到 configStores 里

```golang
gwc := gateway.NewController(s.kubeClient, configController, s.kubeClient.CrdWatcher().WaitForCRD,
			s.environment.CredentialsController, args.RegistryOptions.KubeOptions)
		s.environment.GatewayAPIController = gwc
		s.ConfigStores = append(s.ConfigStores, s.environment.GatewayAPIController)
```



回到 initConfigController ，创建好 ConfigStore 之后，再对其进一步包装：


```golang
// 将所有 ConfigStore 聚合并缓存
aggregateConfigController, err := configaggregate.MakeCache(s.ConfigStores)
// 通过 s.configController 统一操作上面聚合的 ConfigStores
s.configController = aggregateConfigController
// 同时传入 environment，便于操作 ServiceEntry/Gateway 等资源
s.environment.ConfigStore = aggregateConfigController
```

最后将该 Controller 的启动函数注册到 startFuncs 中：

```golang
// Defer starting the controller until after the service is created.
	s.addStartFunc("config controller", func(stop <-chan struct{}) error {
		//  启动 configController
		go s.configController.Run(stop)
		return nil
	})
```

3. initServiceControllers 处理服务发现的 Controller 初始化

```golang

// initServiceControllers creates and initializes the service controllers
func (s *Server) initServiceControllers(args *PilotArgs) error {
	// 获取 service controller
	serviceControllers := s.ServiceController()
    //  configController serviceEntry 和 workloadEntry 来源
	s.serviceEntryController = serviceentry.NewController(
		s.configController, s.XDSServer,
		serviceentry.WithClusterID(s.clusterID),
	)
	serviceControllers.AddRegistry(s.serviceEntryController)

	registered := sets.New[provider.ID]()
	for _, r := range args.RegistryOptions.Registries {
		serviceRegistry := provider.ID(r)
		if registered.Contains(serviceRegistry) {
			log.Warnf("%s registry specified multiple times.", r)
			continue
		}
		registered.Insert(serviceRegistry)
		log.Infof("Adding %s registry adapter", serviceRegistry)
		switch serviceRegistry {
		case provider.Kubernetes:
			// k8s Service 来源
			if err := s.initKubeRegistry(args); err != nil {
				return err
			}
		default:
			return fmt.Errorf("service registry %s is not supported", r)
		}
	}

	// Defer running of the service controllers.
	s.addStartFunc("service controllers", func(stop <-chan struct{}) error {
		go serviceControllers.Run(stop)
		return nil
	})

	return nil
}
```

服务来源：
- configController serviceEntry 和 workloadEntry 来源
- k8s 来源

Kubernetes 集群之外的服务，这些服务基本都是通过 ServiceEntry 注册到控制面的，所有 ServiceEntry 配置数据目前还都在之前初始化的 configController 配置中心控制器中，这里将 ServiceEntry 数据单独拎出来初始化一个 ServicEntry 注册中心，加入到 serviceControllers

Kubernetes 注册中心，通过 s.initKubeRegistry 加入到 multiclusterController


### e. 初始化 RegistryEventHandlers

initRegistryEventHandlers 设置了三个事件处理器 serviceHandler 、 configHandler 和 GatewayAPIHandler 分别响应服务、配置、GatewayAPI更新事件。




```golang

// initRegistryEventHandlers sets up event handlers for config and service updates
func (s *Server) initRegistryEventHandlers() {
	log.Info("initializing registry event handlers")
	// Flush cached discovery responses whenever services configuration change.
	serviceHandler := func(prev, curr *model.Service, event model.Event) {
		pushReq := &model.PushRequest{
			Full:           true,
			ConfigsUpdated: sets.New(model.ConfigKey{Kind: kind.ServiceEntry, Name: string(curr.Hostname), Namespace: curr.Attributes.Namespace}),
			Reason:         model.NewReasonStats(model.ServiceUpdate),
		}
		s.XDSServer.ConfigUpdate(pushReq)
	}
	s.ServiceController().AppendServiceHandler(serviceHandler)

	if s.configController != nil {
		configHandler := func(prev config.Config, curr config.Config, event model.Event) {
			defer func() {
				if event != model.EventDelete {
					s.statusReporter.AddInProgressResource(curr)
				} else {
					s.statusReporter.DeleteInProgressResource(curr)
				}
			}()
			log.Debugf("Handle event %s for configuration %s", event, curr.Key())
			// For update events, trigger push only if spec has changed.
			if event == model.EventUpdate && !needsPush(prev, curr) {
				log.Debugf("skipping push for %s as spec has not changed", prev.Key())
				return
			}
			pushReq := &model.PushRequest{
				Full:           true,
				ConfigsUpdated: sets.New(model.ConfigKey{Kind: kind.MustFromGVK(curr.GroupVersionKind), Name: curr.Name, Namespace: curr.Namespace}),
				Reason:         model.NewReasonStats(model.ConfigUpdate),
			}
			s.XDSServer.ConfigUpdate(pushReq)
		}
		schemas := collections.Pilot.All()
		if features.EnableGatewayAPI {
			schemas = collections.PilotGatewayAPI().All()
		}
		for _, schema := range schemas {
			// This resource type was handled in external/servicediscovery.go, no need to rehandle here.
			if schema.GroupVersionKind() == gvk.ServiceEntry {
				continue
			}
			if schema.GroupVersionKind() == gvk.WorkloadEntry {
				continue
			}
			if schema.GroupVersionKind() == gvk.WorkloadGroup {
				continue
			}

			s.configController.RegisterEventHandler(schema.GroupVersionKind(), configHandler)
		}
		if s.environment.GatewayAPIController != nil {
			s.environment.GatewayAPIController.RegisterEventHandler(gvk.Namespace, func(config.Config, config.Config, model.Event) {
				s.XDSServer.ConfigUpdate(&model.PushRequest{
					Full:   true,
					Reason: model.NewReasonStats(model.NamespaceUpdate),
				})
			})
			s.environment.GatewayAPIController.RegisterEventHandler(gvk.Secret, func(_ config.Config, gw config.Config, _ model.Event) {
				s.XDSServer.ConfigUpdate(&model.PushRequest{
					Full: true,
					ConfigsUpdated: map[model.ConfigKey]struct{}{
						{
							Kind:      kind.KubernetesGateway,
							Name:      gw.Name,
							Namespace: gw.Namespace,
						}: {},
					},
					Reason: model.NewReasonStats(model.SecretTrigger),
				})
			})
		}
	}
}

```
可以看到当服务本身发生变化时，会触发 xDS 的全量下发，所有与该服务相关的代理都会收到推送。
上一步初始化了 configController ，它操作的对象主要是像 VirtualService 、 DestinationRules 这些 Istio 定义的配置，这些配置的变化也会触发 xDS 的全量下发，所有与该配置相关的代理都会收到推送。
同时注册了 GatewayAPIController对 Namespace, secret 变更下发通知


### F. 初始化 DiscoveryService

s.initDiscoveryService() 代码如下：

```golang
// initDiscoveryService initializes discovery server on plain text port.
func (s *Server) initDiscoveryService() {
	log.Infof("starting discovery service")
	// Implement EnvoyXdsServer grace shutdown
	s.addStartFunc("xds server", func(stop <-chan struct{}) error {
		log.Infof("Starting ADS server")
		s.XDSServer.Start(stop)
		return nil
	})
}

```

这里将 XDSServer 的启动添加至 startFuncs 中，便于后续统一启动。XDSServer 启动如下：

```golang

func (s *DiscoveryServer) Start(stopCh <-chan struct{}) {
	go s.WorkloadEntryController.Run(stopCh)
	go s.handleUpdates(stopCh)
	go s.periodicRefreshMetrics(stopCh)
	go s.sendPushes(stopCh)
	go s.Cache.Run(stopCh)
}

```

#### H. 注册 kubeClient.RunAndWait

将 kubeClient.RunAndWait 方法注册至 startFuncs 中， RunAndWait 启动后所有 Informer 将开始缓存，并等待它们同步完成。之所以在最后运行，可以保证所有的 Informer 都已经注册。

```golang
// This must be last, otherwise we will not know which informers to register
	if s.kubeClient != nil {
		s.addStartFunc("kube client", func(stop <-chan struct{}) error {
			s.kubeClient.RunAndWait(stop)
			return nil
		})
	}
```

## 启动过程

启动流程比较简单， 








# Reference
- [Pilot源码分析](https://qiankunli.github.io/2020/01/09/istio_pilot_source.html)
- [Istio Pilot 源码分析](https://haidong.dev/Pilot%E6%BA%90%E7%A0%81%E5%88%86%E6%9E%90%EF%BC%88%E4%B8%80%EF%BC%89/)
- [Istio Pilot代码深度解析](https://www.zhaohuabing.com/post/2019-10-21-pilot-discovery-code-analysis/)
- [Pilot源码分析](https://qiankunli.github.io/2020/01/09/istio_pilot_source.html)
- [网易数帆的 Istio 推送性能优化经验 ](https://it.sohu.com/a/550979440_355140)
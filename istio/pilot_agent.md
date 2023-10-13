# Pilot Agent 管理envoy生命周期

Sidecar在注入的时候会注入istio-init和istio-proxy两个容器。 Pilot-agent就是启动istio-proxy的入口。通过kubectl命令我们可以看到启动命令：

```shell
jun@master01:~$ kubectl exec -it httpbin-798dbb9f74-5l4sd -n istio-demo -c istio-proxy -- /bin/sh
$ ps -ef
UID          PID    PPID  C STIME TTY          TIME CMD
istio-p+       1       0  0 Oct03 ?        00:00:17 /usr/local/bin/pilot-agent proxy sidecar --domain istio-demo.svc.cluster.loca
istio-p+      14       1  0 Oct03 ?        00:02:01 /usr/local/bin/envoy -c etc/istio/proxy/envoy-rev.json --drain-time-s 45 --dr
istio-p+      48       0  0 10:03 pts/0    00:00:00 /bin/sh
```

Pilot-agent除了启动istio-proxy以外还有以下能力：

- 生成Envoy的Bootstrap配置文件；
- 健康检查；
- 监视证书的变化，通知Envoy进程热重启，实现证书的热加载；
- 提供Envoy守护功能，当Envoy异常退出的时候重启Envoy；
- 通知Envoy优雅退出；

## pilot-agent proxy 命令参数

```shell
$ /usr/local/bin/pilot-agent proxy --help
XDS proxy agent

Usage:
  pilot-agent proxy [flags]

Flags:
      --concurrency int                 number of worker threads to run
      --domain string                   DNS domain suffix. If not provided uses ${POD_NAMESPACE}.svc.cluster.local
  -h, --help                            help for proxy
      --meshConfig string               File name for Istio mesh configuration. If not specified, a default mesh will be used. This may be overridden by PROXY_CONFIG environment variable or proxy.istio.io/config annotation. (default "./etc/istio/config/mesh")
      --outlierLogPath string           The log path for outlier detection
      --profiling                       Enable profiling via web interface host:port/debug/pprof/. (default true)
      --proxyComponentLogLevel string   The component log level used to start the Envoy proxy. Deprecated, use proxyLogLevel instead
      --proxyLogLevel string            The log level used to start the Envoy proxy (choose from {trace, debug, info, warning, error, critical, off}).Level may also include one or more scopes, such as 'info,misc:error,upstream:debug' (default "warning,misc:error")
      --serviceCluster string           Service cluster (default "istio-proxy")
      --stsPort int                     HTTP Port on which to serve Security Token Service (STS). If zero, STS service will not be provided.
      --templateFile string             Go template bootstrap config
      --tokenManagerPlugin string       Token provider specific plugin name. (default "GoogleTokenExchange")

Global Flags:
      --log_as_json                   Whether to format output as JSON or in plain console-friendly format
      --log_caller string             Comma-separated list of scopes for which to include caller information, scopes can be any of [ads, all, authn, authorization, ca, cache, citadelclient, controllers, default, delta, dns, gcecred, googleca, googlecas, grpcgen, healthcheck, iptables, klog, mockcred, model, monitoring, sds, security, serviceentry, spiffe, stsclient, stsserver, token, trustBundle, validation, wasm, wle, xdsproxy]
      --log_output_level string       Comma-separated minimum per-scope logging level of messages to output, in the form of <scope>:<level>,<scope>:<level>,... where scope can be one of [ads, all, authn, authorization, ca, cache, citadelclient, controllers, default, delta, dns, gcecred, googleca, googlecas, grpcgen, healthcheck, iptables, klog, mockcred, model, monitoring, sds, security, serviceentry, spiffe, stsclient, stsserver, token, trustBundle, validation, wasm, wle, xdsproxy] and level can be one of [debug, info, warn, error, fatal, none] (default "default:info")
      --log_rotate string             The path for the optional rotating log file
      --log_rotate_max_age int        The maximum age in days of a log file beyond which the file is rotated (0 indicates no limit) (default 30)
      --log_rotate_max_backups int    The maximum number of log file backups to keep before older files are deleted (0 indicates no limit) (default 1000)
      --log_rotate_max_size int       The maximum size in megabytes of a log file beyond which the file is rotated (default 104857600)
      --log_stacktrace_level string   Comma-separated minimum per-scope logging level at which stack traces are captured, in the form of <scope>:<level>,<scope:level>,... where scope can be one of [ads, all, authn, authorization, ca, cache, citadelclient, controllers, default, delta, dns, gcecred, googleca, googlecas, grpcgen, healthcheck, iptables, klog, mockcred, model, monitoring, sds, security, serviceentry, spiffe, stsclient, stsserver, token, trustBundle, validation, wasm, wle, xdsproxy] and level can be one of [debug, info, warn, error, fatal, none] (default "default:none")
      --log_target stringArray        The set of paths where to output the log. This can be any path as well as the special values stdout and stderr (default [stdout])
      --vklog Level                   number for the log level verbosity. Like -v flag. ex: --vklog=9
```

## pilot-agent 入口和参数

```golang
# /pilot/cmd/pilot-agent/main.go
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
func NewRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:          "pilot-agent",
		Short:        "Istio Pilot agent.",
		Long:         "Istio Pilot agent runs in the sidecar or gateway container and bootstraps Envoy.",
		SilenceUsage: true,
		FParseErrWhitelist: cobra.FParseErrWhitelist{
			// Allow unknown flags for backward-compatibility.
			UnknownFlags: true,
		},
	}

	// Attach the Istio logging options to the command.
	loggingOptions.AttachCobraFlags(rootCmd)

	cmd.AddFlags(rootCmd)

	proxyCmd := newProxyCommand()
	// 设置命令参数
	addFlags(proxyCmd)
	// proxy command 
	rootCmd.AddCommand(proxyCmd)
	// 
	rootCmd.AddCommand(requestCmd)
	rootCmd.AddCommand(waitCmd)
	rootCmd.AddCommand(version.CobraCommand())
	rootCmd.AddCommand(iptables.GetCommand())
	rootCmd.AddCommand(cleaniptables.GetCommand())

	rootCmd.AddCommand(collateral.CobraCommand(rootCmd, &doc.GenManHeader{
		Title:   "Istio Pilot Agent",
		Section: "pilot-agent CLI",
		Manual:  "Istio Pilot Agent",
	}))

	return rootCmd
}
```

```golang
# /pilot/cmd/pilot-agent/app/cmd.go
func newProxyCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "proxy",
	    ...
		RunE: func(c *cobra.Command, args []string) error {
	        //...
			// 初始化代理结构体,解析podip,domain
			// Proxy contains information about an specific instance of a proxy (envoy sidecar, gateway,
			// etc).
			proxy, err := initProxy(args)
			if err != nil {
				return err
			}
			
			// 构建 ProxyConfig， 从 MeshConfigFile 默认是  (default "./etc/istio/config/mesh") 和 proxy.istio.io/config annotation 构建 ProxyConfig
			proxyConfig, err := config.ConstructProxyConfig(proxyArgs.MeshConfigFile, proxyArgs.ServiceCluster, options.ProxyConfigEnv, proxyArgs.Concurrency)
		    // ....
			
			// 初始化安全设置用于 SDS 和 CA配置
			// 创建sts配置
			secOpts, err := options.NewSecurityOptions(proxyConfig, proxyArgs.StsPort, proxyArgs.TokenManagerPlugin)
			if err != nil {
				return err
			}

			// If security token service (STS) port is not zero, start STS server and
			// listen on STS port for STS requests. For STS, see
			// https://tools.ietf.org/html/draft-ietf-oauth-token-exchange-16.
			// STS is used for stackdriver or other Envoy services using google gRPC.
			if proxyArgs.StsPort > 0 {
				// 初始化 A Security Token Service (STS)  服务
				stsServer, err := initStsServer(proxy, secOpts.TokenManager)
				if err != nil {
					return err
				}
				defer stsServer.Stop()
			}

			// If we are using a custom template file (for control plane proxy, for example), configure this.
			//  Envoy 启动配置模版设置
			if proxyArgs.TemplateFile != "" && proxyConfig.CustomConfigFile == "" {
				proxyConfig.ProxyBootstrapTemplatePath = proxyArgs.TemplateFile
			}

			envoyOptions := envoy.ProxyConfig{
				LogLevel:          proxyArgs.ProxyLogLevel,
				ComponentLogLevel: proxyArgs.ProxyComponentLogLevel,
				LogAsJSON:         loggingOptions.JSONEncoding,
				NodeIPs:           proxy.IPAddresses,
				Sidecar:           proxy.Type == model.SidecarProxy,
				OutlierLogPath:    proxyArgs.OutlierLogPath,
			}
			// 初始化Agent参数
			agentOptions := options.NewAgentOptions(proxy, proxyConfig)
			//  初始化 Agent
			agent := istio_agent.NewAgent(proxyConfig, agentOptions, secOpts, envoyOptions)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			defer agent.Close()

			// If a status port was provided, start handling status probes.
			if proxyConfig.StatusPort > 0 {
				//  初始化 status 服务
				if err := initStatusServer(ctx, proxy, proxyConfig,
					agentOptions.EnvoyPrometheusPort, proxyArgs.EnableProfiling, agent); err != nil {
					return err
				}
			}

			go iptableslog.ReadNFLOGSocket(ctx)

			// On SIGINT or SIGTERM, cancel the context, triggering a graceful shutdown
			go cmd.WaitSignalFunc(cancel)

			// Start in process SDS, dns server, xds proxy, and Envoy.
			// 启动Agent
			wait, err := agent.Run(ctx)
			if err != nil {
				return err
			}
			wait()
			return nil
		},
	}
}

```

核心流程：

1. 生成 proxy是运行envoy的一个代理.
2. 生成 envoy的配置信息proxyConfig.
3. 初始化 status 服务
4. 初始化 agent 负责envoy的生命周期.
5. 运行agent
6. 等待``agent```结束.

## agent

```golang
# /pkg/istio-agent/agent.go
// Agent contains the configuration of the agent, based on the injected
// environment:
// - SDS hostPath if node-agent was used
// - /etc/certs/key if Citadel or other mounted Secrets are used
// - root cert to use for connecting to XDS server
// - CA address, with proper defaults and detection
type Agent struct {
    // 代理配置,配置envoy运行文件目录,代理镜像地址等
    // 与mesh.proxyconfig属性一致
	proxyConfig *mesh.ProxyConfig
	// 主要将proxyConfig,secOpts信息进行封装
	cfg       *AgentOptions
    // 安全信息配置,主要存储了静态证书的地址,证书提供者等
	secOpts   *security.Options
	//  envoy运行时所需要的信息,比如envoy二进制文件,日志级别等
	envoyOpts envoy.ProxyConfig

	envoyAgent             *envoy.Agent
	dynamicBootstrapWaitCh chan error

    // SDSGRPC服务器,主要用于工作负载的证书申请
    // sds会生成证书然后调用secretCache对证书进行签证,完成后发送给envoy
	sdsServer   *sds.Server
	// 用于SDS证书签证,可以通过文件的形式进行签证
	// 默认使用istiod对工作负载进行签证
	secretCache *cache.SecretManagerClient

	// Used when proxying envoy xds via istio-agent is enabled.
    // agnet的XDS服务器,主要用于连接上游istiod与下游envoy通讯的桥梁
    // envoy发送注册事件给agent,agent 会将注册事件转发给istiod
    // istiod下发的配置会通过agent传输给envoy
	xdsProxy    *XdsProxy

	fileWatcher filewatcher.FileWatcher

	// local DNS Server that processes DNS requests locally and forwards to upstream DNS if needed.
    // 本地DNS服务器
	localDNSServer *dnsClient.LocalDNSServer

	// Signals true completion (e.g. with delayed graceful termination of Envoy)
	wg sync.WaitGroup
}
```
## SecurityOptions

安全配置包含了agent要使用的证书,token等,强烈建议仔细理解,对于下面的证书请求,认证有很大的帮助.

```golang
// Options provides all of the configuration parameters for secret discovery service
// and CA configuration. Used in both Istiod and Agent.
// TODO: ProxyConfig should have most of those, and be passed to all components
// (as source of truth)
type Options struct {
	// CAEndpoint is the CA endpoint to which node agent sends CSR request.
	// CA签发服务器,默认为istiod.istio-system.svc:15012
	CAEndpoint string

	// CAEndpointSAN overrides the ServerName extracted from CAEndpoint.
	// 设置ServerName 来覆盖CAEndpoint提取的ServerName
	CAEndpointSAN string

	// The CA provider name.
	// CA提供者的名称,默认为Citadel说明使用内部的证书管理
	CAProviderName string

	// TrustDomain corresponds to the trust root of a system.
	// https://github.com/spiffe/spiffe/blob/master/standards/SPIFFE-ID.md#21-trust-domain
	TrustDomain string

	// WorkloadRSAKeySize is the size of a private key for a workload certificate.
	WorkloadRSAKeySize int

	// Whether to generate PKCS#8 private keys.
    // 是否生成PKCS#8私钥
	Pkcs8Keys bool

	// OutputKeyCertToDir is the directory for output the key and certificate
    // 为工作负载生成的证书输出目录
	OutputKeyCertToDir string

	// ProvCert is the directory for client to provide the key and certificate to CA server when authenticating
	// with mTLS. This is not used for workload mTLS communication, and is
	// 客户端在认证时向CA服务器提供密钥和证书的目录
	ProvCert string

	// ClusterID is the cluster where the agent resides.
	// Normally initialized from ISTIO_META_CLUSTER_ID - after a tortuous journey it
	// makes its way into the ClusterID metadata of Citadel gRPC request to create the cert.
	// Didn't find much doc - but I suspect used for 'central cluster' use cases - so should
	// match the cluster name set in the MC setup.
	ClusterID string

	// The type of Elliptical Signature algorithm to use
	// when generating private keys. Currently only ECDSA is supported.
	//签名算法
	ECCSigAlg string

	// The type of curve to use when generating private keys with ECC. Currently only ECDSA is supported.
	ECCCurve string

	// FileMountedCerts indicates whether the proxy is using file
	// mounted certs created by a foreign CA. Refresh is managed by the external
	// CA, by updating the Secret or VM file. We will watch the file for changes
	// or check before the cert expires. This assumes the certs are in the
	// well-known ./etc/certs location.
	FileMountedCerts bool

	// PilotCertProvider is the provider of the Pilot certificate (PILOT_CERT_PROVIDER env)
	// Determines the root CA file to use for connecting to CA gRPC:
	// - istiod
	// - kubernetes
	// - custom
	// - none
	// 证书提供者 默认为istiod
	PilotCertProvider string

	// secret TTL.
	SecretTTL time.Duration

	// The ratio of cert lifetime to refresh a cert. For example, at 0.10 and 1 hour TTL,
	// we would refresh 6 minutes before expiration.
	SecretRotationGracePeriodRatio float64

	// STS port
	STSPort int

	// authentication provider specific plugins, will exchange the token
	// For example exchange long lived refresh with access tokens.
	// Used by the secret fetcher when signing CSRs.
	// Optional; if not present the token will be used directly
	//身份验证提供程序特定插件
	TokenExchanger TokenExchanger

	// credential fetcher.
	// 这里主要用于grpcPerRPCCredentials中的get.token使用
	// 在请求发送前运行,将token值添加到请求头中
	CredFetcher CredFetcher

	// credential identity provider
	// 凭证身份提供者
	CredIdentityProvider string

	// Namespace corresponding to workload
	// 与工作负载对应的命名空间
	WorkloadNamespace string

	// Name of the Service Account
	ServiceAccount string

	// XDS auth provider
	XdsAuthProvider string

	// Token manager for the token exchange of XDS
    // token管理器
	TokenManager TokenManager

	// Cert signer info
	//证书签名者信息
	CertSigner string

	// Delay in reading certificates from file after the change is detected. This is useful in cases
	// where the write operation of key and cert take longer.
	FileDebounceDuration time.Duration

	// Root Cert read from the OS
	//系统CA证书根地址值
	CARootPath string

	// The path for an existing certificate chain file
   // 本地现有证书集合,agent会首先使用该证书作为工作负载的证书.
	CertChainFilePath string
	// The path for an existing key file
	KeyFilePath string
	// The path for an existing root certificate bundle
	RootCertFilePath string
}

```

## cfg

对于cfg,它就像一个缝合怪将上面的配置封装了一下,下面将重点讲解几个比较重要的属性

```golang
# /pkg/istio-agent/agent.go
// AgentOptions contains additional config for the agent, not included in ProxyConfig.
// Most are from env variables ( still experimental ) or for testing only.
// Eventually most non-test settings should graduate to ProxyConfig
// Please don't add 100 parameters to the NewAgent function (or any other)!
type AgentOptions struct {
	// ProxyXDSDebugViaAgent if true will listen on 15004 and forward queries
	// to XDS istio.io/debug.
	ProxyXDSDebugViaAgent bool
	// Port value for the debugging endpoint.
	ProxyXDSDebugViaAgentPort int
	// DNSCapture indicates if the XDS proxy has dns capture enabled or not
	DNSCapture bool
	// DNSAddr is the DNS capture address
	DNSAddr string
	// DNSForwardParallel indicates whether the agent should send parallel DNS queries to all upstream nameservers.
	DNSForwardParallel bool
	// ProxyType is the type of proxy we are configured to handle
	ProxyType model.NodeType
	// ProxyNamespace to use for local dns resolution
	ProxyNamespace string
	// ProxyDomain is the DNS domain associated with the proxy (assumed
	// to include the namespace as well) (for local dns resolution)
	ProxyDomain string
	// Node identifier used by Envoy
	ServiceNode string

	// XDSRootCerts is the location of the root CA for the XDS connection. Used for setting platform certs or
	// using custom roots.
	// 创建XDS服务器中的上游连接时所需要的Root证书,默认为/var/run/secrets/istio/root-cert.pem
	// 这个是cm中的istio-ca-root-cert里的key
	// istio-ca-root-cert是discovery创建的,当命名空间被创建后,会自动为其创建一个istio-ca-root-cert
	XDSRootCerts string

	// CARootCerts of the location of the root CA for the CA connection. Used for setting platform certs or
	// using custom roots.
    // 用于SDS连接上游istiod时所用到的证书默认值也是/var/run/secrets/istio/root-cert.pem
	CARootCerts string

	// Extra headers to add to the XDS connection.
	XDSHeaders map[string]string

	// Is the proxy an IPv6 proxy
	IsIPv6 bool

	// Path to local UDS to communicate with Envoy
	// agent使用unix与envoy通讯,当前unix的目录
	XdsUdsPath string

	// Ability to retrieve ProxyConfig dynamically through XDS
	EnableDynamicProxyConfig bool

	// All of the proxy's IP Addresses
	ProxyIPAddresses []string

	// Enables dynamic generation of bootstrap.
	EnableDynamicBootstrap bool

	// Envoy status port (that circles back to the agent status port). Really belongs to the proxy config.
	// Cannot be eradicated because mistakes have been made.
	EnvoyStatusPort int

	// Envoy prometheus port that circles back to its admin port for prom endpoint. Really belongs to the
	// proxy config.
	EnvoyPrometheusPort int

	MinimumDrainDuration time.Duration

	ExitOnZeroActiveConnections bool

	// Cloud platform
	Platform platform.Environment

	// GRPCBootstrapPath if set will generate a file compatible with GRPC_XDS_BOOTSTRAP
	GRPCBootstrapPath string

	// Disables all envoy agent features
	DisableEnvoy          bool
	DownstreamGrpcOptions []grpc.ServerOption

	IstiodSAN string

	WASMOptions wasm.Options

	// Is the proxy in Dual Stack environment
	DualStack bool

	UseExternalWorkloadSDS bool
}
```

## 配置

我们主要讲解一下更改这些配置的方式

- 使用静态的mesh配置文件,这个istio已经创建了一个configmap
- 使用环境变量,具体细节请参考 https://istio.io/latest/zh/docs/reference/commands/pilot-agent/#pilot-agent-completion
- 使用运行参数.请参考 https://istio.io/latest/zh/docs/reference/commands/pilot-agent/#pilot-agent-completion

主要关心证书的配置,因为agent最重要的一个功能就是实现了工作负载到工作负载之间的双向TLS.
对于 /var/run/secrets/istio/root-cert.pem 这个证书的 ROOTCA 是由 discovery 中的 maybeCreateCA 进行创建的,然后通过initMulticluster监听命名空间资源添加生成root-cert.pem证书的事件.
具体流程为 discovery 创建了一个CA服务器,生成CA证书然后然后监视每个命名空间,为其创建  istio-ca-root-cert configmap
这里还有一个需要注意的就是当agent连接istiod时除了证书的双向认证外还需要JWT认证(默认),默认情况下是使用k8s进行认证也就是说传输的token值必须与k8s有关系,在agent中也是这么做的,agent会将k8s中 default SA 用户的token值映射到 /var/run/secrets/tokens/istio-token,然后通过grpc的前置函数将当前token值添加到请求头中.


## Agent Run

agent 的对于grpc的创建,envoy 的启动都在run方法中运行,接下里就列举一下它都做了那些事.

1. 创建 DNS 服务器
2. 创建 SDS服务器,主要用于envoy证书的申请
3. 创建 XDSproxy,主要用于envoy服务发现
4. 启动 envoy,使用cmd命令启动


envoy, agent, istiod 通讯方式：

- agent<->envoy 使用的是 unix (/etc/istio/proxy/XDS) 通讯,这种方式没有使用 tls,但是又保证了安全性,性能要比 tls 要好 
- agent<->istiod 使用的是 tls 通讯,通过证书与 istiod 建立连接,然后通过 token 进行身份校验


```golang
// Run is a non-blocking call which returns either an error or a function to await for completion.
func (a *Agent) Run(ctx context.Context) (func(), error) {
	var err error
	if err = a.initLocalDNSServer(); err != nil {
		return nil, fmt.Errorf("failed to start local DNS server: %v", err)
	}

	socketExists, err := checkSocket(ctx, security.WorkloadIdentitySocketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to check SDS socket: %v", err)
	}
	if socketExists {
		log.Info("Workload SDS socket found. Istio SDS Server won't be started")
	} else {
		if a.cfg.UseExternalWorkloadSDS {
			return nil, errors.New("workload SDS socket is required but not found")
		}
		log.Info("Workload SDS socket not found. Starting Istio SDS Server")
		err = a.initSdsServer()
		if err != nil {
			return nil, fmt.Errorf("failed to start SDS server: %v", err)
		}
	}
	a.xdsProxy, err = initXdsProxy(a)
	if err != nil {
		return nil, fmt.Errorf("failed to start xds proxy: %v", err)
	}
	if a.cfg.ProxyXDSDebugViaAgent {
		err = a.xdsProxy.initDebugInterface(a.cfg.ProxyXDSDebugViaAgentPort)
		if err != nil {
			return nil, fmt.Errorf("failed to start istio tap server: %v", err)
		}
	}

	if a.cfg.GRPCBootstrapPath != "" {
		if err := a.generateGRPCBootstrap(); err != nil {
			return nil, fmt.Errorf("failed generating gRPC XDS bootstrap: %v", err)
		}
	}
	if a.proxyConfig.ControlPlaneAuthPolicy != mesh.AuthenticationPolicy_NONE {
		rootCAForXDS, err := a.FindRootCAForXDS()
		if err != nil {
			return nil, fmt.Errorf("failed to find root XDS CA: %v", err)
		}
		go a.startFileWatcher(ctx, rootCAForXDS, func() {
			if err := a.xdsProxy.initIstiodDialOptions(a); err != nil {
				log.Warnf("Failed to init xds proxy dial options")
			}
		})
	}

	if !a.EnvoyDisabled() {
		err = a.initializeEnvoyAgent(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize envoy agent: %v", err)
		}

		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			// This is a blocking call for graceful termination.
			a.envoyAgent.Run(ctx)
		}()
	} else if a.WaitForSigterm() {
		// wait for SIGTERM and perform graceful shutdown
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			<-ctx.Done()
		}()
	}
	return a.wg.Wait, nil
}

```


### 1. SDS服务器

SDS主要用于envoy申请证书使用,申请的证书用作envoy->envoy(工作负载->工作负载)之间的 mtls.
具体流程为:
- envoy发送证书申请指令(这个应该是写死的请求的url,在envoy里没有找到响应的配置)
- 由于使用的是 unix 通讯没有使用 tls,所以 agent 中的 xdsserver 直接跳过身份校验环节, xdsserver 与 envoy 建立连接接受到请求后,首先查询是否使用的为静态CA,否则生成证书然后调用 istiod 客户端,向 istiod 进行签名.
- istiod 接受到连接后对其进行签名返回
- xdsserver 接受到返回信息后会将其缓存到 agent 中,然后调用 envoy 配置更新方法(具体的流程pilotDiscovery启动文章中有讲到),将证书转换为envoy配置资源推送给envoy.


看一下SDS服务器的创建代码:

```golang
# /pkg/istio-agent/agent.go
func (a *Agent) initSdsServer() error {
	var err error
	// 检查是否配置了静态CA证书,如果配置了直接使用该证书,就不用动态判断证书提供者从而获取证书了
	if security.CheckWorkloadCertificate(security.WorkloadIdentityCertChainPath, security.WorkloadIdentityKeyPath, security.WorkloadIdentityRootCertPath) {
		log.Info("workload certificate files detected, creating secret manager without caClient")
		a.secOpts.RootCertFilePath = security.WorkloadIdentityRootCertPath
		a.secOpts.CertChainFilePath = security.WorkloadIdentityCertChainPath
		a.secOpts.KeyFilePath = security.WorkloadIdentityKeyPath
		a.secOpts.FileMountedCerts = true
	}

   // 根据 istio-ca-secret 生成的 root.pem(istio-ca-root-cert) 与istiod进行双向认证,然后建立证书创建管理连接
   // 并返回该client
	a.secretCache, err = a.newSecretManager()
	if err != nil {
		return fmt.Errorf("failed to start workload secret manager %v", err)
	}

    // 这里判断如果不使用envoy
	if a.cfg.DisableEnvoy {
		// For proxyless we don't need an SDS server, but still need the keys and
		// we need them refreshed periodically.
		//
		// This is based on the code from newSDSService, but customized to have explicit rotation.
		go func() {
			st := a.secretCache
			st.RegisterSecretHandler(func(resourceName string) {
				// The secret handler is called when a secret should be renewed, after invalidating the cache.
				// The handler does not call GenerateSecret - it is a side-effect of the SDS generate() method, which
				// is called by sdsServer.OnSecretUpdate, which triggers a push and eventually calls sdsservice.Generate
				// TODO: extract the logic to detect expiration time, and use a simpler code to rotate to files.
				_, _ = a.getWorkloadCerts(st)
			})
			_, _ = a.getWorkloadCerts(st)
		}()
	} else {
       // 使用tls加速,是istio1.14新特性主要加速双向tls
		pkpConf := a.proxyConfig.GetPrivateKeyProvider()
        // 创建sds服务器,与istiod建立连接
		a.sdsServer = sds.NewServer(a.secOpts, a.secretCache, pkpConf)
       // 这里的OnSecretUpdate会通知XDS服务器重新生成证书推送给envoy
       // 注意,推送的类型为secret,但是对该类型的生成器并不是discovery默认生成器,而是sdsservice类型生成器
       // sdsservice该生成器的作用是重新向istiod获取证书,然后推送
		a.secretCache.RegisterSecretHandler(a.sdsServer.OnSecretUpdate)
	}

	return nil
}
```

sdsServer Server 结构定义
```golang
// Server is the gPRC server that exposes SDS through UDS.
type Server struct {
	// sdsservice 是实现 envoy 获取 SecretDiscoveryServiceServer secrets 接口
	workloadSds *sdsservice

	//  unix net listerner
	grpcWorkloadListener net.Listener

	// grpc server
	grpcWorkloadServer *grpc.Server
	stopped            *atomic.Bool
}

```

SecretDiscoveryServiceServer 接口定义 

```golang
// github.com/envoyproxy/go-control-plane@v0.11.2-0.20230925135906-b8d05208285f/envoy/service/secret/v3/sds.pb.go
// SecretDiscoveryServiceServer is the server API for SecretDiscoveryService service.
type SecretDiscoveryServiceServer interface {
	DeltaSecrets(SecretDiscoveryService_DeltaSecretsServer) error
	StreamSecrets(SecretDiscoveryService_StreamSecretsServer) error
	FetchSecrets(context.Context, *v3.DiscoveryRequest) (*v3.DiscoveryResponse, error)
}

```


```golang
// NewServer creates and starts the Grpc server for SDS.
func NewServer(options *security.Options, workloadSecretCache security.SecretManager, pkpConf *mesh.PrivateKeyProvider) *Server {
	s := &Server{stopped: atomic.NewBool(false)}
	s.workloadSds = newSDSService(workloadSecretCache, options, pkpConf)
	s.initWorkloadSdsService()
	return s
}
```

```golang

func (s *Server) initWorkloadSdsService() {
	s.grpcWorkloadServer = grpc.NewServer(s.grpcServerOptions()...)
	s.workloadSds.register(s.grpcWorkloadServer)
	var err error
    // 对于agent<-> envoy的通讯采用的是unix的方式,通过文件管道的形式  /var/run/secrets/workload-spiffe-uds/socket
	s.grpcWorkloadListener, err = uds.NewListener(security.WorkloadIdentitySocketPath)
	go func() {
		sdsServiceLog.Info("Starting SDS grpc server")
		waitTime := time.Second
		started := false
		for i := 0; i < maxRetryTimes; i++ {
			if s.stopped.Load() {
				return
			}
			serverOk := true
			setUpUdsOK := true
			if s.grpcWorkloadListener == nil {
				if s.grpcWorkloadListener, err = uds.NewListener(security.WorkloadIdentitySocketPath); err != nil {
					sdsServiceLog.Errorf("SDS grpc server for workload proxies failed to set up UDS: %v", err)
					setUpUdsOK = false
				}
			}
			if s.grpcWorkloadListener != nil {
				if err = s.grpcWorkloadServer.Serve(s.grpcWorkloadListener); err != nil {
					sdsServiceLog.Errorf("SDS grpc server for workload proxies failed to start: %v", err)
					serverOk = false
				}
			}
			if serverOk && setUpUdsOK {
				sdsServiceLog.Infof("SDS server for workload certificates started, listening on %q", security.WorkloadIdentitySocketPath)
				started = true
				break
			}
			time.Sleep(waitTime)
			waitTime *= 2
		}
		if !started {
			sdsServiceLog.Warn("SDS grpc server could not be started")
		}
	}()
}

```

### 2. XDS代理服务器

xdsproxy主要作为 istiod<->envoy 通讯的桥梁, istiod下发配置到 envoy 的具体流程

- istiod 生成配置后推送给 conn 连接
- xdsproxy 接受到 istiod 传来的数据后,进行判断如果不是自己的则转发给下面的 envoy
- envoy 接收到配置后进行处理

所以说 envoy 并不直接与 istiod 进行通讯,那么也就不用关心与它的认证的关系,这一些列的操作都由 xdsproxy 完成
下面看一下 envoy 服务注册的具体代码

#### XdsProxy 结构

```golang
// /pkg/istio-agent/xds_proxy.go
// XdsProxy proxies all XDS requests from envoy to istiod, in addition to allowing
// subsystems inside the agent to also communicate with either istiod/envoy (eg dns, sds, etc).
// The goal here is to consolidate all xds related connections to istiod/envoy into a
// single tcp connection with multiple gRPC streams.
// TODO: Right now, the workloadSDS server and gatewaySDS servers are still separate
// connections. These need to be consolidated.
// TODO: consolidate/use ADSC struct - a lot of duplication.
type XdsProxy struct {
	stopChan             chan struct{}
	clusterID            string
	downstreamListener   net.Listener
	downstreamGrpcServer *grpc.Server
	istiodAddress        string
	optsMutex            sync.RWMutex
	dialOptions          []grpc.DialOption
	handlers             map[string]ResponseHandler
	healthChecker        *health.WorkloadHealthChecker
	xdsHeaders           map[string]string
	xdsUdsPath           string
	proxyAddresses       []string
	ia                   *Agent

	httpTapServer      *http.Server
	tapMutex           sync.RWMutex
	tapResponseChannel chan *discovery.DiscoveryResponse

	// connected stores the active gRPC stream. The proxy will only have 1 connection at a time
	connected                 *ProxyConnection
	initialHealthRequest      *discovery.DiscoveryRequest
	initialDeltaHealthRequest *discovery.DeltaDiscoveryRequest
	connectedMutex            sync.RWMutex

	// Wasm cache and ecds channel are used to replace wasm remote load with local file.
	wasmCache wasm.Cache

	// ecds version and nonce uses atomic only to prevent race in testing.
	// In reality there should not be race as istiod will only have one
	// in flight update for each type of resource.
	// TODO(bianpengyuan): this relies on the fact that istiod versions all ECDS resources
	// the same in a update response. This needs update to support per resource versioning,
	// in case istiod changes its behavior, or a different ECDS server is used.
	ecdsLastAckVersion    atomic.String
	ecdsLastNonce         atomic.String
	downstreamGrpcOptions []grpc.ServerOption
	istiodSAN             string
}

```

#### 初始化 XdsProxy

```golang
// /pkg/istio-agent/xds_proxy.go
func initXdsProxy(ia *Agent) (*XdsProxy, error) {
	var err error
	localHostAddr := localHostIPv4
	if ia.cfg.IsIPv6 {
		localHostAddr = localHostIPv6
	}
	// 初始化 envoy 探针
	var envoyProbe ready.Prober
	if !ia.cfg.DisableEnvoy {
		envoyProbe = &ready.Probe{
			AdminPort:     uint16(ia.proxyConfig.ProxyAdminPort),
			LocalHostAddr: localHostAddr,
		}
	}

	// 初始化 wasm 缓存
	cache := wasm.NewLocalFileCache(constants.IstioDataDir, ia.cfg.WASMOptions)
	proxy := &XdsProxy{
		istiodAddress:         ia.proxyConfig.DiscoveryAddress,
		istiodSAN:             ia.cfg.IstiodSAN,
		clusterID:             ia.secOpts.ClusterID,
		handlers:              map[string]ResponseHandler{},
		stopChan:              make(chan struct{}),
		healthChecker:         health.NewWorkloadHealthChecker(ia.proxyConfig.ReadinessProbe, envoyProbe, ia.cfg.ProxyIPAddresses, ia.cfg.IsIPv6),
		xdsHeaders:            ia.cfg.XDSHeaders,
		xdsUdsPath:            ia.cfg.XdsUdsPath,
		wasmCache:             cache,
		proxyAddresses:        ia.cfg.ProxyIPAddresses,
		ia:                    ia,
		downstreamGrpcOptions: ia.cfg.DownstreamGrpcOptions,
	}

	// 初始化 LocalDNS Server
	if ia.localDNSServer != nil {
		// ...
	}

    // 初始化 ProxyConfig 配置 
	if ia.cfg.EnableDynamicProxyConfig && ia.secretCache != nil {
		// ...
	}

	proxyLog.Infof("Initializing with upstream address %q and cluster %q", proxy.istiodAddress, proxy.clusterID)

	//  初始化 和envoy连接 grpc server
	if err = proxy.initDownstreamServer(); err != nil {
		return nil, err
	}

	// 初始化和 istiod 连接 options
	if err = proxy.initIstiodDialOptions(ia); err != nil {
		return nil, err
	}

	go func() {
		// 启动和 envoy 连接 grpc 服务
		if err := proxy.downstreamGrpcServer.Serve(proxy.downstreamListener); err != nil {
			log.Errorf("failed to accept downstream gRPC connection %v", err)
		}
	}()

	// ...

	return proxy, nil
}

```

#### Envoy <-> istiod 通讯基本逻辑

```golang
// grpc 服务接口
// github.com/envoyproxy/go-control-plane@v0.11.2-0.20230925135906-b8d05208285f/envoy/service/discovery/v3/ads.pb.go
var _AggregatedDiscoveryService_serviceDesc = grpc.ServiceDesc{
	ServiceName: "envoy.service.discovery.v3.AggregatedDiscoveryService",
	HandlerType: (*AggregatedDiscoveryServiceServer)(nil),
	Methods:     []grpc.MethodDesc{},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "StreamAggregatedResources",
			Handler:       _AggregatedDiscoveryService_StreamAggregatedResources_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
		{
			StreamName:    "DeltaAggregatedResources",
			Handler:       _AggregatedDiscoveryService_DeltaAggregatedResources_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "envoy/service/discovery/v3/ads.proto",
}


// AggregatedDiscoveryServiceServer is the server API for AggregatedDiscoveryService service.
type AggregatedDiscoveryServiceServer interface {
    // This is a gRPC-only API.
    StreamAggregatedResources(AggregatedDiscoveryService_StreamAggregatedResourcesServer) error
    DeltaAggregatedResources(AggregatedDiscoveryService_DeltaAggregatedResourcesServer) error
}


```

```golang
// StreamAggregatedResources is an implementation of XDS API used for proxying between Istiod and Envoy.
// Every time envoy makes a fresh connection to the agent, we reestablish a new connection to the upstream xds
// This ensures that a new connection between istiod and agent doesn't end up consuming pending messages from envoy
// as the new connection may not go to the same istiod. Vice versa case also applies.
func (p *XdsProxy) StreamAggregatedResources(downstream xds.DiscoveryStream) error {
	proxyLog.Debugf("accepted XDS connection from Envoy, forwarding to upstream XDS server")
	return p.handleStream(downstream)
}

func (p *XdsProxy) handleStream(downstream adsStream) error {
    // 创建代理连接
	con := &ProxyConnection{
		conID:           connectionNumber.Inc(),
		upstreamError:   make(chan error, 2), // can be produced by recv and send
		downstreamError: make(chan error, 2), // can be produced by recv and send
		// Requests channel is unbounded. The Envoy<->XDS Proxy<->Istiod system produces a natural
		// looping of Recv and Send. Due to backpressure introduced by gRPC natively (that is, Send() can
		// only send so much data without being Recv'd before it starts blocking), along with the
		// backpressure provided by our channels, we have a risk of deadlock where both Xdsproxy and
		// Istiod are trying to Send, but both are blocked by gRPC backpressure until Recv() is called.
		// However, Recv can fail to be called by Send being blocked. This can be triggered by the two
		// sources in our system (Envoy request and Istiod pushes) producing more events than we can keep
		// up with.
		// See https://github.com/istio/istio/issues/39209 for more information
		//
		// To prevent these issues, we need to either:
		// 1. Apply backpressure directly to Envoy requests or Istiod pushes
		// 2. Make part of the system unbounded
		//
		// (1) is challenging because we cannot do a conditional Recv (for Envoy requests), and changing
		// the control plane requires substantial changes. Instead, we make the requests channel
		// unbounded. This is the least likely to cause issues as the messages we store here are the
		// smallest relative to other channels.
		requestsChan: channels.NewUnbounded[*discovery.DiscoveryRequest](),
		// Allow a buffer of 1. This ensures we queue up at most 2 (one in process, 1 pending) responses before forwarding.
		responsesChan: make(chan *discovery.DiscoveryResponse, 1),
		stopChan:      make(chan struct{}),
		downstream:    downstream,
	}

	// 赋值给当前xdsproxy
	p.registerStream(con)
	defer p.unregisterStream(con)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

    // 创建上游连接,就是构建与 istiod 的连接
    // 证书信息在创建时已经初始化
	upstreamConn, err := p.buildUpstreamConn(ctx)
	if err != nil {
		proxyLog.Errorf("failed to connect to upstream %s: %v", p.istiodAddress, err)
		metrics.IstiodConnectionFailures.Increment()
		return err
	}
	defer upstreamConn.Close()

	// 这里创建与 isitod grpc 通讯的客户端
	xds := discovery.NewAggregatedDiscoveryServiceClient(upstreamConn)
	ctx = metadata.AppendToOutgoingContext(context.Background(), "ClusterID", p.clusterID)
	for k, v := range p.xdsHeaders {
		ctx = metadata.AppendToOutgoingContext(ctx, k, v)
	}
	// We must propagate upstream termination to Envoy. This ensures that we resume the full XDS sequence on new connection
	// 开始调用服务注册请求
	return p.handleUpstream(ctx, con, xds)
}

func (p *XdsProxy) buildUpstreamConn(ctx context.Context) (*grpc.ClientConn, error) {
	p.optsMutex.RLock()
	opts := p.dialOptions
	p.optsMutex.RUnlock()
	return grpc.DialContext(ctx, p.istiodAddress, opts...)
}

func (p *XdsProxy) handleUpstream(ctx context.Context, con *ProxyConnection, xds discovery.AggregatedDiscoveryServiceClient) error {
    // 调用grpc服务注册方法
	upstream, err := xds.StreamAggregatedResources(ctx,
		grpc.MaxCallRecvMsgSize(defaultClientMaxReceiveMessageSize))
	if err != nil {
		// ...
		return err
	}
    // ...
	con.upstream = upstream

	// Handle upstream xds recv
	go func() {
		for {
			// from istiod
			resp, err := con.upstream.Recv()
			if err != nil {
				select {
				case con.upstreamError <- err:
				case <-con.stopChan:
				}
				return
			}
			select {
			// 发送给 handleUpstreamResponse 处理
			case con.responsesChan <- resp:
			case <-con.stopChan:
			}
		}
	}()

   // 向上游发送数据方法处理函数
	go p.handleUpstreamRequest(con)
    // 接受上游发送的数据方法处理函数
	go p.handleUpstreamResponse(con)

	for {
		select {
		case err := <-con.upstreamError:
			// error from upstream Istiod.
			// ...
			return err
		case err := <-con.downstreamError:
			// error from downstream Envoy.
			// ...
			// On downstream error, we will return. This propagates the error to downstream envoy which will trigger reconnect
			return err
		case <-con.stopChan:
			// ...
			return nil
		}
	}
}
```

handleUpstream 是 istio<->envoy 通讯的基本逻辑。


#### istiod->envoy 处理逻辑 handleUpstreamResponse

```golang

func (p *XdsProxy) handleUpstreamResponse(con *ProxyConnection) {
	forwardEnvoyCh := make(chan *discovery.DiscoveryResponse, 1)
	for {
		select {
        // 接受数据
		case resp := <-con.responsesChan:
			// TODO: separate upstream response handling from requests sending, which are both time costly
			proxyLog.Debugf("response for type url %s", resp.TypeUrl)
			metrics.XdsProxyResponses.Increment()
            // 判断当前请求的url,如果是调用的 xdsproxy 则进行处理,否则转发给 envoy
			if h, f := p.handlers[resp.TypeUrl]; f {
				if len(resp.Resources) == 0 {
					// Empty response, nothing to do
					// This assumes internal types are always singleton
					break
				}
				err := h(resp.Resources[0])
				var errorResp *google_rpc.Status
				if err != nil {
					errorResp = &google_rpc.Status{
						Code:    int32(codes.Internal),
						Message: err.Error(),
					}
				}
				// Send ACK/NACK
				con.sendRequest(&discovery.DiscoveryRequest{
					VersionInfo:   resp.VersionInfo,
					TypeUrl:       resp.TypeUrl,
					ResponseNonce: resp.Nonce,
					ErrorDetail:   errorResp,
				})
				continue
			}
			switch resp.TypeUrl {
			case v3.ExtensionConfigurationType:
				if features.WasmRemoteLoadConversion {
					// If Wasm remote load conversion feature is enabled, rewrite and send.
					go p.rewriteAndForward(con, resp, func(resp *discovery.DiscoveryResponse) {
						// Forward the response using the thread of `handleUpstreamResponse`
						// to prevent concurrent access to forwardToEnvoy
						select {
						case forwardEnvoyCh <- resp:
						case <-con.stopChan:
						}
					})
				} else {
					// Otherwise, forward ECDS resource update directly to Envoy.
					// 将数据发送给envoy
					forwardToEnvoy(con, resp)
				}
			default:
				if strings.HasPrefix(resp.TypeUrl, v3.DebugType) {
					p.forwardToTap(resp)
				} else {
                    // 将数据发送给envoy
					forwardToEnvoy(con, resp)
				}
			}
		case resp := <-forwardEnvoyCh:
		    // 将数据发送给envoy
			forwardToEnvoy(con, resp)
		case <-con.stopChan:
			return
		}
	}
}

func forwardToEnvoy(con *ProxyConnection, resp *discovery.DiscoveryResponse) {
	// ...
	if err := sendDownstream(con.downstream, resp); err != nil {
		select {
		case con.downstreamError <- err:
		default:
		}
		return
	}
}

```

#### envoy->istiod 通讯 handleUpstreamRequest

```golang
func (p *XdsProxy) handleUpstreamRequest(con *ProxyConnection) {
	initialRequestsSent := atomic.NewBool(false)
	go func() {
		for {
			// recv xds requests from envoy
			// 接受envoy传来的数据
			req, err := con.downstream.Recv()
			if err != nil {
				select {
				case con.downstreamError <- err:
				case <-con.stopChan:
				}
				return
			}

			// forward to istiod
			// 发送给istiod
			con.sendRequest(req)
			// ...
		}
	}()

	defer con.upstream.CloseSend() // nolint
	for {
		select {
		case req := <-con.requestsChan.Get():
			con.requestsChan.Load()
			// 获取发送 req 
			// ...
			// 发送给 istiod 
            if err := sendUpstream(con.upstream, req); err != nil {
                err = fmt.Errorf("upstream [%d] send error for type url %s: %v", con.conID, req.TypeUrl, err)
                con.upstreamError <- err
                return
            }
		case <-con.stopChan:
			return
		}
	}
}

```

### 3. 初始化 envoyAgent

```golang
// # pkg/envoy/agent.go
func (a *Agent) initializeEnvoyAgent(ctx context.Context) error {
	node, err := a.generateNodeMetadata()
	if err != nil {
		return fmt.Errorf("failed to generate bootstrap metadata: %v", err)
	}

	log.Infof("Pilot SAN: %v", node.Metadata.PilotSubjectAltName)

	// Note: the cert checking still works, the generated file is updated if certs are changed.
	// We just don't save the generated file, but use a custom one instead. Pilot will keep
	// monitoring the certs and restart if the content of the certs changes.
	if len(a.proxyConfig.CustomConfigFile) > 0 {
		// there is a custom configuration. Don't write our own config - but keep watching the certs.
		a.envoyOpts.ConfigPath = a.proxyConfig.CustomConfigFile
		a.envoyOpts.ConfigCleanup = false
	} else {
		// 构建 bootstrap 配置文件
		out, err := bootstrap.New(bootstrap.Config{
			Node: node,
		}).CreateFile()
		if err != nil {
			return fmt.Errorf("failed to generate bootstrap config: %v", err)
		}
		a.envoyOpts.ConfigPath = out
		a.envoyOpts.ConfigCleanup = true
	}

	// Back-fill envoy options from proxy config options
	a.envoyOpts.BinaryPath = a.proxyConfig.BinaryPath
	a.envoyOpts.AdminPort = a.proxyConfig.ProxyAdminPort
	a.envoyOpts.DrainDuration = a.proxyConfig.DrainDuration
	a.envoyOpts.Concurrency = a.proxyConfig.Concurrency.GetValue()

	// Checking only uid should be sufficient - but tests also run as root and
	// will break due to permission errors if we start envoy as 1337.
	// This is a mode used for permission-less docker, where iptables can't be
	// used.
	a.envoyOpts.AgentIsRoot = os.Getuid() == 0 && strings.HasSuffix(a.cfg.DNSAddr, ":53")

	a.envoyOpts.DualStack = a.cfg.DualStack

	// 初始化 envoyProxy
	envoyProxy := envoy.NewProxy(a.envoyOpts)

	drainDuration := a.proxyConfig.TerminationDrainDuration.AsDuration()
	localHostAddr := localHostIPv4
	if a.cfg.IsIPv6 {
		localHostAddr = localHostIPv6
	}
	// 初始化 envoyAgent
	a.envoyAgent = envoy.NewAgent(envoyProxy, drainDuration, a.cfg.MinimumDrainDuration, localHostAddr,
		int(a.proxyConfig.ProxyAdminPort), a.cfg.EnvoyStatusPort, a.cfg.EnvoyPrometheusPort, a.cfg.ExitOnZeroActiveConnections)
	if a.cfg.EnableDynamicBootstrap {
		// ...
	}
	return nil
}

// # pkg/envoy/agent.go
// Run starts the envoy and waits until it terminates.
func (a *Agent) Run(ctx context.Context) {
	log.Info("Starting proxy agent")
	go a.runWait(a.abortCh)

	select {
	case status := <-a.statusCh:
		// ...

	case <-ctx.Done():
		a.terminate()
		// ...
	}
}

// runWait runs the start-up command as a go routine and waits for it to finish
func (a *Agent) runWait(abortCh <-chan error) {
	log.Infof("starting")
	err := a.proxy.Run(abortCh)
	a.proxy.Cleanup()
	a.statusCh <- exitStatus{err: err}
}
```

最终调用 proxy.Run 启动 envoy

```golang

// # /pkg/envoy/proxy.go
func (e *envoy) Run(abort <-chan error) error {
	// spin up a new Envoy process
	args := e.args(e.ConfigPath, istioBootstrapOverrideVar.Get())
	log.Infof("Envoy command: %v", args)

	/* #nosec */
	cmd := exec.Command(e.BinaryPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if e.AgentIsRoot {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		cmd.SysProcAttr.Credential = &syscall.Credential{
			Uid: 1337,
			Gid: 1337,
		}
	}

	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-abort:
		log.Warnf("Aborting proxy")
		if errKill := cmd.Process.Kill(); errKill != nil {
			log.Warnf("killing proxy caused an error %v", errKill)
		}
		return err
	case err := <-done:
		return err
	}
}

```


# Reference
- https://blog.csdn.net/a1023934860/article/details/126053998
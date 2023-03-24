# envoy的集群管理

## 集群管理内容

- 集群管理器与服务发现机制

- 主动健康状态检测与异常点探测

- 负载均衡策略
  - 分布式负载均衡
    - 负载均衡算法：加权轮询、加权最少连接、环哈希、磁悬浮和随机等
    - 区域感知路由
  - 全局负载均衡
    - 位置优先级
    - 位置权重
    - 均衡器子集

- 熔断和连接池

## Cluster Manager

- Envoy支持同时配置任意数量的上游集群，并基于Cluster Manager 管理它们

> - Cluster Manager负责为集群管理上游主机的健康状态、负载均衡机制、连接类型及适用协 议等；
> - 生成集群配置的方式由静态或动态（CDS）两种；
> 

- 集群预热

> - 集群在服务器启动或者通过 CDS 进行初始化时需要一个预热的过程，这意味着集群存在下列状况
初始服务发现加载 (例如DNS 解析、EDS 更新等)完成之前不可用
配置了主动健康状态检查机制时，Envoy会主动发送健康状态检测请求报文至发现的每个上游主机； 于是，初始的主动健康检查成功完成之前不可用
> - 新增集群初始化完成之前对Envoy的其它组件来说不可见；而对于需要更新的集群，在其预热完成后通过与旧集群的原子交换来确保不会发生流量中断类的错误；
> 
> 

## [集群配置](https://cloudnative.to/envoy/api-v3/config/cluster/v3/cluster.proto.html)

配置单个集群: 

```json
{
  "transport_socket_matches": [],
  "name": "...",
  "alt_stat_name": "...",
  "type": "...",
  "cluster_type": "{...}",
  "eds_cluster_config": "{...}",
  "connect_timeout": "{...}",
  "per_connection_buffer_limit_bytes": "{...}",
  "lb_policy": "...",
  "load_assignment": "{...}",
  "health_checks": [],
  "max_requests_per_connection": "{...}",
  "circuit_breakers": "{...}",
  "upstream_http_protocol_options": "{...}",
  "common_http_protocol_options": "{...}",
  "http_protocol_options": "{...}",
  "http2_protocol_options": "{...}",
  "typed_extension_protocol_options": "{...}",
  "dns_refresh_rate": "{...}",
  "dns_failure_refresh_rate": "{...}",
  "respect_dns_ttl": "...",
  "dns_lookup_family": "...",
  "dns_resolvers": [],
  "use_tcp_for_dns_lookups": "...",
  "outlier_detection": "{...}",
  "cleanup_interval": "{...}",
  "upstream_bind_config": "{...}",
  "lb_subset_config": "{...}",
  "ring_hash_lb_config": "{...}",
  "maglev_lb_config": "{...}",
  "original_dst_lb_config": "{...}",
  "least_request_lb_config": "{...}",
  "common_lb_config": "{...}",
  "transport_socket": "{...}",
  "metadata": "{...}",
  "protocol_selection": "...",
  "upstream_connection_options": "{...}",
  "close_connections_on_host_health_failure": "...",
  "ignore_health_on_host_removal": "...",
  "filters": [],
  "track_timeout_budgets": "...",
  "upstream_config": "{...}",
  "track_cluster_stats": "{...}",
  "connection_pool_per_downstream_connection": "..."
}
```

- 集群管理器配置上游集群时需要知道如何解析集群成员，相应的解析机制即为服务发现

> - 集群中的每个成员由endpoint进行标识，它可由用户静态配置，也可通过EDS或DNS服务 动态发现；
> - Static ：静态配置，即显式指定每个上游主机的已解析名称（IP地址/端口或unix 域套按字文件）； 
> - Strict DNS：严格DNS，Envoy将持续和异步地解析指定的DNS目标，并将DNS结果中的返回的每个IP地址视为上游集群中可用成员；
> - Logical DNS：逻辑DNS，集群仅使用在需要启动新连接时返回的第一个IP地址，而非严格获取 DNS 查询的结果并假设它们构成整个上游集群；适用于必须通过DNS访问的大规模Web服务集群；
> - Original destination：当传入连接通过iptables的REDIRECT或TPROXY target或使用代理协议重定向 到Envoy时，可以使用原始目标集群；
> - Endpoint discovery service (EDS) ：EDS是一种基于GRPC或REST-JSON API的xDS 管理服务器获取集 群成员的服务发现方式；
> - Custom cluster ：Envoy还支持在集群配置上的cluster_type字段中指定使用自定义集群发现机制；

- Envoy的服务发现并未采用完全一致的机制，而是假设主机以最终一致的方式加入或 离开网格，它结合主动健康状态检查机制来判定集群的健康状态；

> - 健康与否的决策机制以完全分布式的方式进行，因此可以很好地应对网络分区
> - 为集群启用主机健康状态检查机制后，Envoy基于如下方式判定是否路由请求到一个主机
> 

![img.png](images/envoy_cluster_check.png)

## 集群的故障处理机制

### 故障处理机制

- Envoy提供了一系列开箱即用的故障处理机制

> - 超时(timeout)
> - 有限次数的重试，并支持可变的重试延迟
> - 主动健康检查与异常探测
> - 连接池
> - 断路器
>

- 所有这些特性，都可以在运行时动态配置

- 结合流量管理机制，用户可为每个服务/版本定制所需的故障恢复机制


###  Upstreams 健康状态检测

- 健康状态检测用于确保代理服务器不会将下游客户端的请求代理至工作异常的上游主机

- Envoy支持两种类型的健康状态检测，二者均基于集群进行定义

  - 主动检测（Active Health Checking）：Envoy周期性地发送探测报文至上游主机，并根据其响应 判断其 健康状态；Envoy目前支持三种类型的主动检测：
    - HTTP：向上游主机发送HTTP请求报文
    - L3/L4：向上游主机发送L3/L4请求报文，基于响应的结果判定其健康状态，或仅通过连接状态进行判定； 
    - Redis：向上游的redis服务器发送Redis PING ；

  - 被动检测（Passive Health Checking）：Envoy通过异常检测（Outlier Detection）机制进行被动模式的健康状态检测
  目前，仅http router、tcp proxy和redis proxy三个过滤器支持异常值检测；
  Envoy支持以下类型的异常检测
    - 连续5XX：意指所有类型的错误，非http router过滤器生成的错误也会在内部映射为5xx错误代码；
    - 连续网关故障：连续5XX的子集，单纯用于http的502、503或504错误，即网关故障；
    - 连续的本地原因故障：Envoy无法连接到上游主机或与上游主机的通信被反复中断；
    - 成功率：主机的聚合成功率数据阈值；

### Upstreams主动健康状态检测

集群的主机健康状态检测机制需要显式定义，否则，发现的所有上游主机即被视为可用；定义语法

```yaml

clusters:
- name: ...
  ...
  load_assignment:
    endpoints:
    - lb_endpoints:
      - endpoint:
        health_check_config:
          port_value: ... # 自定义健康状态检测时使用的端口；
   
    health_checks:
    - timeout: ... # 超时时长
      interval: ... # 时间间隔
      initial_jitter: ... # 初始检测时间点散开量，以毫秒为单位；
      interval_jitter: ... # 间隔检测时间点散开量，以毫秒为单位；
      unhealthy_threshold: ... # 将主机标记为不健康状态的检测阈值，即至少多少次不健康的检测后才将其标记为不可用；
      healthy_threshold: ... # 将主机标记为健康状态的检测阈值，但初始检测成功一次即视主机为健康；
      http_health_check: {...} # HTTP类型的检测；包括此种类型在内的以下四种检测类型必须设置一种；
      tcp_health_check: {...} # TCP类型的检测；
      grpc_health_check: {...} # GRPC专用的检测；
      custom_health_check: {...} # 自定义检测；
      reuse_connection: ... # 布尔型值，是否在多次检测之间重用连接，默认值为true；
      unhealthy_interval: ... # 标记为“unhealthy” 状态的端点的健康检测时间间隔，一旦重新标记为“healthy” 即转为正常时间间隔；
      unhealthy_edge_interval: ... # 端点刚被标记为“unhealthy” 状态时的健康检测时间间隔，随后即转为同unhealthy_interval的定义；
      healthy_edge_interval: ... # 端点刚被标记为“healthy” 状态时的健康检测时间间隔，随后即转为同interval的定义；
      tls_options: { … } # tls相关的配置
      transport_socket_match_criteria: {…} # Optional key/value pairs that will be used to match a transport socket from those specified in the cluster’s transport socket matches.

```

### 主动健康状态检查：TCP

TCP类型的检测:

```yaml

clusters:
- name: local_service
  connect_timeout: 0.25s
  lb_policy: ROUND_ROBIN
  type: EDS
  eds_cluster_config:
    eds_config:
      api_config_source:
        api_type: GRPC
        grpc_services:
        - envoy_grpc:
            cluster_name: xds_cluster
    health_checks:
    - timeout: 5s
      interval: 10s
      unhealthy_threshold: 2
      healthy_threshold: 2
      tcp_health_check: {}

```
空负载的tcp检测意味着仅通过连接状态判定其检测结果

非空负载的tcp检测可以使用send和receive来分别指定请求负荷及 于响应报文中期望模糊匹配 的结果

```json
{
"send": "{...}",
"receive": []
}
```

### HTTP类型的检测

http类型的检测可以自定义使用的path、 host和期望的响应码等，并能够在必要时修 改（添加/删除）请求报文的标头 。

具体配置语法如下：

```yaml

health_checks: []
- ...
  http_health_check:
    "host": "..." # 检测时使用的主机标头，默认为空，此时使用集群名称；
    "path": "..." # 检测时使用的路径，例如/healthz；必选参数；
    “service_name_matcher”: “...” # 用于验证检测目标集群服务名称的参数，可选； 
    "request_headers_to_add": [] # 向检测报文添加的自定义标头列表； 
    "request_headers_to_remove": [] # 从检测报文中移除的标头列表； 
    "expected_statuses": [] # 期望的响应码列表；

```

配置示例:

```yaml

clusters:
- name: local_service
  connect_timeout: 0.25s
  lb_policy: ROUND_ROBIN
  type: EDS
  eds_cluster_config:
    eds_config:
    api_config_source:
    api_type: GRPC
    grpc_services:
    - envoy_grpc:
      cluster_name: xds_cluster
      health_checks:
      - timeout: 5s
        interval: 10s
        unhealthy_threshold: 2
        healthy_threshold: 2
        http_health_check:
          host: ... # 默认为空值，并自动使用集群为其值； 
          path: ... # 检测针对的路径，例如/healthz； 
          expected_statuses: ... # 期望的响应码，默认为200；

```
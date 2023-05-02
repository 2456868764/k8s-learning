# istio

# get started
1. [文档](https://istio.io/latest/zh/docs/)
2. [install](https://istio.io/latest/zh/docs/setup/install/)

## 使用 Istioctl 安装
1. get istioctl

```shell
https://github.com/istio/istio/releases/tag/1.17.2
```

2. install

```shell
istioctl install --set profile=demo
```

3. 查看安装Pods

```shell
$  kubectl get pods -n istio-system
   NAME                                   READY   STATUS    RESTARTS   AGE
   istio-egressgateway-575466f5bb-xbv8d   1/1     Running   0          48s
   istio-ingressgateway-cb9c6b49d-x6zgx   1/1     Running   0          48s
   istiod-6fbbf67d58-5tlwc                1/1     Running   0          2m11s
```
4. 查看安装CRD

```shell
$ kubectl get crd | grep istio
authorizationpolicies.security.istio.io               2023-04-29T10:02:00Z
destinationrules.networking.istio.io                  2023-04-29T10:02:00Z
envoyfilters.networking.istio.io                      2023-04-29T10:02:00Z
gateways.networking.istio.io                          2023-04-29T10:02:00Z
istiooperators.install.istio.io                       2023-04-29T10:02:00Z
peerauthentications.security.istio.io                 2023-04-29T10:02:00Z
proxyconfigs.networking.istio.io                      2023-04-29T10:02:00Z
requestauthentications.security.istio.io              2023-04-29T10:02:00Z
serviceentries.networking.istio.io                    2023-04-29T10:02:01Z
sidecars.networking.istio.io                          2023-04-29T10:02:01Z
telemetries.telemetry.istio.io                        2023-04-29T10:02:01Z
virtualservices.networking.istio.io                   2023-04-29T10:02:01Z
wasmplugins.extensions.istio.io                       2023-04-29T10:02:01Z
workloadentries.networking.istio.io                   2023-04-29T10:02:01Z
workloadgroups.networking.istio.io                    2023-04-29T10:02:01Z
```

5. 卸载 Istio
要从集群中完整卸载 Istio，运行下面命令：

```shell
$ istioctl uninstall --purge
```

## Install Addons

```shell
$ kubectl apply -f samples/addons/grafna.yaml
$ kubectl apply -f samples/addons/jaeger.yaml
$ kubectl apply -f samples/addons/kaili.yaml
$ kubectl apply -f samples/addons/prometheus.yaml
```


## 需要开启 Envoy 访问日志，执行以下命令修改 istio 配置

  ```shell
  kubectl -n istio-system edit configmap istio
  ```
  
  ```yaml
  data:
  mesh: |-
    accessLogEncoding: JSON
    accessLogFile: /dev/stdout
  ```

- accessLogEncoding表示 accesslog 输出格式，Istio 预定义了 TEXT 和 JSON 两种日志输出格式。 默认使用 TEXT，通常改成 JSON 以提升可读性；
- accessLogFile:表示 accesslog 输出位置，通常指定到 /dev/stdout (标准输出)，以便使用 kubectl logs 来查看日志。


## 安装 httpbin 测试

- Start the httpbin service inside the Istio service mesh:
    ```bash
    kubectl create namespace istio-demo
    kubectl label namespace istio-demo istio-injection=enabled --overwrite
    kubectl apply -f samples/httpbin/httpbin.yaml -n istio-demo
    ```

- Otherwise manually inject the sidecars before applying:

    ```bash
    kubectl apply -f <(istioctl kube-inject -f samples/httpbin/httpbin.yaml) -n istio-demo
    ```

- apply gateway to access httpbin

  ```shell
  kubectl apply -f samples/httpbin/httpbin-gateway.yaml -n istio-demo
  ```
  
- apply sleep pod
  ```shell
  kubectl apply -f samples/sleep/sleep.yaml -n istio-demo
  ```
- test 
  ```shell
   export SLEEP_POD=$(kubectl get pods -l app=sleep -o 'jsonpath={.items[0].metadata.name}' -n istio-demo)
   kubectl exec "$SLEEP_POD" -n istio-demo -c sleep -- curl -sS http://httpbin:8000/headers
  
  {"Accept":"*/*","User-Agent":"curl/8.0.1-DEV","X-B3-Parentspanid":"b30e5a52126e8a3a","X-B3-Sampled":"1","X-B3-Spanid":"32e1a5ae744f591f","X-B3-Traceid":"2d88aa543462b4e1b30e5a52126e8a3a","X-Envoy-Attempt-Count":"1","X-Forwarded-Client-Cert":"By=spiffe://cluster.local/ns/istio-demo/sa/httpbin;Hash=3cb80bdef2f0cd5c2ecb03244a378fb1faecfc192667bd0cb67886d9f7341b80;Subject=\"\";URI=spiffe://cluster.local/ns/istio-demo/sa/sleep","X-Forwarded-Proto":"http","X-Request-Id":"65508395-36bf-9ac6-98f2-72ad5400f54a"}%   

  ```
- check sleep pod envoy proxy logs
  ```shell
  kubectl logs -l app=sleep -n istio-demo -c istio-proxy
  
  {"route_name":"default","response_flags":"-","response_code":200,"authority":"httpbin:8000","user_agent":"curl/8.0.1-DEV","method":"GET","bytes_sent":508,"request_id":"6618b006-b862-927e-ac43-aaf4f2026c0d","bytes_received":0,"downstream_remote_address":"10.10.241.84:40658","x_forwarded_for":null,"upstream_cluster":"outbound|8000||httpbin.istio-demo.svc.cluster.local","upstream_transport_failure_reason":null,"path":"/headers","start_time":"2023-05-02T01:45:04.154Z","protocol":"HTTP/1.1","connection_termination_details":null,"upstream_service_time":"11","duration":12,"upstream_local_address":"10.10.241.84:58514","upstream_host":"10.10.241.82:80","response_code_details":"via_upstream","downstream_local_address":"10.97.94.170:8000","requested_server_name":null}

  ```

- check httpbin pod envoy proxy logs
  ```shell
  kubectl logs -l app=httpbin -n istio-demo -c istio-proxy
  
  {"response_flags":"-","x_forwarded_for":null,"start_time":"2023-05-02T01:45:04.158Z","connection_termination_details":null,"path":"/headers","upstream_cluster":"inbound|80||","user_agent":"curl/8.0.1-DEV","upstream_transport_failure_reason":null,"response_code_details":"via_upstream","requested_server_name":"outbound_.8000_._.httpbin.istio-demo.svc.cluster.local","response_code":200,"upstream_host":"10.10.241.82:80","duration":2,"route_name":"default","upstream_local_address":"127.0.0.6:56619","bytes_received":0,"protocol":"HTTP/1.1","authority":"httpbin:8000","downstream_remote_address":"10.10.241.84:58514","method":"GET","bytes_sent":508,"request_id":"6618b006-b862-927e-ac43-aaf4f2026c0d","downstream_local_address":"10.10.241.82:80","upstream_service_time":"1"}

  ```
  
# 架构

## 概览

Istio 服务网格从逻辑上分为数据平面和控制平面。

- 数据平面 由一组智能代理（Envoy）组成，被部署为 Sidecar。这些代理负责协调和控制微服务之间的所有网络通信。它们还收集和报告所有网格流量的遥测数据。

- 控制平面 管理并配置代理来进行流量路由。

下图展示了组成每个平面的不同组件：

![img.png](images/istio3.png)

### 组件

#### Envoy

Istio 使用 Envoy 代理的扩展版本。Envoy 是用 C++ 开发的高性能代理，用于协调服务网格中所有服务的入站和出站流量。Envoy 代理是唯一与数据平面流量交互的 Istio 组件。

Envoy 代理被部署为服务的 Sidecar，在逻辑上为服务增加了 Envoy 的许多内置特性，例如：

- 动态服务发现
- 负载均衡
- TLS 终端
- HTTP/2 与 gRPC 代理
- 熔断器
- 健康检查
- 基于百分比流量分割的分阶段发布
- 故障注入
- 丰富的指标

由 Envoy 代理启用的一些 Istio 的功能和任务包括：

- 流量控制功能：通过丰富的 HTTP、gRPC、WebSocket 和 TCP 流量路由规则来执行细粒度的流量控制。
- 网络弹性特性：重试设置、故障转移、熔断器和故障注入。
- 安全性和身份认证特性：执行安全性策略，并强制实行通过配置 API 定义的访问控制和速率限制。
- 基于 WebAssembly 的可插拔扩展模型，允许通过自定义策略执行和生成网格流量的遥测。


### Istiod

- istiod
  - 服务发现
  - 配置
  - 证书管理

- Istiod 将控制流量行为的高级路由规则转换为 Envoy 特定的配置，并在运行时将其传播给 Sidecar。Pilot 提取了特定平台的服务发现机制，并将其综合为一种标准格式，任何符合 Envoy API 的 Sidecar 都可以使用。

- Istiod 安全通过内置的身份和凭证管理，实现了强大的服务对服务和终端用户认证。

- Istiod 充当证书授权（CA），并生成证书以允许在数据平面中进行安全的 mTLS 通信。


## 项目代码

### 顶层目录

```shell

(base) ➜  istio git:(source-1.17.2) tree . -L 1
├── bin
├── cni
├── common
├── docker
├── go.mod
├── go.sum
├── istio.deps
├── istioctl
├── licenses
├── logo
├── manifests
├── operator
├── pilot
├── pkg
├── prow
├── release
├── releasenotes
├── samples
├── security
├── tests
└── tools

18 directories, 13 files

```

- cni, istioctl, operator, pilot 目录分别包含同名相应模块的代码。下面的 cmd 是模块下相应二进制的编译入口，cmd 下面的 pkg 是 cmd 中的代码需要调用的依赖逻辑。
- 多个模块共同依赖的一些逻辑会放到外层的 pkg 目录下。
- security 是 istio 安全模块，构建零信任网络

### pilot 

Pilot 是最核心的模块，有 pilot-agent 和 pilot-discovery 两个二进制:

```shell
pilot
├── cmd
│   ├── pilot-agent
│   └── pilot-discovery
```

- pilot-discovery 就是 "istiod"，即 istio 控制面。
- pilot-agent 是连接 istiod (控制面) 和 envoy (数据面) 之间的纽带，主要负责拉起和管理数据面进程。


pilot-discovery(istiod) <--> pliot-agent <---> Envoy 


## 组件

```shell
(base) ➜  k8s-learning git:(main) ✗ kubectl get pods -n istio-system

NAME                                   READY   STATUS    RESTARTS   AGE
istio-egressgateway-575466f5bb-xbv8d   1/1     Running   0          46h
istio-ingressgateway-cb9c6b49d-x6zgx   1/1     Running   0          46h
istiod-6fbbf67d58-5tlwc                1/1     Running   0          46h

```

有三个核心Pod:
- istiod
- ingressgateway
- egressgateway


### ingressgateway

```shell
(base) ➜  k8s-learning git:(main) ✗ kubectl describe pod istio-ingressgateway-cb9c6b49d-x6zgx -n istio-system

Name:         istio-ingressgateway-cb9c6b49d-x6zgx
Namespace:    istio-system
Priority:     0
Node:         master01/192.168.64.16
Start Time:   Sat, 29 Apr 2023 18:03:24 +0800
Labels:       app=istio-ingressgateway
              chart=gateways
              heritage=Tiller
              install.operator.istio.io/owning-resource=unknown
              istio=ingressgateway
              istio.io/rev=default
              operator.istio.io/component=IngressGateways
              pod-template-hash=cb9c6b49d
              release=istio
              service.istio.io/canonical-name=istio-ingressgateway
              service.istio.io/canonical-revision=latest
              sidecar.istio.io/inject=false
Annotations:  cni.projectcalico.org/containerID: 0168393a6c023d87f5cfb6d1cba3da3850d9269a5d41887e2ca9d6bbb439ce81
              cni.projectcalico.org/podIP: 10.10.241.73/32
              cni.projectcalico.org/podIPs: 10.10.241.73/32
              prometheus.io/path: /stats/prometheus
              prometheus.io/port: 15020
              prometheus.io/scrape: true
              sidecar.istio.io/inject: false
Status:       Running
IP:           10.10.241.73
IPs:
  IP:           10.10.241.73
Controlled By:  ReplicaSet/istio-ingressgateway-cb9c6b49d
Containers:
  istio-proxy:
    Container ID:  containerd://81a3db62892613845cb3e10ffab56acc77b197df07bafd704e9f79f755a3a1c8
    Image:         docker.io/istio/proxyv2:1.17.2
    Image ID:      docker.io/istio/proxyv2@sha256:f41745ee1183d3e70b10e82c727c772bee9ac3907fea328043332aaa90d7aa18
    Ports:         15021/TCP, 8080/TCP, 8443/TCP, 31400/TCP, 15443/TCP, 15090/TCP
    Host Ports:    0/TCP, 0/TCP, 0/TCP, 0/TCP, 0/TCP, 0/TCP
    Args:
      proxy
      router
      --domain
      $(POD_NAMESPACE).svc.cluster.local
      --proxyLogLevel=warning
      --proxyComponentLogLevel=misc:error
      --log_output_level=default:info
    State:          Running
      Started:      Sat, 29 Apr 2023 18:03:52 +0800
    Ready:          True
    Restart Count:  0
    Limits:
      cpu:     2
      memory:  1Gi
    Requests:
      cpu:      10m
      memory:   40Mi
    Readiness:  http-get http://:15021/healthz/ready delay=1s timeout=1s period=2s #success=1 #failure=30
    Environment:
      JWT_POLICY:                   third-party-jwt
      PILOT_CERT_PROVIDER:          istiod
      CA_ADDR:                      istiod.istio-system.svc:15012
      NODE_NAME:                     (v1:spec.nodeName)
      POD_NAME:                     istio-ingressgateway-cb9c6b49d-x6zgx (v1:metadata.name)
      POD_NAMESPACE:                istio-system (v1:metadata.namespace)
      INSTANCE_IP:                   (v1:status.podIP)
      HOST_IP:                       (v1:status.hostIP)
      SERVICE_ACCOUNT:               (v1:spec.serviceAccountName)
      ISTIO_META_WORKLOAD_NAME:     istio-ingressgateway
      ISTIO_META_OWNER:             kubernetes://apis/apps/v1/namespaces/istio-system/deployments/istio-ingressgateway
      ISTIO_META_MESH_ID:           cluster.local
      TRUST_DOMAIN:                 cluster.local
      ISTIO_META_UNPRIVILEGED_POD:  true
      ISTIO_META_CLUSTER_ID:        Kubernetes
      ISTIO_META_NODE_NAME:          (v1:spec.nodeName)
    Mounts:
      /etc/istio/config from config-volume (rw)
      /etc/istio/ingressgateway-ca-certs from ingressgateway-ca-certs (ro)
      /etc/istio/ingressgateway-certs from ingressgateway-certs (ro)
      /etc/istio/pod from podinfo (rw)
      /etc/istio/proxy from istio-envoy (rw)
      /var/lib/istio/data from istio-data (rw)
      /var/run/secrets/credential-uds from credential-socket (rw)
      /var/run/secrets/istio from istiod-ca-cert (rw)
      /var/run/secrets/kubernetes.io/serviceaccount from kube-api-access-2j2jm (ro)
      /var/run/secrets/tokens from istio-token (ro)
      /var/run/secrets/workload-spiffe-credentials from workload-certs (rw)
      /var/run/secrets/workload-spiffe-uds from workload-socket (rw)
Conditions:
  Type              Status
  Initialized       True 
  Ready             True 
  ContainersReady   True 
  PodScheduled      True 
Volumes:
  workload-socket:
    Type:       EmptyDir (a temporary directory that shares a pod's lifetime)
    Medium:     
    SizeLimit:  <unset>
  credential-socket:
    Type:       EmptyDir (a temporary directory that shares a pod's lifetime)
    Medium:     
    SizeLimit:  <unset>
  workload-certs:
    Type:       EmptyDir (a temporary directory that shares a pod's lifetime)
    Medium:     
    SizeLimit:  <unset>
  istiod-ca-cert:
    Type:      ConfigMap (a volume populated by a ConfigMap)
    Name:      istio-ca-root-cert
    Optional:  false
  podinfo:
    Type:  DownwardAPI (a volume populated by information about the pod)
    Items:
      metadata.labels -> labels
      metadata.annotations -> annotations
  istio-envoy:
    Type:       EmptyDir (a temporary directory that shares a pod's lifetime)
    Medium:     
    SizeLimit:  <unset>
  istio-data:
    Type:       EmptyDir (a temporary directory that shares a pod's lifetime)
    Medium:     
    SizeLimit:  <unset>
  istio-token:
    Type:                    Projected (a volume that contains injected data from multiple sources)
    TokenExpirationSeconds:  43200
  config-volume:
    Type:      ConfigMap (a volume populated by a ConfigMap)
    Name:      istio
    Optional:  true
  ingressgateway-certs:
    Type:        Secret (a volume populated by a Secret)
    SecretName:  istio-ingressgateway-certs
    Optional:    true
  ingressgateway-ca-certs:
    Type:        Secret (a volume populated by a Secret)
    SecretName:  istio-ingressgateway-ca-certs
    Optional:    true
  kube-api-access-2j2jm:
    Type:                    Projected (a volume that contains injected data from multiple sources)
    TokenExpirationSeconds:  3607
    ConfigMapName:           kube-root-ca.crt
    ConfigMapOptional:       <nil>
    DownwardAPI:             true
QoS Class:                   Burstable
Node-Selectors:              <none>
Tolerations:                 node.kubernetes.io/not-ready:NoExecute op=Exists for 300s
                             node.kubernetes.io/unreachable:NoExecute op=Exists for 300s
Events:                      <none>

```

可以得到以下信息:

1. Pod 由 istio-ingressgateway 启动

```shell
(base) ➜  k8s-learning git:(main) ✗ kubectl get deployment istio-ingressgateway -n istio-system

NAME                   READY   UP-TO-DATE   AVAILABLE   AGE
istio-ingressgateway   1/1     1            1           47h

```
2. 对应容器镜像

```shell
docker.io/istio/proxyv2:1.17.2
```
3. 启动参数

```shell
Args:
      proxy
      router
      --domain
      $(POD_NAMESPACE).svc.cluster.local
      --proxyLogLevel=warning
      --proxyComponentLogLevel=misc:error
      --log_output_level=default:info
```

4. 环境变量 ENV: 

```shell
Environment:
      JWT_POLICY:                   third-party-jwt
      PILOT_CERT_PROVIDER:          istiod
      CA_ADDR:                      istiod.istio-system.svc:15012
      NODE_NAME:                     (v1:spec.nodeName)
      POD_NAME:                     istio-ingressgateway-cb9c6b49d-x6zgx (v1:metadata.name)
      POD_NAMESPACE:                istio-system (v1:metadata.namespace)
      INSTANCE_IP:                   (v1:status.podIP)
      HOST_IP:                       (v1:status.hostIP)
      SERVICE_ACCOUNT:               (v1:spec.serviceAccountName)
      ISTIO_META_WORKLOAD_NAME:     istio-ingressgateway
      ISTIO_META_OWNER:             kubernetes://apis/apps/v1/namespaces/istio-system/deployments/istio-ingressgateway
      ISTIO_META_MESH_ID:           cluster.local
      TRUST_DOMAIN:                 cluster.local
      ISTIO_META_UNPRIVILEGED_POD:  true
      ISTIO_META_CLUSTER_ID:        Kubernetes
      ISTIO_META_NODE_NAME:          (v1:spec.nodeName)
```
5. Annotations:

```shell
   cni.projectcalico.org/containerID: 0168393a6c023d87f5cfb6d1cba3da3850d9269a5d41887e2ca9d6bbb439ce81
   cni.projectcalico.org/podIP: 10.10.241.73/32
   cni.projectcalico.org/podIPs: 10.10.241.73/32
   prometheus.io/path: /stats/prometheus
   prometheus.io/port: 15020
   prometheus.io/scrape: true
   sidecar.istio.io/inject: false
```

6. Mounts

```shell
Mounts:
      /etc/istio/config from config-volume (rw)
      /etc/istio/ingressgateway-ca-certs from ingressgateway-ca-certs (ro)
      /etc/istio/ingressgateway-certs from ingressgateway-certs (ro)
      /etc/istio/pod from podinfo (rw)
      /etc/istio/proxy from istio-envoy (rw)
      /var/lib/istio/data from istio-data (rw)
      /var/run/secrets/credential-uds from credential-socket (rw)
      /var/run/secrets/istio from istiod-ca-cert (rw)
      /var/run/secrets/kubernetes.io/serviceaccount from kube-api-access-2j2jm (ro)
      /var/run/secrets/tokens from istio-token (ro)
      /var/run/secrets/workload-spiffe-credentials from workload-certs (rw)
      /var/run/secrets/workload-spiffe-uds from workload-socket (rw)
```

这里包括 ca-certs, certs, token, serviceaccount, spiffe等后面在 istio 安全与零信任网络具体说明。


7. svc

```shell
(base) ➜  k8s-learning git:(main) ✗ kubectl get svc -n istio-system | grep ingressgateway
istio-ingressgateway   LoadBalancer   10.98.130.245    <pending>     15021:31829/TCP,80:32603/TCP,443:30221/TCP,31400:31870/TCP,15443:31806/TCP   47h
(
```

8. 测试 ingressgateway

查看 ingressgateway，本地测试这里用 nodePort 来暴露访问 ingressgateway, 获取 name == http2 暴露端口

```shell
export INGRESS_NAME=istio-ingressgateway
export INGRESS_NS=istio-system
export INGRESS_HOST=$(kubectl get po -l istio=ingressgateway -n "${INGRESS_NS}" -o jsonpath='{.items[0].status.hostIP}')
export INGRESS_PORT=$( kubectl get svc istio-ingressgateway -n istio-system -o jsonpath='{.spec.ports[?(@.name=="http2")].nodePort}' )
export SECURE_INGRESS_PORT=$(kubectl -n "${INGRESS_NS}" get service "${INGRESS_NAME}" -o jsonpath='{.spec.ports[?(@.name=="https")].nodePort}')
export TCP_INGRESS_PORT=$(kubectl -n "${INGRESS_NS}" get service "${INGRESS_NAME}" -o jsonpath='{.spec.ports[?(@.name=="tcp")].nodePort}')
```

然后通过 nodeIP + Ingress_PORT 来访问 ingressgateway 入口

- 使用 curl 访问 httpbin 服务：

  ```shell
  $ curl -s -H "Host:httpbin.example.com" "http://$INGRESS_HOST:$INGRESS_PORT/hostname"
  
  "httpbin-798dbb9f74-l7ntg"%    
  ```

- 检查 ingressgateway proxy日志

  ```shell
  kubectl logs -l app=istio-ingressgateway -n istio-system -c istio-proxy
  
  {"method":"GET","response_flags":"-","connection_termination_details":null,"response_code_details":"via_upstream","request_id":"1e6c664a-0604-9ac1-b4d7-484be35cee92","protocol":"HTTP/1.1","upstream_local_address":"10.10.241.86:34222","downstream_local_address":"10.10.241.86:8080","upstream_service_time":"9","user_agent":"curl/7.64.1","upstream_cluster":"outbound|8000||httpbin.istio-demo.svc.cluster.local","route_name":null,"downstream_remote_address":"192.168.64.16:55163","requested_server_name":null,"bytes_received":0,"duration":15,"upstream_host":"10.10.241.82:80","path":"/hostname","bytes_sent":26,"x_forwarded_for":"192.168.64.16","upstream_transport_failure_reason":null,"response_code":200,"authority":"httpbin.example.com","start_time":"2023-05-02T01:35:38.296Z"}

  ```


## side car

下图展示的是 Istio 数据平面中 sidecar 的组成，以及与其交互的对象。

![img.png](images/istio2.png)


## 常用连接

- [Ports used by istio](https://istio.io/latest/docs/ops/deployment/requirements/#ports-used-by-istio)
- [Resource Annotations](https://istio.io/latest/docs/reference/config/annotations/)

# Reference
* https://jimmysong.io/blog/istio-components-and-ports/
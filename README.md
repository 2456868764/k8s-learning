# k8s-learning

## K8s

### 1.容器技术深度剖析
* 容器技术基础
* 镜像文件
* Dockerfile
* Namespace
* CGroup
* 网络
* containerd & Runc

### 2.k8s安装
* [Apple M1/M2 安装 k8s 1.27](./k8s/installation/utm-mac-m1-ubuntu-22.04.md)

### 3.控制面
* k8s架构和设计原则
* k8s API
* kubectl
* etcd
* Api-Server
* Scheduler-Server
* Controller-Server

### 4.k8s基本对象
* Deployment, ReplicatSet, Pod, StatefulSet, DamonSet
* Service
* Configmap, Secret,Service Account


### 5.k8s安全
* [RBAC](./doc/security.md)
* NetworkPolicy
* SecurtiyContext

### 6.Pod生命周期
* kubelet
* CRI & CNI & CSI 

### 7.k8s数据面
* CoreDNS
* Ingress & Service
* kube-proxy & Iptables
* Ipvs

### 8.k8s网络组件和流量
* Calico
* Cilium

## Operator开发

* [Cobra](./doc/cobra.md)
* GRPC
  * [protobuf基础](./doc/grpc_basic.md)
  * [通信模式](./doc/grpc_transport.md) 
  * 拦截器
  * metadata
  * 超时控制
  * 认证
  * 安全

* K8s Api
* Client-Go
* Controller-Runtime
* KubeBuilder


## 可观测性
* 日志
* 监控
  * [Prometheus PromQL](./observability/prometheus-promql.md)
* Trace

## GitOps
* harbor
* [helm](./helm/README.md)
* kustomize
* ArgoCD

## 应用迁移和生成化运维实践
* 运维最佳实践
* 排查


## 微服务项目开发和部署案例
* [Apisix + nacos + dubbo](./microservice/apisix.md)

## 服务网格
* envoy
  * [envoy 基础](./istio/envoy_basic.md)
  * [envoy xds](./istio/envoy_xds.md)
  * [envoy Cluster管理](./istio/envoy_cluser.md)
  * [envoy http流量管理](./istio/envoy_http.md)
  * [envoy 认证机制](./istio/envoy_tls.md)
  * [envoy Filter]
* istio


## 工具

* [shell](./tool/shell.md)
* [tproxy](./tool/tproxy.md)









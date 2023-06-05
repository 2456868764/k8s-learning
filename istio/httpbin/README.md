# httpbin 是什么？

httpbin 基于 Gin 开发，用于快速测试基于云原生微服务可观测性和流量管理等功能。

服务可观测性包括：
- 日志
- 指标
- 调用链路跟踪

流量管理包括：

- 网关
- Envoy & Istio
- 服务发现

# httpbin 1.0.0 版本

## 支持接口

````shell
GET    /metrics                  --> httpbin/pkg/middleware.(*metricMiddleWareBuilder).prometheusHandler.func1 (2 handlers)
GET    /                         --> httpbin/api.Anything (4 handlers)
POST   /                         --> httpbin/api.Anything (4 handlers)
GET    /hostname                 --> httpbin/api.HostName (4 handlers)
GET    /headers                  --> httpbin/api.Headers (4 handlers)
GET    /prob/liveness            --> httpbin/api.Healthz (4 handlers)
GET    /prob/livenessfile        --> httpbin/api.HealthzFile (4 handlers)
GET    /prob/readiness           --> httpbin/api.Readiness (4 handlers)
GET    /prob/readinessfile       --> httpbin/api.ReadinessFile (4 handlers)
GET    /prob/startup             --> httpbin/api.Startup (4 handlers)
GET    /prob/startupfile         --> httpbin/api.StartupFile (4 handlers)
GET    /data/bool                --> httpbin/api.Bool (4 handlers)
GET    /data/dto                 --> httpbin/api.ReponseAnyDto (4 handlers)
GET    /data/array               --> httpbin/api.ReponseAnyArray (4 handlers)
GET    /data/string              --> httpbin/api.ReponseAnyString (4 handlers)
GET    /service                  --> httpbin/api.Service (4 handlers)

````
## 支持功能

1. 支持 skywalking 调用链路跟踪
2. 支持 HTTP 接口 /metrics 指标输出
3. Readiness,Liveness,Startup 探针
4. 通过 /service?services=middle,backend 来模拟调用链路

## 待支持功能

1. 支持日志集成 Trace
2. 接入 opentelemetry & jager
3. grafana metric 看板
4. 基于 isito 流量管理样例
5. 其他 httpbin 原始接口迁移
6. 接入 nacos 服务发现


# 使用

## 镜像下载

```shell
docker pull 2456868764/httpbin:1.0.0
```

## 基本使用

1. 部署 deployment 

```shell
kubectl apply -f deploy/basic.yaml
```
2. 本地端口转发  

```shell
kubectl -n app-system port-forward service/basic  9090:80
```
3. 测试接口 /hostname 接口 输出 POD_NAME

```shell
curl -v http://127.0.0.1:9090/hostname 

"basic-6d6969cf9c-llzcp"
```

4. 测试 / 接口 输出整个请求头，请求参数，请求体，环境变量，调用链路等信息

```shell
curl -v http://127.0.0.1:9090/\?name\=httpbin
```
```json
{
  "args": {
    "name": "httpbin"
  },
  "form": {
    
  },
  "headers": {
    "accept": "*/*",
    "user-agent": "curl/7.64.1",
    "x-httpbin-trace-host": "basic-6d6969cf9c-llzcp",  # 调用链路中POD_NAME
    "x-httpbin-trace-service": "basic"                 # 调用链路中SERVICE_NAME
  },
  "method": "GET",
  "origin": "",
  "url": "/",
  "envs": {
    "BACKEND_PORT": "tcp://10.96.92.115:80",
    "BACKEND_PORT_80_TCP": "tcp://10.96.92.115:80",
    "BACKEND_PORT_80_TCP_ADDR": "10.96.92.115",
    "BACKEND_PORT_80_TCP_PORT": "80",
    "BACKEND_PORT_80_TCP_PROTO": "tcp",
    "BACKEND_SERVICE_HOST": "10.96.92.115",
    "BACKEND_SERVICE_PORT": "80",
    "BACKEND_SERVICE_PORT_HTTP": "80",
    "BASIC_PORT": "tcp://10.96.150.13:80",
    "BASIC_PORT_80_TCP": "tcp://10.96.150.13:80",
    "BASIC_PORT_80_TCP_ADDR": "10.96.150.13",
    "BASIC_PORT_80_TCP_PORT": "80",
    "BASIC_PORT_80_TCP_PROTO": "tcp",
    "BASIC_SERVICE_HOST": "10.96.150.13",
    "BASIC_SERVICE_PORT": "80",
    "BASIC_SERVICE_PORT_HTTP": "80",
    "BFF_PORT": "tcp://10.96.144.248:80",
    "BFF_PORT_80_TCP": "tcp://10.96.144.248:80",
    "BFF_PORT_80_TCP_ADDR": "10.96.144.248",
    "BFF_PORT_80_TCP_PORT": "80",
    "BFF_PORT_80_TCP_PROTO": "tcp",
    "BFF_SERVICE_HOST": "10.96.144.248",
    "BFF_SERVICE_PORT": "80",
    "BFF_SERVICE_PORT_HTTP": "80",
    "HOME": "/root",
    "HOSTNAME": "basic-6d6969cf9c-llzcp",
    "KUBERNETES_PORT": "tcp://10.96.0.1:443",
    "KUBERNETES_PORT_443_TCP": "tcp://10.96.0.1:443",
    "KUBERNETES_PORT_443_TCP_ADDR": "10.96.0.1",
    "KUBERNETES_PORT_443_TCP_PORT": "443",
    "KUBERNETES_PORT_443_TCP_PROTO": "tcp",
    "KUBERNETES_SERVICE_HOST": "10.96.0.1",
    "KUBERNETES_SERVICE_PORT": "443",
    "KUBERNETES_SERVICE_PORT_HTTPS": "443",
    "NODE_NAME": "higress-worker",
    "PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
    "POD_IP": "10.244.1.7",
    "POD_NAME": "basic-6d6969cf9c-llzcp",
    "POD_NAMESPACE": "app-system",
    "SERVICE_ACCOUNT": "basic",
    "SERVICE_NAME": "basic",
    "VERSION": "v1"
  },
  "host_name": "basic-6d6969cf9c-llzcp",
  "body": ""
}
```
5. 测试 /metrics 接口

```shell
curl -v http://127.0.0.1:9090/metrics 
# HELP app_request_duration_seconds The HTTP request latencies in seconds.
# TYPE app_request_duration_seconds histogram
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/",le="0.005"} 0
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/",le="0.01"} 1
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/",le="0.025"} 3
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/",le="0.05"} 4
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/",le="0.1"} 4
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/",le="0.25"} 4
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/",le="0.5"} 4
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/",le="1"} 4
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/",le="2.5"} 4
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/",le="5"} 4
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/",le="10"} 4
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/",le="+Inf"} 4
app_request_duration_seconds_sum{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/"} 0.07803300099999999
app_request_duration_seconds_count{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/"} 4
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/hostname",le="0.005"} 4
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/hostname",le="0.01"} 4
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/hostname",le="0.025"} 4
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/hostname",le="0.05"} 4
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/hostname",le="0.1"} 4
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/hostname",le="0.25"} 4
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/hostname",le="0.5"} 4
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/hostname",le="1"} 4
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/hostname",le="2.5"} 4
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/hostname",le="5"} 4
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/hostname",le="10"} 4
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/hostname",le="+Inf"} 4
app_request_duration_seconds_sum{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/hostname"} 0.0007844589999999999
app_request_duration_seconds_count{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/hostname"} 4
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/liveness",le="0.005"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/liveness",le="0.01"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/liveness",le="0.025"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/liveness",le="0.05"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/liveness",le="0.1"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/liveness",le="0.25"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/liveness",le="0.5"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/liveness",le="1"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/liveness",le="2.5"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/liveness",le="5"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/liveness",le="10"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/liveness",le="+Inf"} 346
app_request_duration_seconds_sum{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/liveness"} 0.024784950999999993
app_request_duration_seconds_count{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/liveness"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/readiness",le="0.005"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/readiness",le="0.01"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/readiness",le="0.025"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/readiness",le="0.05"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/readiness",le="0.1"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/readiness",le="0.25"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/readiness",le="0.5"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/readiness",le="1"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/readiness",le="2.5"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/readiness",le="5"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/readiness",le="10"} 346
app_request_duration_seconds_bucket{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/readiness",le="+Inf"} 346
app_request_duration_seconds_sum{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/readiness"} 0.011009689000000008
app_request_duration_seconds_count{code="200",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/readiness"} 346
# HELP app_request_size_bytes The HTTP request sizes in bytes.
# TYPE app_request_size_bytes summary
app_request_size_bytes_sum{instance="basic-6d6969cf9c-llzcp",service="basic"} 61030
app_request_size_bytes_count{instance="basic-6d6969cf9c-llzcp",service="basic"} 700
# HELP app_requests_total How many HTTP requests processed, partitioned by status code and HTTP method.
# TYPE app_requests_total counter
app_requests_total{code="200",handler="httpbin/api.Anything",host="127.0.0.1:8080",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/"} 4
app_requests_total{code="200",handler="httpbin/api.Healthz",host="10.244.1.7:80",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/liveness"} 346
app_requests_total{code="200",handler="httpbin/api.HostName",host="127.0.0.1:8080",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/hostname"} 4
app_requests_total{code="200",handler="httpbin/api.Readiness",host="10.244.1.7:80",instance="basic-6d6969cf9c-llzcp",method="GET",service="basic",url="/prob/readiness"} 346
# HELP app_response_size_bytes The HTTP response sizes in bytes.
# TYPE app_response_size_bytes summary
app_response_size_bytes_sum{instance="basic-6d6969cf9c-llzcp",service="basic"} 13848
app_response_size_bytes_count{instance="basic-6d6969cf9c-llzcp",service="basic"} 700
# HELP go_gc_duration_seconds A summary of the pause duration of garbage collection cycles.

```

## 调用链路使用

1. 部署

```shell
kubectl apply -f deploy/skywalking.yaml
kubectl apply -f deploy/app.yaml
```
2. 检查POD情况

```shell
kubectl get pods -n app-system
NAME                       READY   STATUS    RESTARTS   AGE
backend-6b9549bc64-ggpws   1/1     Running   1          20h
bff-756955bb86-465zl       1/1     Running   1          20h

kubectl get pods -n op-system 
NAME                                        READY   STATUS    RESTARTS   AGE
skywalking-oap-dashboard-65f496ccc9-zfrsd   1/1     Running   1          23h
skywalking-oap-server-859694656b-7s569      1/1     Running   1          23h

```
3. skywalking dashboard 和 bff 服务端口转发

```shell
kubectl -n op-system port-forward service/skywalking-oap-dashboard 8080:8080
kubectl -n app-system port-forward service/bff 9090:80
```

4. 模拟调用链路 通过 bff 服务调用 backend 服务

```shell
curl -v -H  http://127.0.0.1:9090/service\?services\=backend

{
  "args": {
    
  },
  "form": {
    
  },
  "headers": {
    "accept-encoding": "gzip",
    "sw8": "1-ZjJjYWU1MzcwMzZkMTFlZWE0YmRlMmZkNDMzZTlkNTc=-ZjJjYWU1NWMwMzZkMTFlZWE0YmRlMmZkNDMzZTlkNTc=-1-YmZm-YmZmLTc1Njk1NWJiODYtNDY1emw=-L0dFVC9zZXJ2aWNl-aHR0cDovL2JhY2tlbmQv",
    "sw8-correlation": "",
    "user-agent": "Go-http-client/1.1",
    "x-httpbin-trace-host": "bff-756955bb86-465zl/backend-6b9549bc64-ggpws",
    "x-httpbin-trace-service": "bff/backend"
  },
  "method": "GET",
  "origin": "",
  "url": "/",
  "envs": {
    "BACKEND_PORT": "tcp://10.96.92.115:80",
    "BACKEND_PORT_80_TCP": "tcp://10.96.92.115:80",
    "BACKEND_PORT_80_TCP_ADDR": "10.96.92.115",
    "BACKEND_PORT_80_TCP_PORT": "80",
    "BACKEND_PORT_80_TCP_PROTO": "tcp",
    "BACKEND_SERVICE_HOST": "10.96.92.115",
    "BACKEND_SERVICE_PORT": "80",
    "BACKEND_SERVICE_PORT_HTTP": "80",
    "BASIC_PORT": "tcp://10.96.150.13:80",
    "BASIC_PORT_80_TCP": "tcp://10.96.150.13:80",
    "BASIC_PORT_80_TCP_ADDR": "10.96.150.13",
    "BASIC_PORT_80_TCP_PORT": "80",
    "BASIC_PORT_80_TCP_PROTO": "tcp",
    "BASIC_SERVICE_HOST": "10.96.150.13",
    "BASIC_SERVICE_PORT": "80",
    "BASIC_SERVICE_PORT_HTTP": "80",
    "BFF_PORT": "tcp://10.96.144.248:80",
    "BFF_PORT_80_TCP": "tcp://10.96.144.248:80",
    "BFF_PORT_80_TCP_ADDR": "10.96.144.248",
    "BFF_PORT_80_TCP_PORT": "80",
    "BFF_PORT_80_TCP_PROTO": "tcp",
    "BFF_SERVICE_HOST": "10.96.144.248",
    "BFF_SERVICE_PORT": "80",
    "BFF_SERVICE_PORT_HTTP": "80",
    "HOME": "/root",
    "HOSTNAME": "backend-6b9549bc64-ggpws",
    "KUBERNETES_PORT": "tcp://10.96.0.1:443",
    "KUBERNETES_PORT_443_TCP": "tcp://10.96.0.1:443",
    "KUBERNETES_PORT_443_TCP_ADDR": "10.96.0.1",
    "KUBERNETES_PORT_443_TCP_PORT": "443",
    "KUBERNETES_PORT_443_TCP_PROTO": "tcp",
    "KUBERNETES_SERVICE_HOST": "10.96.0.1",
    "KUBERNETES_SERVICE_PORT": "443",
    "KUBERNETES_SERVICE_PORT_HTTPS": "443",
    "NODE_NAME": "higress-worker",
    "PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
    "POD_IP": "10.244.1.5",
    "POD_NAME": "backend-6b9549bc64-ggpws",
    "POD_NAMESPACE": "app-system",
    "SERVICE_ACCOUNT": "backend",
    "SERVICE_NAME": "backend",
    "VERSION": "v1"
  },
  "host_name": "backend-6b9549bc64-ggpws",
  "body": ""
}
```

这里 x-httpbin-trace-host 和 x-httpbin-trace-service 是调用链路经过的 POD_NAME 和 SERVICE_NAME
- "x-httpbin-trace-host": "bff-756955bb86-465zl/backend-6b9549bc64-ggpws"
- "x-httpbin-trace-service": "bff/backend"

skywalking dashboard 调用链路如下

![skywalking.png](images/skywalking.png)

















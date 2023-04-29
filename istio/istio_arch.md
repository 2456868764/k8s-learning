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


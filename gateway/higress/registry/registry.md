# registry 

# nacos
Blog: https://nacos.io/zh-cn/blog/higress.html
## 配置 yaml 
```yaml
apiVersion: networking.higress.io/v1
kind: McpBridge
metadata:
  name: default
  namespace: higress-system
spec:
  registries:
    # 定义一个名为 “production” 的服务来源
  - name: production
    # 注册中心类型是 Nacos 2.x，支持 gRPC 协议
    type: nacos2  
    # 注册中心的访问地址，可以是域名或者IP
    domain: 192.xxx.xx.32
    # 注册中心的访问端口，Nacos 默认都是 8848
    port: 8848
    # Nacos 命名空间 ID
    nacosNamespaceId: d8ac64f3-xxxx-xxxx-xxxx-47a814ecf358    
    # Nacos 服务分组
    nacosGroups:
    - DEFAULT_GROUP
    # 定义一个名为 “uat” 的服务来源
  - name: uat
    # 注册中心类型是 Nacos 1.x，只支持 HTTP 协议
    type: nacos
    # 注册中心的访问地址，可以是域名或者IP
    domain: 192.xxx.xx.31
    # 注册中心的访问端口，Nacos 默认都是 8848
    port: 8848
    # Nacos 命名空间 ID
    nacosNamespaceId: 98ac6df3-xxxx-xxxx-xxxx-ab98115dfde4    
    # Nacos 服务分组
    nacosGroups:
    - DEFAULT_GROUP  
```
通过 McpBridge 资源配置了两个服务来源，分别取名 “production”和“uat”，需要注意的是 Higress 对接 Nacos 同时支持 HTTP 和 gRPC 两种协议，建议将 Nacos 升级到 2.x 版本，这样可以在上述配置的 type 中指定 “nacos2” 使用 gRPC 协议，从而更快速地感知到服务变化，并消耗更少的 Nacos 服务端资源。 基于 McpBridge 中的 registries 数组配置，Higress 可以轻松对接多个且不同类型的服务来源（Nacos/Zookeeper/Eureka/Consul/...），这里对于 Nacos 类型的服务来源，支持配置多个不同命名空间，从而实现不同命名空间的微服务可以共用一个网关，降低自建微服务网关的资源成本开销。

## 配置 Ingress
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    higress.io/destination: service-provider.DEFAULT-GROUP.d8ac64f3-xxxx-xxxx-xxxx-47a814ecf358.nacos
  name: demo
  namespace: default
spec:
  rules:
  - http:
      paths:
      - backend:
          resource:
            apiGroup: networking.higress.io
            kind: McpBridge
            name: default
        path: /
        pathType: Prefix
```

和常见的 Ingress 在 backend 中定义 service 不同，这里基于 Ingress 的 resource backend 将上面定义服务来源的 McpBridge 进行关联。并通过注解higress.io/destination指定路由最终要转发到的目标服务。对于 Nacos 来源的服务，这里的目标服务格式为：“服务名称.服务分组.命名空间ID.nacos”，注意这里需要遵循 DNS 域名格式，因此服务分组中的下划线'_'被转换成了横杠'-'。

## 灰度发布
Higress 完全兼容了 Nginx Ingress 的金丝雀（Canary）相关注解，如下所示，可以将带有HTTP Header为x-user-id: 100的请求流量路由到灰度服务中。

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  annotations:
    higress.io/destination: service-provider.DEFAULT-GROUP.98ac6df3-xxxx-xxxx-xxxx-ab98115dfde4.nacos
    nginx.ingress.kubernetes.io/canary: 'true'
    nginx.ingress.kubernetes.io/canary-by-header: x-user-id
    nginx.ingress.kubernetes.io/canary-by-header-value: '100'
  name: demo-uat
  namespace: default
spec:
  rules:
  - http:
      paths:
      - backend:
          resource:
            apiGroup: networking.higress.io
            kind: McpBridge
            name: default
        path: /
        pathType: Prefix
```
还可以基于 OpenKruise Rollout 让灰度发布和服务部署过程联动，从而实现渐进式交付，具体可以参考这篇文章 [《Higress & Kruise Rollout: 渐进式交付为应用发布保驾护航》](https://mp.weixin.qq.com/s/vqwAUITNq9_twYHWX_5ZDg)


# secrets
1. 创建Secret

```shell
 方法一
[root@k8s-master ~]# echo -n 'admin' > ./username.txt
[root@k8s-master ~]# echo -n '1f2d1e2e67df' > ./password.txt
[root@k8s-master ~]# kubectl create secret generic db-user-pass \
> --from-file=username=./username.txt --from-file=password=./password.txt
secret/db-user-pass created

# 方法二
[root@k8s-master ~]# kubectl create secret generic db-user-pass \
> --from-literal=username=devuser --from-literal=password='S!B\*d$zDsb='
secret/db-user-pass created
```
2. 解码Secret

```shell
[root@k8s-master ~]# kubectl get secret db-user-pass -o jsonpath='{.data}'
{"password":"MWYyZDFlMmU2N2Rm","username":"YWRtaW4="}[root@k8s-master ~]#

# 为了避免在shell历史记录中存储Secret的编码值，建议执行如下命令
[root@k8s-master ~]# kubectl get secret db-user-pass -o jsonpath='{.data.username}' | base64 --decode
admin[root@k8s-master ~]#
[root@k8s-master ~]# kubectl get secret db-user-pass -o jsonpath='{.data.password}' | base64 --decode
1f2d1e2e67df[root@k8s-master ~]#
```

3. 使用stringData字段可以将一个非base64编码的字符串直接放入Secret中，当创建或更新该Secret时，此字段将被编码。

```yaml
piVersion: v1
kind: Secret
metadata:
  name: mysecret-stringdata
type: Opaque
stringData:
  config.yaml: |
    apiUrl: "https://my.api.com/api/v1"
    username: devsuer
    password: Nsjhudt75&
```
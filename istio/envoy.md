# quickstart
1. 网站和文档
```
https://www.envoyproxy.io/
https://www.envoyproxy.io/docs/envoy/latest/intro/what_is_envoy
```
2. func-e 安装和启动

```shell
sudo curl https://func-e.io/install.sh | bash -s -- -b /usr/local/bin
```
或者 https://github.com/tetratelabs/func-e/releases/tag/v1.1.4
下载对应的操作系统版本

```shell

func-e run -c /path/to/envoy.yaml

func-e run --config-yaml "admin: {address: {socket_address: {address: '127.0.0.1', port_value: 9901}}}"

```
```shell
(base) ➜  ~ func-e run --config-yaml "admin: {address: {socket_address: {address: '127.0.0.1', port_value: 9901}}}"

looking up the latest Envoy version
downloading https://archive.tetratelabs.io/envoy/download/v1.25.0/envoy-v1.25.0-darwin-arm64.tar.xz
```

3. docker安装

社区提供的镜像位于 envoyproxy 中，常用的有：

- envoyproxy/envoy-alpine : 基于 alpine 的发行镜像
- envoyproxy/envoy-alpine-dev : 基于 alpine 的 Nightly 版本发行镜像
- envoyproxy/envoy : 基于 Ubuntu 的发行镜像
- envoyproxy/envoy-dev : 基于 Ubuntu 的 Nightly 版本发行镜像

```shell
docker pull envoyproxy/envoy:v1.25-latest
```
启动 Envoy 容器时，可以用本地的 envoy.yaml 覆盖镜像中的 envoy.yaml：

```shell
 docker run -d --network=host -v `pwd`/envoy.yaml:/etc/envoy/envoy.yaml envoyproxy/envoy:v1.25-latest
```

4. Centos  安装

```shell
sudo yum install yum-utils
sudo rpm --import 'https://rpm.dl.getenvoy.io/public/gpg.CF716AF503183491.key'
curl -sL 'https://rpm.dl.getenvoy.io/public/config.rpm.txt?distro=el&codename=7' > /tmp/tetrate-getenvoy-rpm-stable.repo
sudo yum-config-manager --add-repo '/tmp/tetrate-getenvoy-rpm-stable.repo'
sudo yum makecache --disablerepo='*' --enablerepo='tetrate-getenvoy-rpm-stable'
sudo yum install getenvoy-envoy
```
5. Ubuntu Linux 安装

```shell
sudo apt update
sudo apt install apt-transport-https gnupg2 curl lsb-release
curl -sL 'https://deb.dl.getenvoy.io/public/gpg.8115BA8E629CC074.key' | sudo gpg --dearmor -o /usr/share/keyrings/getenvoy-keyring.gpg
Verify the keyring - this should yield "OK"
echo a077cb587a1b622e03aa4bf2f3689de14658a9497a9af2c427bba5f4cc3c4723 /usr/share/keyrings/getenvoy-keyring.gpg | sha256sum --check
echo "deb [arch=amd64 signed-by=/usr/share/keyrings/getenvoy-keyring.gpg] https://deb.dl.getenvoy.io/public/deb/ubuntu $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/getenvoy.list
sudo apt update
sudo apt install -y getenvoy-envoy
```

7. MacOs 安装 

```shell
brew update
brew install envoy
```

# Envoy核心功能

1. 核心功能

- 非侵入的架构 : Envoy 是一个独立进程，设计为伴随每个应用程序服务运行。所有的 Envoy 形成一个透明的通信网格，每个应用程序发送消息到本地主机或从本地主机接收消息，不需要知道网络拓扑，对服务的实现语言也完全无感知，这种模式也被称为 Sidecar。
- L3/L4/L7 架构 : 传统的网络代理，要么在 HTTP 层工作，要么在 TCP 层工作。在 HTTP 层的话，你将会从传输线路上读取整个 HTTP 请求的数据，对它做解析，查看 HTTP 头部和 URL，并决定接下来要做什么。随后，你将从后端读取整个响应的数据，并将其发送给客户端。但这种做法的缺点就是非常复杂和缓慢，更好的选择是下沉到 TCP 层操作：只读取和写入字节，并使用 IP 地址，TCP 端口号等来决定如何处理事务，但无法根据不同的 URL 代理到不同的后端。Envoy 支持同时在 3/4 层和 7 层操作，以此应对这两种方法各自都有其实际限制的现实。
- HTTP/2 支持 : 可以在 HTTP/2 和 HTTP/1.1 之间相互转换（双向），建议使用 HTTP/2。
- 服务发现和动态配置 : 与 Nginx 等代理的热加载不同，Envoy 可以通过 API 来实现其控制平面，控制平面可以集中服务发现，并通过 API 接口动态更新数据平面的配置，不需要重启数据平面的代理。不仅如此，控制平面还可以通过 API 将配置进行分层，然后逐层更新，例如：上游集群中的虚拟主机、HTTP 路由、监听的套接字等。
- gRPC 支持 : gRPC 它使用 HTTP/2 作为底层多路复用传输协议。Envoy 完美支持 HTTP/2，也可以很方便地支持 gRPC
- 特殊协议支持 : Envoy 支持对特殊协议在 L7 进行嗅探和统计，包括：MongoDB[2]、DynamoDB[3] 等。
- 可观测性 : Envoy 的主要目标是使网络透明，可以生成许多流量方面的统计数据，这是其它代理软件很难取代的地方，内置 stats 模块，可以集成诸如 prometheus/statsd 等监控方案。还可以集成分布式追踪系统，对请求进行追踪。

2. 特色

- 性能
  性能：除了大量功外， 还提供极高的 吞吐量和低尾延迟差异，同时消耗相对较少CPU和 RAM；
- 可扩展性
  Envoy 在L4 和L7 上提供丰富的可插拔过滤器功能，允许用户轻松添加新功能
- API可配置性
  Envoy提供了一组可由控制平面服务实现的管理 API，也称为 xDS API
  若控制平面实现了这些API ，则可以使用引导配置在整个基础架构中运行 Envoy 所有的配置更改都可通过管理服务器无缝地行动态传递，Envoy不需要重新启动
  于是，这使得 Envoy 成为一个通用数据平面，当与足够复杂的控制面相结合时可大大地降低整体操作性， Envoy已经成为现代服务网格和边缘关的 “ 通用数据平面 API ” ，包括Istio 、Ambassador和 Gloo 等项目
  
3. 整体架构

![架构](./images/envoy_arch.png)

Envoy 接收到请求后，会先走 FilterChain，通过各种 L3/L4/L7 Filter 对请求进行微处理，然后再路由到指定的集群，并通过负载均衡获取一个目标地址，最后再转发出去。

其中每一个环节可以静态配置，也可以动态服务发现，也就是所谓的 xDS

术语介绍
- 下游（ DownstreamDownstream ）：

下游主机连接到 Envoy ，发送请求并接收响应它们是 Envoy 的客户端

- 上游（ Upstream）: 

上游主机接收来自 Envoy 的连接和请求并返回响应，它们是 Envoy 代理的后端服务器


- 监听器（ Listener）: 

监听器是能够由下游客户端连接的命名网络位置，例如端口或 unix 域套接字等， Envoy 会暴露一个或者多个listener监听downstream的请求。
- 集群 (Cluster ): 

服务提供方集群。Envoy 通过服务发现定位集群成员并获取服务。 具体请求到哪个集群成员是由负载均衡策略决定。
- 端点（ Endpoint）:

端点即上游主机，是一个或多集群的成员可通过 EDS 发现
- 路由 （ Router ）：

上下游之间的桥梁， Listener可以接收来自下游的连接，Cluster可以将流量发送给具体的上游服务，而Router则决定Listener在接收到下游连接和数据之后，应该将数据交给哪一个Cluster处理。

它定义了数据分发的规则。虽然说到Router大部分时候都可以默认理解为HTTP路由，但是Envoy支持多种协议，如Dubbo、Redis等，所以此处Router泛指所有用于桥接Listener和后端服务（不限定HTTP）的规则与资源集合。

Route对应的配置/资源发现服务称之为 RDS 发现。Router中最核心配置包含匹配规则和目标Cluster，此外，也可包含重试、分流、限流等等。

- 过滤器（ Filter ）: 

在 Envoy 中指的是一些“可插拔”和可组合的逻辑处理层。是 Envoy 核心逻辑处理单元。

xDS以及各个资源之间的关系下图所示。

![xds](./images/envoy_xds.png)





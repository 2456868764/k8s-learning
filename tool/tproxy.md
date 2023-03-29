# tproxy

## 官方URL

- URL ： https://github.com/kevwan/tproxy/
- 视频：  https://www.bilibili.com/video/BV16d4y1S7xA

## 安装

```shell
$ GOPROXY=https://goproxy.cn/,direct go install github.com/kevwan/tproxy@latest
```

或者使用 docker 镜像：

```shell
$ docker run --rm -it -p <listen-port>:<listen-port> -p <remote-port>:<remote-port> kevinwan/tproxy:v1 tproxy -l 0.0.0.0 -p <listen-port> -r host.docker.internal:<remote-port>
```

arm64 系统:

```shell
$ docker run --rm -it -p <listen-port>:<listen-port> -p <remote-port>:<remote-port> kevinwan/tproxy:v1-arm64 tproxy -l 0.0.0.0 -p <listen-port> -r host.docker.internal:<remote-port>
```

Windows:

```shell
$ scoop install tproxy
```

## 用法

```shell
$ tproxy --help
Usage of tproxy:
  -d duration
    	the delay to relay packets
  -l string
    	Local address to listen on (default "localhost")
  -p int
    	Local port to listen on, default to pick a random port
  -q	Quiet mode, only prints connection open/close and stats, default false
  -r string
    	Remote address (host:port) to connect
  -s	Enable statistics
  -t string
    	The type of protocol, currently support http2, grpc, redis and mongodb
```

## 示例

### 分析 gRPC 连接

```shell
$ tproxy -p 8088 -r localhost:8081 -t grpc -d 100ms
```

- 侦听在 localhost 和 8088 端口
- 重定向请求到 `localhost:8081`
- 识别数据包格式为 gRPC
- 数据包延迟100毫秒

<img width="579" alt="image" src="https://user-images.githubusercontent.com/1918356/181794530-5b25f75f-0c1a-4477-8021-56946903830a.png">

### 分析 MySQL 连接

```shell
$ tproxy -p 3307 -r localhost:3306
```

<img width="600" alt="image" src="https://user-images.githubusercontent.com/1918356/173970130-944e4265-8ba6-4d2e-b091-1f6a5de81070.png">

### 查看网络状况（重传率和RTT）

```shell
$ tproxy -p 3307 -r remotehost:3306 -s -q
```

<img width="548" alt="image" src="https://user-images.githubusercontent.com/1918356/180252614-7cf4d1f9-9ba8-4aa4-a964-6f37cf991749.png">




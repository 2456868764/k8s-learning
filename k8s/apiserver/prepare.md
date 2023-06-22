# Api Server 本地调试环境搭建

## Etcd 本地单机版本搭建

1. 使用 etcdkeeper 可视化管理etcd

https://github.com/evildecay/etcdkeeper/releases/

2. 安装 etcd
安装 ：https://etcd.io/docs/v3.5/install/

```shell
$ brew update
$ brew install etcd
$ etcd --version
```
3. 启动 etcd 和 测试

```shell
$ etcd
{"level":"info","ts":"2021-09-17T09:19:32.783-0400","caller":"etcdmain/etcd.go:72","msg":... }

$ etcdctl put greeting "Hello, etcd"
$ etcdctl get greeting
```

## 启动 etcdkeeper
```shell
$ ./etcdkeeper
2023-06-22 07:43:44.741555 I | listening on 0.0.0.0:8080
2023-06-22 07:43:57.062496 I | POST v3 connect success.
2023-06-22 07:43:57.073476 I | GET v3 /
```

## 证书配置

## OpenApi Swagger导入

## Debug 调试


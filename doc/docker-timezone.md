# Docker 容器时区问题

1. 问题

kubectl exec -it podName -n namespace /bin/sh

进入容器运行 date 命令，发现时区不对是 UTC 时区，造成公司日志系统无法采集日志，需要改为 UTC+8 北京时间。

2. 解决

需要更改 Dockerfile，在 build image 时修改时区.

```shell
FROM alpine:3.11.6

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories

ENV TZ=Asia/Shanghai
RUN apk update \
    && apk add tzdata \
    && echo "${TZ}" > /etc/timezone \
    && ln -sf /usr/share/zoneinfo/${TZ} /etc/localtime \
    && rm /var/cache/apk/*

```

我的基础镜像是 alpine ，可以使用以上的设置。

RUN sed -i 's/dl-cdn.alpinelinux.org/mirrors.aliyun.com/g' /etc/apk/repositories 表示使用阿里镜像包源，可以使安装 tzdata 加速，不然很慢


3.其他系统

基于 Debian

直接设置环境变量即可，默认安装了 tzdata

```shell
ENV TZ=Asia/Shanghai
```

基于 Ubuntu

```shell
FROM ubuntu:bionic
 
ENV TZ=Asia/Shanghai

RUN echo "${TZ}" > /etc/timezone \
    && ln -sf /usr/share/zoneinfo/${TZ} /etc/localtime \
    && apt update \
    && apt install -y tzdata \
    && rm -rf /var/lib/apt/lists/*
```

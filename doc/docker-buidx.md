# docker buildx

## 文档

1. [install](https://docs.docker.com/build/install-buildx/)
2. [how to use](https://docs.docker.com/engine/reference/commandline/buildx/)

## 基本使用

1. buildx 和  build

buildx 和 docker build 命令的使用体验基本一致，还支持 build 常用的选项如 -t、-f等。

2. builder 实例

docker buildx 通过 builder 实例对象来管理构建配置和节点，命令行将构建任务发送至 builder 实例，再由 builder 指派给符合条件的节点执行。我们可以基于同一个 docker 服务程序创建多个 builder 实例，提供给不同的项目使用以隔离各个项目的配置，也可以为一组远程 docker 节点创建一个 builder 实例组成构建阵列，并在不同阵列之间快速切换。

使用 docker buildx create 命令可以创建 builder 实例，这将以当前使用的 docker 服务为节点创建一个新的 builder 实例。要使用一个远程节点，可以在创建示例时通过 DOCKER_HOST 环境变量指定远程端口或提前切换到远程节点的 docker context。

```shell
base) ➜  k8s-learning git:(main) ✗ docker buildx create --help

Usage:  docker buildx create [OPTIONS] [CONTEXT|ENDPOINT]

Create a new builder instance

Options:
      --append                   Append a node to builder instead of changing it
      --bootstrap                Boot builder after creation
      --buildkitd-flags string   Flags for buildkitd daemon
      --config string            BuildKit config file
      --driver string            Driver to use (available: "docker-container", "kubernetes", "remote")
      --driver-opt stringArray   Options for the driver
      --leave                    Remove a node from builder instead of changing it
      --name string              Builder instance name
      --node string              Create/modify node with given name
      --platform stringArray     Fixed platforms for current node
      --use                      Set the current builder instance

```

docker buildx ls 将列出所有可用的 builder 实例和实例中的节点：

```shell
(base) ➜ docker buildx ls
NAME/NODE       DRIVER/ENDPOINT STATUS  BUILDKIT PLATFORMS
default *       docker                           
  default       default         running 20.10.23 linux/arm64, linux/amd64, linux/riscv64, linux/ppc64le, linux/s390x, linux/386, linux/arm/v7, linux/arm/v6
desktop-linux   docker                           
  desktop-linux desktop-linux   running 20.10.23 linux/arm64, linux/amd64, linux/riscv64, linux/ppc64le, linux/s390x, linux/386, linux/arm/v7, linux/arm/v6

```

## 构建多个架构 Go 镜像实践


1. Dockerfile

```shell
# Build the manager binary
FROM golang:1.19 as builder

ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/

RUN go env -w GOPROXY=https://goproxy.cn,direct
# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM alpine:3.13.5

RUN apk add --no-cache tzdata && \
    ln -sf /usr/share/zoneinfo/Asia/Shanghai  /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone


WORKDIR /
COPY --from=builder /workspace/manager .
#USER 65532:65532

ENTRYPOINT ["/manager"]


```
2. Docker buildx

```shell
# PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
PLATFORMS ?= linux/arm64,linux/amd64
.PHONY: docker-buildx
docker-buildx: test ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- docker buildx create --name project-v3-builder
	docker buildx use project-v3-builder
	- docker buildx build --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .  --output type=local,dest=./output
	- docker buildx rm project-v3-builder
	rm Dockerfile.cross
```


3. 构建过程分为两个阶段

- 在一阶段中，我们将拉取一个和当前构建节点相同平台的 golang 镜像，并使用 Go 的交叉编译特性将其编译为目标架构的二进制文件。
- 然后拉取目标平台的 alpine 镜像，并将上一阶段的编译结果拷贝到镜像中。

4. 执行跨平台构建

执行构建命令时，除了指定镜像名称，另外两个重要的选项是指定目标平台和输出格式。

docker buildx build 通过 --platform 选项指定构建的目标平台。Dockerfile 中的 FROM 指令如果没有设置 --platform 标志，就会以目标平台拉取基础镜像，最终生成的镜像也将属于目标平台。

此外 Dockerfile 中可通过 BUILDPLATFORM、TARGETPLATFORM、BUILDARCH 和 TARGETARCH 等参数使用该选项的值。

当使用 docker-container 驱动时，这个选项可以接受用逗号分隔的多个值作为输入以同时指定多个目标平台，所有平台的构建结果将合并为一个整体的镜像列表作为输出，因此无法直接输出为本地的 docker images 镜像。

docker buildx build 支持丰富的输出行为，通过--output=[PATH,-,type=TYPE[,KEY=VALUE] 选项可以指定构建结果的输出类型和路径等，常用的输出类型有以下几种：

- local：构建结果将以文件系统格式写入 dest 指定的本地路径， 如 --output type=local,dest=./output。
- tar：构建结果将在打包后写入 dest 指定的本地路径。
- oci：构建结果以 OCI 标准镜像格式写入 dest 指定的本地路径。
- docker：构建结果以 Docker 标准镜像格式写入 dest 指定的本地路径或加载到 docker 的镜像库中。同时指定多个目标平台时无法使用该选项。
- image：以镜像或者镜像列表输出，并支持 push=true 选项直接推送到远程仓库，同时指定多个目标平台时可使用该选项。
- registry：type=image,push=true 的精简表示。

5. 支持构建镜像

PLATFORMS ?= linux/arm64,linux/amd64  同时构建 linux/amd64、 linux/arm64镜像
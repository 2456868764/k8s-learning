# kubernetes tool

## kubectx + kubens: Power tools for kubectl
What are kubectx and kubens?
kubectx is a tool to switch between contexts (clusters) on kubectl faster.
kubens is a tool to switch between Kubernetes namespaces (and configure them for kubectl) easily.

1. 安装

https://github.com/ahmetb/kubectx#homebrew-macos-and-linux 

2. 使用
```shell
# switch to another cluster that's in kubeconfig
$ kubectx minikube
Switched to context "minikube".

# switch back to previous cluster
$ kubectx -
Switched to context "oregon".

# rename context
$ kubectx dublin=gke_ahmetb_europe-west1-b_dublin
Context "gke_ahmetb_europe-west1-b_dublin" renamed to "dublin".

# change the active namespace on kubectl
$ kubens kube-system
Context "test" set.
Active namespace is "kube-system".

# go back to the previous namespace
$ kubens -
Context "test" set.
Active namespace is "default".

```

## kubefwd (Kube Forward)

kubefwd is a command line utility built to port forward multiple services within one or more namespaces on one or more Kubernetes clusters. kubefwd uses the same port exposed by the service and forwards it from a loopback IP address on your local workstation. kubefwd temporally adds domain entries to your /etc/hosts file with the service names it forwards.

When working on our local workstation, my team and I often build applications that access services through their service names and ports within a Kubernetes namespace. kubefwd allows us to develop locally with services available as they would be in the cluster.

![图片](https://camo.githubusercontent.com/6c35bdcb997d0b6456109962e42d6bdee9ed31a524f8b6b830e8c39e8e9fc901/68747470733a2f2f6d6b2e696d74692e636f2f696d616765732f636f6e74656e742f6b7562656677642d6e65742e706e67)

1. install

https://github.com/txn2/kubefwd

2. usage

Forward all svc for the namespace the-projec

```shell
 sudo kubefwd svc -n the-project

Forward all svc for the namespace the-project where labeled system: wx:

```shell

sudo kubefwd svc -l system=wx -n the-project

```

Forward a single service named my-service in the namespace the-project:

```shell
sudo kubefwd svc -n the-project -f metadata.name=my-service
```

Forward more than one service using the in clause:

```shell
sudo kubefwd svc -l "app in (app1, app2)"

```

## The Kubernetes IDE

官网： https://k8slens.dev/






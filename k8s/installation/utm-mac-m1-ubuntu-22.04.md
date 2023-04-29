## install k8s on mac m1

### 官方安装

https://kubernetes.io/zh-cn/docs/setup/production-environment/tools/kubeadm/install-kubeadm/

### get utm
```
https://getutm.app/
```
###  download ubuntu 20.4 and setup ubuntu VM

https://mac.getutm.app/gallery/ubuntu-20-04

### Login to the system and set ip for second adapter

```sh
vi /etc/netplan/00-installer-config.yaml

network:
  ethernets:
    enp0s1:
      dhcp4: no
      gateway4: 192.168.64.1
      nameservers:
        addresses:
          - 192.168.32.1
      addresses:
        - 192.168.64.16/24
  version: 2
```

```sh
sudo netplan apply
```

### Network configuration

Now your VM has two adapters:

- One is NAT which will get an IP automatically, generally it's 10.0.2.15, this interface is for external access from your VM
- One is host adapter which need create extra ip, which is configured as 192.168.34.2
  the reason we need the host adapter and static IP is then we can set this static IP as k8s advertise IP and you can move your VM in different everywhere.(otherwise your VM IP would be changed in different environment)

### Set no password for sudo

```sh
sudo vim /etc/sudoers

%sudo ALL=(ALL:ALL) NOPASSWD:ALL
```

### Swap off
remove the line with swap keyword
```sh
sudo swapoff -a
sudo vi /etc/fstab
```

### Letting iptables see bridged traffic

```shell
$ cat <<EOF | sudo tee /etc/modules-load.d/k8s.conf
overlay
br_netfilter
EOF

$ sudo modprobe overlay
$ sudo modprobe br_netfilter

$ cat <<EOF | sudo tee /etc/sysctl.d/k8s.conf
net.bridge.bridge-nf-call-iptables  = 1
net.bridge.bridge-nf-call-ip6tables = 1
net.ipv4.ip_forward                 = 1
EOF


$ sudo sysctl --system

$ lsmod | grep br_netfilter
$ lsmod | grep overlay
```

### Add the Kubernetes apt repository

```shell
$ cat <<EOF | sudo tee /etc/apt/sources.list.d/kubernetes.list
 deb https://mirrors.aliyun.com/kubernetes/apt kubernetes-xenial main
EOF
```

### Add Docker’s official GPG key:

```shell
$ curl -s https://mirrors.aliyun.com/kubernetes/apt/doc/apt-key.gpg | sudo apt-key add -
```



### Update the apt package index and install packages to allow apt to use a repository over HTTPS:

```shell
$ sudo apt-get update
```

```shell
$ sudo apt-get install \
    apt-transport-https \
    ca-certificates \
    curl \
    gnupg-agent \
    software-properties-common
```


## 开启ipvs(可以跳过)
在kubernetes中service有两种代理模型，一种是基于iptables（链表），另一种是基于ipvs（hash表）。ipvs的性能要高于iptables的，但是如果要使用它，需要手动载入ipvs模块。

```shell
# 安装ipset和ipvsadm：
sudo apt install -y ipset ipvsadm

# 配置加载模块
cat << EOF | sudo tee /etc/modules-load.d/ipvs.conf 
modprobe -- ip_vs
modprobe -- ip_vs_rr
modprobe -- ip_vs_wrr
modprobe -- ip_vs_sh
modprobe -- nf_conntrack
EOF


# 临时加载
modprobe -- ip_vs
modprobe -- ip_vs_rr
modprobe -- ip_vs_wrr
modprobe -- ip_vs_sh

# 开机加载配置，将ipvs相关模块加入配置文件中
cat <<EOF | sudo tee /etc/modules 
ip_vs_sh
ip_vs_wrr
ip_vs_rr
ip_vs
nf_conntrack
EOF
```


## Installing containerd runtime

```shell
sudo apt install -y containerd
sudo mkdir -p /etc/containerd
containerd config default | sudo tee /etc/containerd/config.toml
sudo systemctl restart containerd
```

### Update cgroupdriver to systemd
在 /etc/containerd/config.toml 中设置

```sh

sudo systemctl daemon-reload && systemctl restart containerd
sudo systemctl status containerd

# sandbox_image镜像源设置为阿里云google_containers镜像源
sudo sed -i "s#registry.k8s.io/pause:3.6#registry.aliyuncs.com/google_containers/pause:3.9#g"  /etc/containerd/config.toml

# 修改Systemdcgroup
sudo sed -i 's#SystemdCgroup = false#SystemdCgroup = true#g' /etc/containerd/config.toml

```


```shell
sudo systemctl restart containerd

```

### # 查看版本

```shell
apt-cache madison kubeadm|head

 kubeadm |  1.27.1-00 | https://mirrors.aliyun.com/kubernetes/apt kubernetes-xenial/main arm64 Packages
   kubeadm |  1.27.0-00 | https://mirrors.aliyun.com/kubernetes/apt kubernetes-xenial/main arm64 Packages
   kubeadm |  1.26.4-00 | https://mirrors.aliyun.com/kubernetes/apt kubernetes-xenial/main arm64 Packages
   kubeadm |  1.26.3-00 | https://mirrors.aliyun.com/kubernetes/apt kubernetes-xenial/main arm64 Packages
   kubeadm |  1.26.2-00 | https://mirrors.aliyun.com/kubernetes/apt kubernetes-xenial/main arm64 Packages
   kubeadm |  1.26.1-00 | https://mirrors.aliyun.com/kubernetes/apt kubernetes-xenial/main arm64 Packages
   kubeadm |  1.26.0-00 | https://mirrors.aliyun.com/kubernetes/apt kubernetes-xenial/main arm64 Packages
   kubeadm |  1.25.9-00 | https://mirrors.aliyun.com/kubernetes/apt kubernetes-xenial/main arm64 Packages
   kubeadm |  1.25.8-00 | https://mirrors.aliyun.com/kubernetes/apt kubernetes-xenial/main arm64 Packages
   kubeadm |  1.25.7-00 | https://mirrors.aliyun.com/kubernetes/apt kubernetes-xenial/main arm64 Packages

```

### Update the apt package index, and install the latest version of Docker Engine and containerd, or go to the next step to install a specific version:


```shell
$ sudo apt-get update
$ sudo apt-get install -y kubelet=1.27.1-00 kubeadm=1.27.1-00 kubectl=1.27.1-00
$ sudo apt-mark hold kubelet kubeadm kubectl
```

### 设置crictl

```shell

cat << EOF | sudo tee /etc/crictl.yaml 
runtime-endpoint: unix:///var/run/containerd/containerd.sock
image-endpoint: unix:///var/run/containerd/containerd.sock
timeout: 10
debug: false
EOF
```


### kubeadm init

```shell
$ sudo kubeadm init \
 --image-repository registry.aliyuncs.com/google_containers \
 --kubernetes-version v1.27.1 \
 --pod-network-cidr=10.10.0.0/16 \
 --apiserver-advertise-address=192.168.64.16
```


### Copy kubeconfig

```shell
Your Kubernetes control-plane has initialized successfully!

To start using your cluster, you need to run the following as a regular user:

  mkdir -p $HOME/.kube
  sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
  sudo chown $(id -u):$(id -g) $HOME/.kube/config

Alternatively, if you are the root user, you can run:

  export KUBECONFIG=/etc/kubernetes/admin.conf

You should now deploy a pod network to the cluster.
Run "kubectl apply -f [podnetwork].yaml" with one of the options listed at:
  https://kubernetes.io/docs/concepts/cluster-administration/addons/

Then you can join any number of worker nodes by running the following on each as root:

kubeadm join 192.168.64.16:6443 --token oc9xch.aqw7m3oqny1hmyvr \
        --discovery-token-ca-cert-hash sha256:7abc5ab8e66376204beaf43432d17bb5871bd0893b980755f805ce8dd5e0cee2 
```


### Untaint master

```shell
$ kubectl taint nodes --all node-role.kubernetes.io/control-plane-
```

### join 
```bigquery

Your Kubernetes control-plane has initialized successfully!

To start using your cluster, you need to run the following as a regular user:

mkdir -p $HOME/.kube
sudo cp -i /etc/kubernetes/admin.conf $HOME/.kube/config
sudo chown $(id -u):$(id -g) $HOME/.kube/config
export KUBECONFIG=$HOME/.kube/config
Alternatively, if you are the root user, you can run:

export KUBECONFIG=/etc/kubernetes/admin.conf

  You should now deploy a pod network to the cluster.
  Run "kubectl apply -f [podnetwork].yaml" with one of the options listed at:
                                             https://kubernetes.io/docs/concepts/cluster-administration/addons/

                                             Then you can join any number of worker nodes by running the following on each as root:
  Then you can join any number of worker nodes by running the following on each as root:

  kubeadm join 192.168.64.16:6443 --token 5t0gju.ip6zwnsfr54upidy \
--discovery-token-ca-cert-hash sha256:7f68b6feb453397d34be08b8a44aefd24568ad816108e6824760186a29f94716 
```

### install calico 
```
[root@vm210 ~]# wget https://docs.projectcalico.org/manifests/tigera-operator.yaml
[root@vm210 ~]# wget https://docs.projectcalico.org/manifests/custom-resources.yaml
// you should let the cidr match your network range, don not use the url to install directly.
[root@vm210 ~]# vim custom-resources.yaml
cidr: 192.168.0.0/16  => cidr: 10.10.0.0/16
[root@vm210 ~]# kubectl create -f tigera-operator.yaml
[root@vm210 ~]# kubectl create -f custom-resources.yaml

```

### watch calico
```shell
 watch kubectl get pods -n calico-system
```

### check installation

```shell
 kubectl apply -f busybox-deploy.yaml
```

### Reference
* https://zhuanlan.zhihu.com/p/563177876
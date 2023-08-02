# consul install on docker


## consul docker
```shell

docker run -d -p 8500:8500 -p 8300:8300 -p 8301:8301 -p 8302:8302 -p 8600:8600/udp -v /Users/jun/appdata/consul/data:/consul/data -v /Users/jun/appdata/consul/config:/consul/config -v /Users/jun/appdata/consul/log:/consul/log --name=consul_server_1 consul consul agent -client=0.0.0.0 -data-dir=/consul/data -config-dir=/consul/config

 
docker run -d \
--name=registrator \
--net=host \
-v /var/run/docker.sock:/tmp/docker.sock \
--restart=always \
gliderlabs/registrator:latest \
-ip=172.17.0.3 \
consul://172.17.0.2:8500


docker run -itd -p:81:80 --name test-01 -h test01 nginx
docker run -itd -p:82:80 --name test-02 -h test02 nginx
docker run -itd -p:83:80 --name test-03 -h test03 httpd
docker run -itd -p:84:80 --name test-04 -h test04 httpd

```

## 启用ACL，配置master token

1. config.json

```json
{
    "datacenter": "dc1",
    "bootstrap_expect": 1,
    "data_dir": "/consul/data",
    "log_file": "/consul/log/",
    "log_level": "INFO",
    "node_name": "consul_server_1",
    "client_addr": "0.0.0.0",
    "server": true,
    "ui": true,
    "enable_script_checks": true,
    "addresses": {
        "https": "0.0.0.0",
        "dns": "0.0.0.0"
    }
}

```

2. 添加配置文件acl.hcl

```shell
primary_datacenter = "dc1"
acl {
  enabled = true
  default_policy = "allow"
  enable_token_persistence = true
  tokens { 
  }
}

```

3. 重载配置文件，创建初始token，生成的SecretID就是token

```shell
docker exec consul_server_1 consul reload 
```

```shell
consul acl bootstrap
```

出现： Failed ACL bootstrapping: Unexpected response code: 403 (Permission denied: ACL bootstrap no longer allowed (reset index: 5))

执行以下：
```shell
cd /Users/jun/appdata/consul/data
echo 5 > acl-bootstrap-reset
```

重新执行 consul acl bootstrap, 生成的SecretID就是token

```shell
/ # consul acl bootstrap
AccessorID:       47a3ebd3-c049-3d91-710d-072375affac8
SecretID:         33ba0a2d-7be2-661f-c495-cc16fce60adb
Description:      Bootstrap Token (Global Management)
Local:            false
Create Time:      2023-07-20 23:59:05.726901803 +0000 UTC
Policies:
   00000000-0000-0000-0000-000000000001 - global-management
```

4. acl.hcl

```shell
primary_datacenter = "dc1"
acl {
  enabled = true
  default_policy = "allow"
  enable_token_persistence = true
  tokens { 
    master = "eaef9b03-23b1-d84e-d4e3-8467a8fabdda"  
  }
}
```

5. 不是在docker启动的consul，可以通过增加环境变量CONSUL_HTTP_TOKEN代替每次命令后带token参数

```shell
export CONSUL_HTTP_TOKEN=33ba0a2d-7be2-661f-c495-cc16fce60adb
```

6. create new token for serive edit and view
用API生成全权限的token作为agent token，可以根据实际情况分配 token的权限
```shell
curl -X PUT \
  http://localhost:8510/v1/acl/create \
  -H 'X-Consul-Token: dcb93655-0661-4ea1-bfc4-e5744317f99e' \
  -d '{"Name": "dc1","Type": "management"}'

```

7. rules

higress policy
```shell

node_prefix "" {
   policy = "read"
}
 
service_prefix "" {
   policy = "read"
}
```

higress token "4add1665-5394-7c55-23a5-70806d51bc42"

service policy

```shell
node_prefix "" {
   policy = "write"
}
 
service_prefix "" {
   policy = "write"
}
```
service token: 413f6518-d69e-4f01-e784-f964a6e459cf

# consul reference
- [watcher] https://blog.csdn.net/qq_42009262/article/details/128311946
- [watcher] https://juejin.cn/post/6984378158347157512
- [install&Register] https://blog.csdn.net/weixin_56270746/article/details/125915275
- [多数据中心] https://cloud.tencent.com/developer/article/2141110
- [docker] https://developer.hashicorp.com/consul/tutorials/docker/docker-container-agents
- https://blog.csdn.net/manzhizhen/article/details/122731072
- https://www.codenong.com/cs105951195/
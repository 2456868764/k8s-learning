# istio流量劫持原理

## 环境介绍

1. 创建 namespace : istio-demo, 同时 允许 istio 注入 sidecar

```shell
  kubectl create namespace istio-demo
  kubectl label namespace istio-demo istio-injection=enabled --overwrite
```
 
2. 启动 sleep 服务 examples/sleep/sleep.yaml

```yaml

apiVersion: v1
kind: ServiceAccount
metadata:
  name: sleep
---
apiVersion: v1
kind: Service
metadata:
  name: sleep
  labels:
    app: sleep
    service: sleep
spec:
  ports:
  - port: 80
    name: http
  selector:
    app: sleep
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: sleep
spec:
  replicas: 1
  selector:
    matchLabels:
      app: sleep
  template:
    metadata:
      labels:
        app: sleep
    spec:
      terminationGracePeriodSeconds: 0
      serviceAccountName: sleep
      containers:
      - name: sleep
        image: curlimages/curl
        command: ["/bin/sleep", "infinity"]
        imagePullPolicy: IfNotPresent
        volumeMounts:
        - mountPath: /etc/sleep/tls
          name: secret-volume
      volumes:
      - name: secret-volume
        secret:
          secretName: sleep-secret
          optional: true
```

```bash
kubectl apply -f examples/sleep/sleep.yaml -n istio-demo
````

3. 启动 httpbin 服务 examples/httpbin/httpbin.yaml

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: httpbin
---
apiVersion: v1
kind: Service
metadata:
  name: httpbin
  labels:
    app: httpbin
    service: httpbin
spec:
  ports:
  - name: http
    port: 8000
    targetPort: 80
  selector:
    app: httpbin
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: httpbin
spec:
  replicas: 1
  selector:
    matchLabels:
      app: httpbin
      version: v1
  template:
    metadata:
      labels:
        app: http
        version: v1
    spec:
      serviceAccountName: httpbin
      containers:
      - image: docker.io/2456868764/httpbin:1.0.0
        imagePullPolicy: IfNotPresent
        name: httpbin
        ports:
        - containerPort: 80
        env:
          - name: POD_NAME
            valueFrom:
              fieldRef:
                fieldPath: metadata.name
          - name: POD_NAMESPACE
            valueFrom:
              fieldRef:
                fieldPath: metadata.namespace
          - name: POD_IP
            valueFrom:
              fieldRef:
                fieldPath: status.podIP
          - name: NODE_NAME
            valueFrom:
              fieldRef:
                fieldPath: spec.nodeName
          - name: SERVICE_ACCOUNT
            valueFrom:
              fieldRef:
                fieldPath: spec.serviceAccountName

```

```bash
kubectl apply -f examples/httpbin/httpbin.yaml -n istio-demo
````

4. 启动 gateway 访问 httpbin

  ```shell
  kubectl apply -f samples/httpbin/httpbin-gateway.yaml -n istio-demo
  ```
5. 测试 

```shell
kubectl get pods -n istio-demo
NAME                       READY   STATUS    RESTARTS        AGE
httpbin-798dbb9f74-l7ntg   2/2     Running   2 (6m53s ago)   18d
sleep-9454cc476-8592b      2/2     Running   2 (6m53s ago)   18d
```

```shell
export SLEEP_POD=$(kubectl get pods -l app=sleep -o 'jsonpath={.items[0].metadata.name}' -n istio-demo)
kubectl exec "$SLEEP_POD" -n istio-demo -c sleep -- curl -sS http://httpbin:8000/hostname

httpbin-798dbb9f74-l7ntg"
```


## 流量访问原理

通过 curl -sS http://httpbin:8000/hostname 从 SLEEP_POD 访问 httpbin service， 来分析流量







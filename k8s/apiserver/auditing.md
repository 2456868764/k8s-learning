# Auditing 审计

文档位置： https://kubernetes.io/zh-cn/docs/tasks/debug/debug-cluster/audit/

## 审计 

Kubernetes 审计（Auditing） 功能提供了与安全相关的、按时间顺序排列的记录集， 记录每个用户、使用 Kubernetes API 的应用以及控制面自身引发的活动。

审计功能使得集群管理员能够回答以下问题：

- 发生了什么？
- 什么时候发生的？
- 谁触发的？
- 活动发生在哪个（些）对象上？
- 在哪观察到的？
- 它从哪触发的？
- 活动的后续处理行为是什么？

审计记录最初产生于 kube-apiserver 内部。每个请求在不同执行阶段都会生成审计事件；这些审计事件会根据特定策略被预处理并写入后端。 策略确定要记录的内容和用来存储记录的后端，当前的后端支持日志文件和 webhook。

每个请求都可被记录其相关的阶段（stage）。已定义的阶段有：

- RequestReceived - 此阶段对应审计处理器接收到请求后， 并且在委托给其余处理器之前生成的事件。
- ResponseStarted - 在响应消息的头部发送后，响应消息体发送前生成的事件。 只有长时间运行的请求（例如 watch）才会生成这个阶段。
- ResponseComplete - 当响应消息体完成并且没有更多数据需要传输的时候。
- Panic - 当 panic 发生时生成。

## 审计策略

审计策略定义了关于应记录哪些事件以及应包含哪些数据的规则。 审计策略对象结构定义在 audit.k8s.io API 组。 处理事件时，将按顺序与规则列表进行比较。第一个匹配规则设置事件的审计级别（Audit Level）。 已定义的审计级别有：

- None - 符合这条规则的日志将不会记录。
- Metadata - 记录请求的元数据（请求的用户、时间戳、资源、动词等等）， 但是不记录请求或者响应的消息体。
- Request - 记录事件的元数据和请求的消息体，但是不记录响应的消息体。 这不适用于非资源类型的请求。
- RequestResponse - 记录事件的元数据，请求和响应的消息体。这不适用于非资源类型的请求。

可以使用 --audit-policy-file 标志将包含策略的文件传递给 kube-apiserver。 如果不设置该标志，则不记录事件。 注意 rules 字段必须在审计策略文件中提供。没有（0）规则的策略将被视为非法配置。

```yaml
apiVersion: audit.k8s.io/v1 # 这是必填项。
kind: Policy
# 不要在 RequestReceived 阶段为任何请求生成审计事件。
omitStages:
  - "RequestReceived"
rules:
  # 在日志中用 RequestResponse 级别记录 Pod 变化。
  - level: RequestResponse
    resources:
    - group: ""
      # 资源 "pods" 不匹配对任何 Pod 子资源的请求，
      # 这与 RBAC 策略一致。
      resources: ["pods"]
  # 在日志中按 Metadata 级别记录 "pods/log"、"pods/status" 请求
  - level: Metadata
    resources:
    - group: ""
      resources: ["pods/log", "pods/status"]

  # 不要在日志中记录对名为 "controller-leader" 的 configmap 的请求。
  - level: None
    resources:
    - group: ""
      resources: ["configmaps"]
      resourceNames: ["controller-leader"]

  # 不要在日志中记录由 "system:kube-proxy" 发出的对端点或服务的监测请求。
  - level: None
    users: ["system:kube-proxy"]
    verbs: ["watch"]
    resources:
    - group: "" # core API 组
      resources: ["endpoints", "services"]

  # 不要在日志中记录对某些非资源 URL 路径的已认证请求。
  - level: None
    userGroups: ["system:authenticated"]
    nonResourceURLs:
    - "/api*" # 通配符匹配。
    - "/version"

  # 在日志中记录 kube-system 中 configmap 变更的请求消息体。
  - level: Request
    resources:
    - group: "" # core API 组
      resources: ["configmaps"]
    # 这个规则仅适用于 "kube-system" 名字空间中的资源。
    # 空字符串 "" 可用于选择非名字空间作用域的资源。
    namespaces: ["kube-system"]

  # 在日志中用 Metadata 级别记录所有其他名字空间中的 configmap 和 secret 变更。
  - level: Metadata
    resources:
    - group: "" # core API 组
      resources: ["secrets", "configmaps"]

  # 在日志中以 Request 级别记录所有其他 core 和 extensions 组中的资源操作。
  - level: Request
    resources:
    - group: "" # core API 组
    - group: "extensions" # 不应包括在内的组版本。

  # 一个抓取所有的规则，将在日志中以 Metadata 级别记录所有其他请求。
  - level: Metadata
    # 符合此规则的 watch 等长时间运行的请求将不会
    # 在 RequestReceived 阶段生成审计事件。
    omitStages:
      - "RequestReceived"
```

可以使用最低限度的审计策略文件在 Metadata 级别记录所有请求：

```yaml
# 在 Metadata 级别为所有请求生成日志
apiVersion: audit.k8s.io/v1beta1
kind: Policy
rules:
- level: Metadata
```

可以参考 [Policy 配置参考](https://kubernetes.io/zh-cn/docs/reference/config-api/apiserver-audit.v1/#audit-k8s-io-v1-Policy) 以获取有关已定义字段的详细信息。

## 审计后端

审计后端实现将审计事件导出到外部存储。kube-apiserver 默认提供两个后端：

- Log 后端，将事件写入到文件系统
- Webhook 后端，将事件发送到外部 HTTP API

在这所有情况下，审计事件均遵循 Kubernetes API 在 audit.k8s.io API 组 中定义的结构。

### Log 后端

Log 后端将审计事件写入 JSONlines 格式的文件。 你可以使用以下 kube-apiserver 标志配置 Log 审计后端：

- --audit-log-path 指定用来写入审计事件的日志文件路径。不指定此标志会禁用日志后端。- 意味着标准化
- --audit-log-maxage 定义保留旧审计日志文件的最大天数
- --audit-log-maxbackup 定义要保留的审计日志文件的最大数量
- --audit-log-maxsize 定义审计日志文件轮转之前的最大大小（兆字节）

如果你的集群控制面以 Pod 的形式运行 kube-apiserver，记得要通过 hostPath 卷来访问策略文件和日志文件所在的目录，这样审计记录才会持久保存下来。例如：

```shell
  - --audit-policy-file=/etc/kubernetes/audit-policy.yaml
  - --audit-log-path=/var/log/kubernetes/audit/audit.log
```

接下来挂载数据卷：

```yaml
...
volumeMounts:
  - mountPath: /etc/kubernetes/audit-policy.yaml
    name: audit
    readOnly: true
  - mountPath: /var/log/kubernetes/audit/
    name: audit-log
    readOnly: false
```

最后配置 hostPath：

```yaml
...
volumes:
- name: audit
  hostPath:
    path: /etc/kubernetes/audit-policy.yaml
    type: File

- name: audit-log
  hostPath:
    path: /var/log/kubernetes/audit/
    type: DirectoryOrCreate
```

### Webhook 后端

Webhook 后端将审计事件发送到远程 Web API，该远程 API 应该暴露与 kube-apiserver 形式相同的 API，包括其身份认证机制。你可以使用如下 kube-apiserver 标志来配置 Webhook 审计后端：

- --audit-webhook-config-file 设置 Webhook 配置文件的路径。Webhook 配置文件实际上是一个 kubeconfig 文件。
- --audit-webhook-initial-backoff 指定在第一次失败后重发请求等待的时间。随后的请求将以指数退避重试。

Webhook 配置文件使用 kubeconfig 格式指定服务的远程地址和用于连接它的凭据。


## 源码阅读

- 入口位置 /cmd/kube-apiserver/app/server.go
- buildGenericConfig

```golang
lastErr = s.Audit.ApplyTo(genericConfig)
	if lastErr != nil {
		return
	}

```

### 1. 从配置 --audit-policy-file 加载 audit 策略

```golang
	// 1. Build policy evaluator
	evaluator, err := o.newPolicyRuleEvaluator()
	if err != nil {
		return err
	}
```

### 2. 从配置 --audit-log-path 设置 logBackend

```golang
	// 2. Build log backend
	var logBackend audit.Backend
	w, err := o.LogOptions.getWriter()
	if err != nil {
		return err
	}
	if w != nil {
		if evaluator == nil {
			klog.V(2).Info("No audit policy file provided, no events will be recorded for log backend")
		} else {
			logBackend = o.LogOptions.newBackend(w)
		}
	}

```
 
- 如果后端 --audit-log-path == "-" 表示输出到标准输出

```golang
func (o *AuditLogOptions) getWriter() (io.Writer, error) {
	if !o.enabled() {
		return nil, nil
	}

	if o.Path == "-" {
		return os.Stdout, nil
	}

	if err := o.ensureLogFile(); err != nil {
		return nil, fmt.Errorf("ensureLogFile: %w", err)
	}

	return &lumberjack.Logger{
		Filename:   o.Path,
		MaxAge:     o.MaxAge,
		MaxBackups: o.MaxBackups,
		MaxSize:    o.MaxSize,
		Compress:   o.Compress,
	}, nil
}
```

- lumberjack 日志库

后端使用日志库是 lumberjack   https://github.com/natefinch/lumberjack

Lumberjack is a Go package for writing logs to rolling files.

### 3. 根据配置构建 webhook 后端

```golang
	// 3. Build webhook backend
	var webhookBackend audit.Backend
	if o.WebhookOptions.enabled() {
		if evaluator == nil {
			klog.V(2).Info("No audit policy file provided, no events will be recorded for webhook backend")
		} else {
			if c.EgressSelector != nil {
				var egressDialer utilnet.DialFunc
				egressDialer, err = c.EgressSelector.Lookup(egressselector.ControlPlane.AsNetworkContext())
				if err != nil {
					return err
				}
				webhookBackend, err = o.WebhookOptions.newUntruncatedBackend(egressDialer)
			} else {
				webhookBackend, err = o.WebhookOptions.newUntruncatedBackend(nil)
			}
			if err != nil {
				return err
			}
		}
	}
```

### 4. 构建 dynamicBackend 

```golang
	// 4. Apply dynamic options.
	var dynamicBackend audit.Backend
	if webhookBackend != nil {
		// if only webhook is enabled wrap it in the truncate options
		dynamicBackend = o.WebhookOptions.TruncateOptions.wrapBackend(webhookBackend, groupVersion)
	}
```

### 5. 设置审计的策略计算对象 

```golang
	// 5. Set the policy rule evaluator
	c.AuditPolicyRuleEvaluator = evaluator
```

### 6. Union logBackend 和 dynamicBackend 

```golang
	// 6. Join the log backend with the webhooks
	c.AuditBackend = appendBackend(logBackend, dynamicBackend)
```

```golang
func appendBackend(existing, newBackend audit.Backend) audit.Backend {
	if existing == nil {
		return newBackend
	}
	if newBackend == nil {
		return existing
	}
	return audit.Union(existing, newBackend)
}
```

audit.Union 文件位置： /vendor/k8s.io/apiserver/pkg/audit/union.go

这里也是 组合+ 装饰器 设计模式。

### 7. 最终运行方法和入口

logBackend, webhook backend, 还有 union 后 全部实现了 backend 接口方法

- backend 接口方法位置 ：  /vendor/k8s.io/apiserver/pkg/audit/types.go

接口方法如下： 
```golang

type Sink interface {
	// ProcessEvents handles events. Per audit ID it might be that ProcessEvents is called up to three times.
	// Errors might be logged by the sink itself. If an error should be fatal, leading to an internal
	// error, ProcessEvents is supposed to panic. The event must not be mutated and is reused by the caller
	// after the call returns, i.e. the sink has to make a deepcopy to keep a copy around if necessary.
	// Returns true on success, may return false on error.
	ProcessEvents(events ...*auditinternal.Event) bool
}

type Backend interface {
	Sink

	// Run will initialize the backend. It must not block, but may run go routines in the background. If
	// stopCh is closed, it is supposed to stop them. Run will be called before the first call to ProcessEvents.
	Run(stopCh <-chan struct{}) error

	// Shutdown will synchronously shut down the backend while making sure that all pending
	// events are delivered. It can be assumed that this method is called after
	// the stopCh channel passed to the Run method has been closed.
	Shutdown()

	// Returns the backend PluginName.
	String() string
}
```

- 最终调用 audit 的 ProcessEvents 方法，以 logBackend 为例 

文件位置： /vendor/k8s.io/apiserver/plugin/pkg/audit/log/backend.go

```golang

func NewBackend(out io.Writer, format string, groupVersion schema.GroupVersion) audit.Backend {
	return &backend{
		out:     out,
		format:  format,
		encoder: audit.Codecs.LegacyCodec(groupVersion),
	}
}

func (b *backend) ProcessEvents(events ...*auditinternal.Event) bool {
	success := true
	for _, ev := range events {
		success = b.logEvent(ev) && success
	}
	return success
}

func (b *backend) logEvent(ev *auditinternal.Event) bool {
	line := ""
	switch b.format {
	case FormatLegacy:
		line = audit.EventString(ev) + "\n"
	case FormatJson:
		bs, err := runtime.Encode(b.encoder, ev)
		if err != nil {
			audit.HandlePluginError(PluginName, err, ev)
			return false
		}
		line = string(bs[:])
	default:
		audit.HandlePluginError(PluginName, fmt.Errorf("log format %q is not in list of known formats (%s)",
			b.format, strings.Join(AllowedFormats, ",")), ev)
		return false
	}
	if _, err := fmt.Fprint(b.out, line); err != nil {
		audit.HandlePluginError(PluginName, err, ev)
		return false
	}
	return true
}

```

根据 format 类型生成line, 然后调用 fmt.Fprint(b.out, line)， 向 b.out 输出 line 

### 8. http 侧调用 handler 

在哪里调用 backend 的 ProcessEvents ?

文件位置： /staging/src/k8s.io/apiserver/pkg/endpoints/filters/audit.go

```golang

// WithAudit decorates a http.Handler with audit logging information for all the
// requests coming to the server. Audit level is decided according to requests'
// attributes and audit policy. Logs are emitted to the audit sink to
// process events. If sink or audit policy is nil, no decoration takes place.
func WithAudit(handler http.Handler, sink audit.Sink, policy audit.PolicyRuleEvaluator, longRunningCheck request.LongRunningRequestCheck) http.Handler {
	if sink == nil || policy == nil {
		return handler
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ac, err := evaluatePolicyAndCreateAuditEvent(req, policy)
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("failed to create audit event: %v", err))
			responsewriters.InternalError(w, req, errors.New("failed to create audit event"))
			return
		}

		if ac == nil || ac.Event == nil {
			handler.ServeHTTP(w, req)
			return
		}
		ev := ac.Event

		ctx := req.Context()
		omitStages := ac.RequestAuditConfig.OmitStages

		ev.Stage = auditinternal.StageRequestReceived
		if processed := processAuditEvent(ctx, sink, ev, omitStages); !processed {
			audit.ApiserverAuditDroppedCounter.WithContext(ctx).Inc()
			responsewriters.InternalError(w, req, errors.New("failed to store audit event"))
			return
		}

		// intercept the status code
		var longRunningSink audit.Sink
		if longRunningCheck != nil {
			ri, _ := request.RequestInfoFrom(ctx)
			if longRunningCheck(req, ri) {
				longRunningSink = sink
			}
		}
		respWriter := decorateResponseWriter(ctx, w, ev, longRunningSink, omitStages)

		// send audit event when we leave this func, either via a panic or cleanly. In the case of long
		// running requests, this will be the second audit event.
		defer func() {
			if r := recover(); r != nil {
				defer panic(r)
				ev.Stage = auditinternal.StagePanic
				ev.ResponseStatus = &metav1.Status{
					Code:    http.StatusInternalServerError,
					Status:  metav1.StatusFailure,
					Reason:  metav1.StatusReasonInternalError,
					Message: fmt.Sprintf("APIServer panic'd: %v", r),
				}
				processAuditEvent(ctx, sink, ev, omitStages)
				return
			}

			// if no StageResponseStarted event was sent b/c neither a status code nor a body was sent, fake it here
			// But Audit-Id http header will only be sent when http.ResponseWriter.WriteHeader is called.
			fakedSuccessStatus := &metav1.Status{
				Code:    http.StatusOK,
				Status:  metav1.StatusSuccess,
				Message: "Connection closed early",
			}
			if ev.ResponseStatus == nil && longRunningSink != nil {
				ev.ResponseStatus = fakedSuccessStatus
				ev.Stage = auditinternal.StageResponseStarted
				processAuditEvent(ctx, longRunningSink, ev, omitStages)
			}

			ev.Stage = auditinternal.StageResponseComplete
			if ev.ResponseStatus == nil {
				ev.ResponseStatus = fakedSuccessStatus
			}
			processAuditEvent(ctx, sink, ev, omitStages)
		}()
		handler.ServeHTTP(respWriter, req)
	})
}

```

用http.Handler 包装 audit ProcessEvents， 所有经过 api server请求都会被 audit 处理。






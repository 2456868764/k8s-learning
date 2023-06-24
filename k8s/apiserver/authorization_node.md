# Node 类型 Authorization

文档位置： https://kubernetes.io/zh-cn/docs/reference/access-authn-authz/node/

节点鉴权是一种特殊用途的鉴权模式，专门对 kubelet 发出的 API 请求进行授权。

四种规则
- 如果不是 node 的请求则拒绝
- 如果 nodeName 没有找到则拒绝
- 如果请求是 configmap, secret, pod, pv, pvc 需要校验
  - 如果动作非 Get, 拒绝
  - 如果请求的资源和节点没有关系则拒绝
- 如果请求其他资源，需要按照预先定义好 Rule 匹配


## 概述

节点鉴权器允许 kubelet 执行 API 操作。包括：

读取操作：
- services
- endpoints
- nodes
- pods
- 与绑定到 kubelet 节点的 Pod 相关的 Secret、ConfigMap、PersistentVolumeClaim 和持久卷

写入操作：
- 节点和节点状态（启用 NodeRestriction 准入插件以限制 kubelet 只能修改自己的节点）
- Pod 和 Pod 状态 (启用 NodeRestriction 准入插件以限制 kubelet 只能修改绑定到自身的 Pod)事件

身份认证与鉴权相关的操作：

- 对于基于 TLS 的启动引导过程时使用的 certificationsigningrequests API 的读/写权限
- 为委派的身份验证/鉴权检查创建 TokenReview 和 SubjectAccessReview 的能力


为了获得节点鉴权器的授权，kubelet 必须使用一个凭证以表示它在 system:nodes 组中，用户名为 system:node:<nodeName>。上述的组名和用户名格式要与 kubelet TLS 启动引导 过程中为每个 kubelet 创建的标识相匹配。

<nodeName> 的值必须与 kubelet 注册的节点名称精确匹配。默认情况下，节点名称是由 hostname 提供的主机名，或者通过 kubelet --hostname-override 选项 覆盖。 但是，当使用 --cloud-provider kubelet 选项时，具体的主机名可能由云提供商确定， 忽略本地的 hostname 和 --hostname-override 选项。有关 kubelet 如何确定主机名的详细信息，请参阅 kubelet 选项参考。

要启用节点鉴权器，请使用 --authorization-mode=Node 启动 API 服务器。

要限制 kubelet 可以写入的 API 对象，请使用 --enable-admission-plugins=...,NodeRestriction,... 启动 API 服务器，从而启用 NodeRestriction 准入插件。

## 源码分析
文件位置： /plugin/pkg/auth/authorizer/node/node_authorizer.go

```golang

func (r *NodeAuthorizer) Authorize(ctx context.Context, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
	nodeName, isNode := r.identifier.NodeIdentity(attrs.GetUser())
	if !isNode {
		// reject requests from non-nodes
		return authorizer.DecisionNoOpinion, "", nil
	}
	if len(nodeName) == 0 {
		// reject requests from unidentifiable nodes
		klog.V(2).Infof("NODE DENY: unknown node for user %q", attrs.GetUser().GetName())
		return authorizer.DecisionNoOpinion, fmt.Sprintf("unknown node for user %q", attrs.GetUser().GetName()), nil
	}

	// subdivide access to specific resources
	if attrs.IsResourceRequest() {
		requestResource := schema.GroupResource{Group: attrs.GetAPIGroup(), Resource: attrs.GetResource()}
		switch requestResource {
		case secretResource:
			return r.authorizeReadNamespacedObject(nodeName, secretVertexType, attrs)
		case configMapResource:
			return r.authorizeReadNamespacedObject(nodeName, configMapVertexType, attrs)
		case pvcResource:
			if attrs.GetSubresource() == "status" {
				return r.authorizeStatusUpdate(nodeName, pvcVertexType, attrs)
			}
			return r.authorizeGet(nodeName, pvcVertexType, attrs)
		case pvResource:
			return r.authorizeGet(nodeName, pvVertexType, attrs)
		case vaResource:
			return r.authorizeGet(nodeName, vaVertexType, attrs)
		case svcAcctResource:
			return r.authorizeCreateToken(nodeName, serviceAccountVertexType, attrs)
		case leaseResource:
			return r.authorizeLease(nodeName, attrs)
		case csiNodeResource:
			return r.authorizeCSINode(nodeName, attrs)
		}

	}

	// Access to other resources is not subdivided, so just evaluate against the statically defined node rules
	if rbac.RulesAllow(attrs, r.nodeRules...) {
		return authorizer.DecisionAllow, "", nil
	}
	return authorizer.DecisionNoOpinion, "", nil
}

```

### NodeIdentity() 判断是否 Node 请求 

文件位置：/pkg/auth/nodeidentifier/default.go

```golang

// NewDefaultNodeIdentifier returns a default NodeIdentifier implementation,
// which returns isNode=true if the user groups contain the system:nodes group
// and the user name matches the format system:node:<nodeName>, and populates
// nodeName if isNode is true
func NewDefaultNodeIdentifier() NodeIdentifier {
	return defaultNodeIdentifier{}
}

// defaultNodeIdentifier implements NodeIdentifier
type defaultNodeIdentifier struct{}

// nodeUserNamePrefix is the prefix for usernames in the form `system:node:<nodeName>`
const nodeUserNamePrefix = "system:node:"

// NodeIdentity returns isNode=true if the user groups contain the system:nodes
// group and the user name matches the format system:node:<nodeName>, and
// populates nodeName if isNode is true
func (defaultNodeIdentifier) NodeIdentity(u user.Info) (string, bool) {
	// Make sure we're a node, and can parse the node name
	if u == nil {
		return "", false
	}

	userName := u.GetName()
	if !strings.HasPrefix(userName, nodeUserNamePrefix) {
		return "", false
	}

	isNode := false
	for _, g := range u.GetGroups() {
		if g == user.NodesGroup {
			isNode = true
			break
		}
	}
	if !isNode {
		return "", false
	}

	nodeName := strings.TrimPrefix(userName, nodeUserNamePrefix)
	return nodeName, true
}
```

核心逻辑：
- 用户 Name 是否以 system:node: 开头
- 用户是否在 system:nodes 组中
- 返回 nodeName

### 规则分析

1. 规则
```shell
// NodeAuthorizer authorizes requests from kubelets, with the following logic:
//  1. If a request is not from a node (NodeIdentity() returns isNode=false), reject
//  2. If a specific node cannot be identified (NodeIdentity() returns nodeName=""), reject
//  3. If a request is for a secret, configmap, persistent volume or persistent volume claim, reject unless the verb is get, and the requested object is related to the requesting node:
//     node <- configmap
//     node <- pod
//     node <- pod <- secret
//     node <- pod <- configmap
//     node <- pod <- pvc
//     node <- pod <- pvc <- pv
//     node <- pod <- pvc <- pv <- secret
//  4. For other resources, authorize all nodes uniformly using statically defined rules
```
第一条和第二条规则很容易理解

2. 规则 3 分析
- 如果请求的资源是 secret, configmap , pvc, pv, 需要验证动作是否 GET
- 以 secretResource为例， 调用 authorizeReadNamespacedObject 方法

```golang
		case secretResource:
            return r.authorizeReadNamespacedObject(nodeName, secretVertexType, attrs)

```

3. authorizeReadNamespacedObject 验证 namespace 方法



```golang
// authorizeReadNamespacedObject authorizes "get", "list" and "watch" requests to single objects of a
// specified types if they are related to the specified node.
func (r *NodeAuthorizer) authorizeReadNamespacedObject(nodeName string, startingType vertexType, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
	switch attrs.GetVerb() {
	case "get", "list", "watch":
		//ok
	default:
		klog.V(2).Infof("NODE DENY: '%s' %#v", nodeName, attrs)
		return authorizer.DecisionNoOpinion, "can only read resources of this type", nil
	}

	if len(attrs.GetSubresource()) > 0 {
		klog.V(2).Infof("NODE DENY: '%s' %#v", nodeName, attrs)
		return authorizer.DecisionNoOpinion, "cannot read subresource", nil
	}
	if len(attrs.GetNamespace()) == 0 {
		klog.V(2).Infof("NODE DENY: '%s' %#v", nodeName, attrs)
		return authorizer.DecisionNoOpinion, "can only read namespaced object of this type", nil
	}
	return r.authorize(nodeName, startingType, attrs)
}
```
核心逻辑：
- 方法是 非GET, List, watch, 拒绝
- 子资源拒绝
- 非 NameSpace 请求拒绝
- 调用底层 authorize 方法继续

4. node 底层 authorize 方法


```golang
func (r *NodeAuthorizer) authorize(nodeName string, startingType vertexType, attrs authorizer.Attributes) (authorizer.Decision, string, error) {
	if len(attrs.GetName()) == 0 {
		klog.V(2).Infof("NODE DENY: '%s' %#v", nodeName, attrs)
		return authorizer.DecisionNoOpinion, "No Object name found", nil
	}

	ok, err := r.hasPathFrom(nodeName, startingType, attrs.GetNamespace(), attrs.GetName())
	if err != nil {
		klog.V(2).InfoS("NODE DENY", "err", err)
		return authorizer.DecisionNoOpinion, fmt.Sprintf("no relationship found between node '%s' and this object", nodeName), nil
	}
	if !ok {
		klog.V(2).Infof("NODE DENY: '%s' %#v", nodeName, attrs)
		return authorizer.DecisionNoOpinion, fmt.Sprintf("no relationship found between node '%s' and this object", nodeName), nil
	}
	return authorizer.DecisionAllow, "", nil
}
```
核心逻辑：
- nodeName 为空, 拒绝
- 不是这个 node 下的资源，拒绝

5. 其他资源 rbac.RulesAllow 

文件位置： /plugin/pkg/auth/authorizer/rbac/rbac.go

```golang

func RulesAllow(requestAttributes authorizer.Attributes, rules ...rbacv1.PolicyRule) bool {
	for i := range rules {
		if RuleAllows(requestAttributes, &rules[i]) {
			return true
		}
	}

	return false
}

func RuleAllows(requestAttributes authorizer.Attributes, rule *rbacv1.PolicyRule) bool {
	if requestAttributes.IsResourceRequest() {
		combinedResource := requestAttributes.GetResource()
		if len(requestAttributes.GetSubresource()) > 0 {
			combinedResource = requestAttributes.GetResource() + "/" + requestAttributes.GetSubresource()
		}

		return rbacv1helpers.VerbMatches(rule, requestAttributes.GetVerb()) &&
			rbacv1helpers.APIGroupMatches(rule, requestAttributes.GetAPIGroup()) &&
			rbacv1helpers.ResourceMatches(rule, combinedResource, requestAttributes.GetSubresource()) &&
			rbacv1helpers.ResourceNameMatches(rule, requestAttributes.GetName())
	}

	return rbacv1helpers.VerbMatches(rule, requestAttributes.GetVerb()) &&
		rbacv1helpers.NonResourceURLMatches(rule, requestAttributes.GetPath())
}


```

对每一个原先定义的 Rule 遍历，同时检查是否通过。

rules 在初始化位置传入：
```goalang
nodeAuthorizer := node.NewAuthorizer(graph, nodeidentifier.NewDefaultNodeIdentifier(), bootstrappolicy.NodeRules())
```
这些规则定义位置：bootstrappolicy.NodeRules()， 具体规则

文件位置：/plugin/pkg/auth/authorizer/rbac/bootstrappolicy/policy.go

```golang
// NodeRules returns node policy rules, it is slice of rbacv1.PolicyRule.
func NodeRules() []rbacv1.PolicyRule {
	nodePolicyRules := []rbacv1.PolicyRule{
		// Needed to check API access.  These creates are non-mutating
		rbacv1helpers.NewRule("create").Groups(authenticationGroup).Resources("tokenreviews").RuleOrDie(),
		rbacv1helpers.NewRule("create").Groups(authorizationGroup).Resources("subjectaccessreviews", "localsubjectaccessreviews").RuleOrDie(),

		// Needed to build serviceLister, to populate env vars for services
		rbacv1helpers.NewRule(Read...).Groups(legacyGroup).Resources("services").RuleOrDie(),

		// Nodes can register Node API objects and report status.
		// Use the NodeRestriction admission plugin to limit a node to creating/updating its own API object.
		rbacv1helpers.NewRule("create", "get", "list", "watch").Groups(legacyGroup).Resources("nodes").RuleOrDie(),
		rbacv1helpers.NewRule("update", "patch").Groups(legacyGroup).Resources("nodes/status").RuleOrDie(),
		rbacv1helpers.NewRule("update", "patch").Groups(legacyGroup).Resources("nodes").RuleOrDie(),

		// TODO: restrict to the bound node as creator in the NodeRestrictions admission plugin
		rbacv1helpers.NewRule("create", "update", "patch").Groups(legacyGroup).Resources("events").RuleOrDie(),

		// TODO: restrict to pods scheduled on the bound node once field selectors are supported by list/watch authorization
		rbacv1helpers.NewRule(Read...).Groups(legacyGroup).Resources("pods").RuleOrDie(),

		// Needed for the node to create/delete mirror pods.
		// Use the NodeRestriction admission plugin to limit a node to creating/deleting mirror pods bound to itself.
		rbacv1helpers.NewRule("create", "delete").Groups(legacyGroup).Resources("pods").RuleOrDie(),
		// Needed for the node to report status of pods it is running.
		// Use the NodeRestriction admission plugin to limit a node to updating status of pods bound to itself.
		rbacv1helpers.NewRule("update", "patch").Groups(legacyGroup).Resources("pods/status").RuleOrDie(),
		// Needed for the node to create pod evictions.
		// Use the NodeRestriction admission plugin to limit a node to creating evictions for pods bound to itself.
		rbacv1helpers.NewRule("create").Groups(legacyGroup).Resources("pods/eviction").RuleOrDie(),

		// Needed for imagepullsecrets, rbd/ceph and secret volumes, and secrets in envs
		// Needed for configmap volume and envs
		// Use the Node authorization mode to limit a node to get secrets/configmaps referenced by pods bound to itself.
		rbacv1helpers.NewRule("get", "list", "watch").Groups(legacyGroup).Resources("secrets", "configmaps").RuleOrDie(),
		// Needed for persistent volumes
		// Use the Node authorization mode to limit a node to get pv/pvc objects referenced by pods bound to itself.
		rbacv1helpers.NewRule("get").Groups(legacyGroup).Resources("persistentvolumeclaims", "persistentvolumes").RuleOrDie(),

		// TODO: add to the Node authorizer and restrict to endpoints referenced by pods or PVs bound to the node
		// Needed for glusterfs volumes
		rbacv1helpers.NewRule("get").Groups(legacyGroup).Resources("endpoints").RuleOrDie(),
		// Used to create a certificatesigningrequest for a node-specific client certificate, and watch
		// for it to be signed. This allows the kubelet to rotate it's own certificate.
		rbacv1helpers.NewRule("create", "get", "list", "watch").Groups(certificatesGroup).Resources("certificatesigningrequests").RuleOrDie(),

		// Leases
		rbacv1helpers.NewRule("get", "create", "update", "patch", "delete").Groups("coordination.k8s.io").Resources("leases").RuleOrDie(),

		// CSI
		rbacv1helpers.NewRule("get").Groups(storageGroup).Resources("volumeattachments").RuleOrDie(),

		// Use the Node authorization to limit a node to create tokens for service accounts running on that node
		// Use the NodeRestriction admission plugin to limit a node to create tokens bound to pods on that node
		rbacv1helpers.NewRule("create").Groups(legacyGroup).Resources("serviceaccounts/token").RuleOrDie(),
	}

	// Use the Node authorization mode to limit a node to update status of pvc objects referenced by pods bound to itself.
	// Use the NodeRestriction admission plugin to limit a node to just update the status stanza.
	pvcStatusPolicyRule := rbacv1helpers.NewRule("get", "update", "patch").Groups(legacyGroup).Resources("persistentvolumeclaims/status").RuleOrDie()
	nodePolicyRules = append(nodePolicyRules, pvcStatusPolicyRule)

	// CSI
	csiDriverRule := rbacv1helpers.NewRule("get", "watch", "list").Groups("storage.k8s.io").Resources("csidrivers").RuleOrDie()
	nodePolicyRules = append(nodePolicyRules, csiDriverRule)
	csiNodeInfoRule := rbacv1helpers.NewRule("get", "create", "update", "patch", "delete").Groups("storage.k8s.io").Resources("csinodes").RuleOrDie()
	nodePolicyRules = append(nodePolicyRules, csiNodeInfoRule)

	// RuntimeClass
	nodePolicyRules = append(nodePolicyRules, rbacv1helpers.NewRule("get", "list", "watch").Groups("node.k8s.io").Resources("runtimeclasses").RuleOrDie())

	// DRA Resource Claims
	if utilfeature.DefaultFeatureGate.Enabled(features.DynamicResourceAllocation) {
		nodePolicyRules = append(nodePolicyRules, rbacv1helpers.NewRule("get").Groups(resourceGroup).Resources("resourceclaims").RuleOrDie())
	}
	// Kubelet needs access to ClusterTrustBundles to support the pemTrustAnchors volume type.
	if utilfeature.DefaultFeatureGate.Enabled(features.ClusterTrustBundle) {
		nodePolicyRules = append(nodePolicyRules, rbacv1helpers.NewRule("get", "list", "watch").Groups(certificatesGroup).Resources("clustertrustbundles").RuleOrDie())
	}

	return nodePolicyRules
}


```
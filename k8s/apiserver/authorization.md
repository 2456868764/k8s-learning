# Authorization 鉴权概述

文档地址： https://kubernetes.io/zh-cn/docs/reference/access-authn-authz/authorization/


## 确定是允许还是拒绝请求

Kubernetes 使用 API 服务器对 API 请求进行鉴权。 它根据所有策略评估所有请求属性来决定允许或拒绝请求。 一个 API 请求的所有部分都必须被某些策略允许才能继续。 这意味着默认情况下拒绝权限。

Kubernetes 仅审查以下 API 请求属性

- 用户 —— 身份验证期间提供的 user 字符串。
- 组 —— 经过身份验证的用户所属的组名列表。
- 额外信息 —— 由身份验证层提供的任意字符串键到字符串值的映射。
- API —— 指示请求是否针对 API 资源。
- 请求路径 —— 各种非资源端点的路径，如 /api 或 /healthz。
- API 请求动词 —— API 动词 get、list、create、update、patch、watch、 proxy、redirect、delete 和 deletecollection 用于资源请求。 要确定资源 API 端点的请求动词，请参阅确定请求动词。
- HTTP 请求动词 —— HTTP 动词 get、post、put 和 delete 用于非资源请求。
- 资源 —— 正在访问的资源的 ID 或名称（仅限资源请求）- 对于使用 get、update、patch 和 delete 动词的资源请求，你必须提供资源名称。
- 子资源 —— 正在访问的子资源（仅限资源请求）。
- 名字空间 —— 正在访问的对象的名称空间（仅适用于名字空间资源请求）。
- API 组 —— 正在访问的 API 组 （仅限资源请求）。空字符串表示核心 API 组。

请求动词
- POST
- GET
- DELETE
- PUT
- LIST
- WATCH

请求类型
- 非资源请求

对于 /api/v1/... 或 /apis/<group>/<version>/... 之外的端点的请求被视为 “非资源请求（Non-Resource Requests）”， 并使用该请求的 HTTP 方法的小写形式作为其请求动词。
例如，对 /api 或 /healthz 这类端点的 GET 请求将使用 get 作为其动词。

- 资源请求

对于 /api/v1/... 或 /apis/<group>/<version>/ 端点的请求视为 “资源请求（Resource Requests）”

## 鉴权模块

1. 四种鉴权模块
- Node —— 一个专用鉴权模式，根据调度到 kubelet 上运行的 Pod 为 kubelet 授予权限。 要了解有关使用节点鉴权模式的更多信息，请参阅[节点鉴权](https://kubernetes.io/zh-cn/docs/reference/access-authn-authz/node/)。
- ABAC —— 基于属性的访问控制（[ABAC](https://kubernetes.io/zh-cn/docs/reference/access-authn-authz/abac/)）定义了一种访问控制范型，通过使用将属性组合在一起的策略， 将访问权限授予用户。策略可以使用任何类型的属性（用户属性、资源属性、对象，环境属性等）。 要了解有关使用 ABAC 模式的更多信息，请参阅 ABAC 模式。
- RBAC —— 基于角色的访问控制（[RBAC](https://kubernetes.io/zh-cn/docs/reference/access-authn-authz/rbac/)） 是一种基于企业内个人用户的角色来管理对计算机或网络资源的访问的方法。 在此上下文中，权限是单个用户执行特定任务的能力， 例如查看、创建或修改文件。要了解有关使用 RBAC 模式的更多信息，请参阅 RBAC 模式。
   - 被启用之后，RBAC（基于角色的访问控制）使用 rbac.authorization.k8s.io API 组来驱动鉴权决策，从而允许管理员通过 Kubernetes API 动态配置权限策略。
   - 要启用 RBAC，请使用 --authorization-mode = RBAC 启动 API 服务器。
- Webhook —— WebHook 是一个 HTTP 回调：发生某些事情时调用的 HTTP POST； 通过 HTTP POST 进行简单的事件通知。 实现 WebHook 的 Web 应用程序会在发生某些事情时将消息发布到 URL。 要了解有关使用 Webhook 模式的更多信息，请参阅 [Webhook 模式](https://kubernetes.io/zh-cn/docs/reference/access-authn-authz/webhook/)。

2. 鉴权执行链 union AuthzHandler

## 鉴权模块设置参数

- --authorization-mode=ABAC 基于属性的访问控制（ABAC）模式允许你使用本地文件配置策略。
- --authorization-mode=RBAC 基于角色的访问控制（RBAC）模式允许你使用 Kubernetes API 创建和存储策略。
- --authorization-mode=Webhook WebHook 是一种 HTTP 回调模式，允许你使用远程 REST 端点管理鉴权。
- --authorization-mode=Node 节点鉴权是一种特殊用途的鉴权模式，专门对 kubelet 发出的 API 请求执行鉴权。
- --authorization-mode=AlwaysDeny 该标志阻止所有请求。仅将此标志用于测试。
- --authorization-mode=AlwaysAllow 此标志允许所有请求。仅在你不需要 API 请求的鉴权时才使用此标志。

可以选择多个鉴权模块。模块按顺序检查，以便较靠前的模块具有更高的优先级来允许或拒绝请求。

## 代码解析

代码入口位置： /cmd/kube-apiserver/app/server.go 

```golang
// 授权
	genericConfig.Authorization.Authorizer, genericConfig.RuleResolver, err = BuildAuthorizer(s, genericConfig.EgressSelector, versionedInformers)
```

通过 authorizationConfig.New() 构建，入口文件位置： /pkg/kubeapiserver/authorizer/config.go

### authorizationConfig.New() 分析

```golang

// New returns the right sort of union of multiple authorizer.Authorizer objects
// based on the authorizationMode or an error.
func (config Config) New() (authorizer.Authorizer, authorizer.RuleResolver, error) {
	if len(config.AuthorizationModes) == 0 {
		return nil, nil, fmt.Errorf("at least one authorization mode must be passed")
	}

	var (
		authorizers   []authorizer.Authorizer
		ruleResolvers []authorizer.RuleResolver
	)

	// Add SystemPrivilegedGroup as an authorizing group
	superuserAuthorizer := authorizerfactory.NewPrivilegedGroups(user.SystemPrivilegedGroup)
	authorizers = append(authorizers, superuserAuthorizer)

	for _, authorizationMode := range config.AuthorizationModes {
		// Keep cases in sync with constant list in k8s.io/kubernetes/pkg/kubeapiserver/authorizer/modes/modes.go.
		switch authorizationMode {
		case modes.ModeNode:
			node.RegisterMetrics()
			graph := node.NewGraph()
			node.AddGraphEventHandlers(
				graph,
				config.VersionedInformerFactory.Core().V1().Nodes(),
				config.VersionedInformerFactory.Core().V1().Pods(),
				config.VersionedInformerFactory.Core().V1().PersistentVolumes(),
				config.VersionedInformerFactory.Storage().V1().VolumeAttachments(),
			)
			nodeAuthorizer := node.NewAuthorizer(graph, nodeidentifier.NewDefaultNodeIdentifier(), bootstrappolicy.NodeRules())
			authorizers = append(authorizers, nodeAuthorizer)
			ruleResolvers = append(ruleResolvers, nodeAuthorizer)

		case modes.ModeAlwaysAllow:
			alwaysAllowAuthorizer := authorizerfactory.NewAlwaysAllowAuthorizer()
			authorizers = append(authorizers, alwaysAllowAuthorizer)
			ruleResolvers = append(ruleResolvers, alwaysAllowAuthorizer)
		case modes.ModeAlwaysDeny:
			alwaysDenyAuthorizer := authorizerfactory.NewAlwaysDenyAuthorizer()
			authorizers = append(authorizers, alwaysDenyAuthorizer)
			ruleResolvers = append(ruleResolvers, alwaysDenyAuthorizer)
		case modes.ModeABAC:
			abacAuthorizer, err := abac.NewFromFile(config.PolicyFile)
			if err != nil {
				return nil, nil, err
			}
			authorizers = append(authorizers, abacAuthorizer)
			ruleResolvers = append(ruleResolvers, abacAuthorizer)
		case modes.ModeWebhook:
			if config.WebhookRetryBackoff == nil {
				return nil, nil, errors.New("retry backoff parameters for authorization webhook has not been specified")
			}
			clientConfig, err := webhookutil.LoadKubeconfig(config.WebhookConfigFile, config.CustomDial)
			if err != nil {
				return nil, nil, err
			}
			webhookAuthorizer, err := webhook.New(clientConfig,
				config.WebhookVersion,
				config.WebhookCacheAuthorizedTTL,
				config.WebhookCacheUnauthorizedTTL,
				*config.WebhookRetryBackoff,
			)
			if err != nil {
				return nil, nil, err
			}
			authorizers = append(authorizers, webhookAuthorizer)
			ruleResolvers = append(ruleResolvers, webhookAuthorizer)
		case modes.ModeRBAC:
			rbacAuthorizer := rbac.New(
				&rbac.RoleGetter{Lister: config.VersionedInformerFactory.Rbac().V1().Roles().Lister()},
				&rbac.RoleBindingLister{Lister: config.VersionedInformerFactory.Rbac().V1().RoleBindings().Lister()},
				&rbac.ClusterRoleGetter{Lister: config.VersionedInformerFactory.Rbac().V1().ClusterRoles().Lister()},
				&rbac.ClusterRoleBindingLister{Lister: config.VersionedInformerFactory.Rbac().V1().ClusterRoleBindings().Lister()},
			)
			authorizers = append(authorizers, rbacAuthorizer)
			ruleResolvers = append(ruleResolvers, rbacAuthorizer)
		default:
			return nil, nil, fmt.Errorf("unknown authorization mode %s specified", authorizationMode)
		}
	}

	return union.New(authorizers...), union.NewRuleResolvers(ruleResolvers...), nil
}

```

核心逻辑如下：

- 根据 config.AuthorizationModes 激活认证插件类型，初始化对应认证插件，同时插件添加到 authorizers, ruleResolvers 列表中
- 通过 Union authorizers 和 ruleResolvers 包装一下返回（ 这里 组合 + 装饰器模式）



两个核心变量
- authorizers   []authorizer.Authorizer
- ruleResolvers []authorizer.RuleResolver


Authorizer接口定义

文件位置： /ve ndor/k8s.io/apiserver/pkg/authorization/authorizer/interfaces.go

```golang

// Authorizer makes an authorization decision based on information gained by making
// zero or more calls to methods of the Attributes interface.  It returns nil when an action is
// authorized, otherwise it returns an error.
type Authorizer interface {
	Authorize(ctx context.Context, a Attributes) (authorized Decision, reason string, err error)
}

```
Decision定义：
```golang
type Decision int

const (
	// DecisionDeny means that an authorizer decided to deny the action.
	DecisionDeny Decision = iota
	// DecisionAllow means that an authorizer decided to allow the action.
	DecisionAllow
	// DecisionNoOpionion means that an authorizer has no opinion on whether
	// to allow or deny an action.
	DecisionNoOpinion
)
```
- DecisionDeny ：表示拒绝
- DecisionAllow： 表示通过
- DecisionNoOpinion： 表示 未表态

属性定义

```golang

// Attributes is an interface used by an Authorizer to get information about a request
// that is used to make an authorization decision.
type Attributes interface {
	// GetUser returns the user.Info object to authorize
	GetUser() user.Info

	// GetVerb returns the kube verb associated with API requests (this includes get, list, watch, create, update, patch, delete, deletecollection, and proxy),
	// or the lowercased HTTP verb associated with non-API requests (this includes get, put, post, patch, and delete)
	GetVerb() string

	// When IsReadOnly() == true, the request has no side effects, other than
	// caching, logging, and other incidentals.
	IsReadOnly() bool

	// The namespace of the object, if a request is for a REST object.
	GetNamespace() string

	// The kind of object, if a request is for a REST object.
	GetResource() string

	// GetSubresource returns the subresource being requested, if present
	GetSubresource() string

	// GetName returns the name of the object as parsed off the request.  This will not be present for all request types, but
	// will be present for: get, update, delete
	GetName() string

	// The group of the resource, if a request is for a REST object.
	GetAPIGroup() string

	// GetAPIVersion returns the version of the group requested, if a request is for a REST object.
	GetAPIVersion() string

	// IsResourceRequest returns true for requests to API resources, like /api/v1/nodes,
	// and false for non-resource endpoints like /api, /healthz
	IsResourceRequest() bool

	// GetPath returns the path of the request
	GetPath() string
}
```


RuleResolver接口定义：

文件位置： /vendor/k8s.io/apiserver/pkg/authorization/authorizer/interfaces.go

```golang

// RuleResolver provides a mechanism for resolving the list of rules that apply to a given user within a namespace.
type RuleResolver interface {
	// RulesFor get the list of cluster wide rules, the list of rules in the specific namespace, incomplete status and errors.
	RulesFor(user user.Info, namespace string) ([]ResourceRuleInfo, []NonResourceRuleInfo, bool, error)
}
```

用户信息定义：

文件位置： /vendor/k8s.io/apiserver/pkg/authentication/user/user.go
```golang

// Info describes a user that has been authenticated to the system.
type Info interface {
	// GetName returns the name that uniquely identifies this user among all
	// other active users.
	GetName() string
	// GetUID returns a unique value for a particular user that will change
	// if the user is removed from the system and another user is added with
	// the same name.
	GetUID() string
	// GetGroups returns the names of the groups the user is a member of
	GetGroups() []string

	// GetExtra can contain any additional information that the authenticator
	// thought was interesting.  One example would be scopes on a token.
	// Keys in this map should be namespaced to the authenticator or
	// authenticator/authorizer pair making use of them.
	// For instance: "example.org/foo" instead of "foo"
	// This is a map[string][]string because it needs to be serializeable into
	// a SubjectAccessReviewSpec.authorization.k8s.io for proper authorization
	// delegation flows
	// In order to faithfully round-trip through an impersonation flow, these keys
	// MUST be lowercase.
	GetExtra() map[string][]string
}
```
ResourceRuleInfo 和 NonResourceRuleInfo 定义：

文件位置： /vendor/k8s.io/apiserver/pkg/authorization/authorizer/rule.go

```golang
type ResourceRuleInfo interface {
	// GetVerbs returns a list of kubernetes resource API verbs.
	GetVerbs() []string
	// GetAPIGroups return the names of the APIGroup that contains the resources.
	GetAPIGroups() []string
	// GetResources return a list of resources the rule applies to.
	GetResources() []string
	// GetResourceNames return a white list of names that the rule applies to.
	GetResourceNames() []string
}

type NonResourceRuleInfo interface {
    // GetVerbs returns a list of kubernetes resource API verbs.
    GetVerbs() []string
    // GetNonResourceURLs return a set of partial urls that a user should have access to.
    GetNonResourceURLs() []string
}
```

union.New(authorizers...) 分析：

文件位置： /vendor/k8s.io/apiserver/pkg/authorization/union/union.go

```golang

// unionAuthzHandler authorizer against a chain of authorizer.Authorizer
type unionAuthzHandler []authorizer.Authorizer

// New returns an authorizer that authorizes against a chain of authorizer.Authorizer objects
func New(authorizationHandlers ...authorizer.Authorizer) authorizer.Authorizer {
	return unionAuthzHandler(authorizationHandlers)
}

// Authorizes against a chain of authorizer.Authorizer objects and returns nil if successful and returns error if unsuccessful
func (authzHandler unionAuthzHandler) Authorize(ctx context.Context, a authorizer.Attributes) (authorizer.Decision, string, error) {
	var (
		errlist    []error
		reasonlist []string
	)

	for _, currAuthzHandler := range authzHandler {
		decision, reason, err := currAuthzHandler.Authorize(ctx, a)

		if err != nil {
			errlist = append(errlist, err)
		}
		if len(reason) != 0 {
			reasonlist = append(reasonlist, reason)
		}
		switch decision {
		case authorizer.DecisionAllow, authorizer.DecisionDeny:
			return decision, reason, err
		case authorizer.DecisionNoOpinion:
			// continue to the next authorizer
		}
	}

	return authorizer.DecisionNoOpinion, strings.Join(reasonlist, "\n"), utilerrors.NewAggregate(errlist)
}


```
union执行逻辑：
- unionAuthzHandler的鉴权执行方法 Authorize 同样遍历内部 authzHandler 执行 Authorize 方法
- 如果任何一个鉴权插件返回结果 decision 为通过或者拒绝，就直接返回
- 否则代表不表态，继续执行下个 Authorize 方法
- 如果都不表态，默认返回不表态


union.NewRuleResolvers(ruleResolvers...)分析：

文件位置： /vendor/k8s.io/apiserver/pkg/authorization/union/union.go

```golang

// unionAuthzRulesHandler authorizer against a chain of authorizer.RuleResolver
type unionAuthzRulesHandler []authorizer.RuleResolver

// NewRuleResolvers returns an authorizer that authorizes against a chain of authorizer.Authorizer objects
func NewRuleResolvers(authorizationHandlers ...authorizer.RuleResolver) authorizer.RuleResolver {
	return unionAuthzRulesHandler(authorizationHandlers)
}

// RulesFor against a chain of authorizer.RuleResolver objects and returns nil if successful and returns error if unsuccessful
func (authzHandler unionAuthzRulesHandler) RulesFor(user user.Info, namespace string) ([]authorizer.ResourceRuleInfo, []authorizer.NonResourceRuleInfo, bool, error) {
	var (
		errList              []error
		resourceRulesList    []authorizer.ResourceRuleInfo
		nonResourceRulesList []authorizer.NonResourceRuleInfo
	)
	incompleteStatus := false

	for _, currAuthzHandler := range authzHandler {
		resourceRules, nonResourceRules, incomplete, err := currAuthzHandler.RulesFor(user, namespace)

		if incomplete {
			incompleteStatus = true
		}
		if err != nil {
			errList = append(errList, err)
		}
		if len(resourceRules) > 0 {
			resourceRulesList = append(resourceRulesList, resourceRules...)
		}
		if len(nonResourceRules) > 0 {
			nonResourceRulesList = append(nonResourceRulesList, nonResourceRules...)
		}
	}

	return resourceRulesList, nonResourceRulesList, incompleteStatus, utilerrors.NewAggregate(errList)
}

```
union执行逻辑：
- unionAuthzRulesHandler的执行方法 RulesFor 中遍历内部的 authzHandler
- 执行他们的 RulesFor 方法获取 resourceRules, nonResourceRules
- 并将结果添加到 resourceRulesList， nonResourceRulesList 中，同时返回

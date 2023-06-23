# Authentication 认证

## 认证支持多个插件认证模式
- Password
- Plan Token
- Jwt Token
- Bootstrap Token
- Client Certificate
- Webhook
- OpenID Connect（OIDC）令牌

https://kubernetes.io/zh-cn/docs/reference/access-authn-authz/authentication/

## Union 统一认证规则
- 如果某个认证方式报错就返回
- 如果摸个认证方式通过，说明认证通过，就Return,无需再运行其他认证方式
- 如果所有认证方式都没有通过，认为认证没有通过

## 身份认证策略

Kubernetes 通过身份认证插件利用客户端证书、持有者令牌（Bearer Token）或身份认证代理（Proxy） 来认证 API 请求的身份。

HTTP 请求发给 API 服务器时，插件会将以下属性关联到请求本身：

- 用户名：用来辩识最终用户的字符串。常见的值可以是 kube-admin 或 jane@example.com。
- 用户 ID：用来辩识最终用户的字符串，旨在比用户名有更好的一致性和唯一性。
- 用户组：取值为一组字符串，其中各个字符串用来标明用户是某个命名的用户逻辑集合的成员。 常见的值可能是 system:masters 或者 devops-team 等。
- 附加字段：一组额外的键-值映射，键是字符串，值是一组字符串； 用来保存一些鉴权组件可能觉得有用的额外信息。

所有（属性）值对于身份认证系统而言都是不透明的， 只有被鉴权组件解释过之后才有意义。

你可以同时启用多种身份认证方法，并且你通常会至少使用两种方法：

- 针对服务账号使用服务账号令牌
- 至少另外一种方法对用户的身份进行认证

当集群中启用了多个身份认证模块时，第一个成功地对请求完成身份认证的模块会直接做出评估决定。 API 服务器并不保证身份认证模块的运行顺序。

对于所有通过身份认证的用户，system:authenticated 组都会被添加到其组列表中。

与其它身份认证协议（LDAP、SAML、Kerberos、X509 的替代模式等等） 都可以通过使用一个身份认证代理或[身份认证 Webhoook](https://kubernetes.io/zh-cn/docs/reference/access-authn-authz/authentication/#webhook-token-authentication) 来实现。

## 代码分析

- 代码入口位置： /cmd/kube-apiserver/app/server.go
- buildGenericConfig func 里

```golang
// Authentication.ApplyTo requires already applied OpenAPIConfig and EgressSelector if present
if lastErr = s.Authentication.ApplyTo(&genericConfig.Authentication, genericConfig.SecureServing, genericConfig.EgressSelector, genericConfig.OpenAPIConfig, genericConfig.OpenAPIV3Config, clientgoExternalClient, versionedInformers); lastErr != nil {
return

```

### 真正初始化位置
文件：/pkg/kubeapiserver/options/authentication.go
```golang
authInfo.Authenticator, openAPIConfig.SecurityDefinitions, err = authenticatorConfig.New()
```

### authenticatorConfig.New() 方法
文件： /pkg/kubeapiserver/authenticator/config.go

- Token: 用于认证Token，比如Jwt Token, 服务账号
- Request: 用于认证用户，比如 client certificate 认证


```golang
// New returns an authenticator.Request or an error that supports the standard
// Kubernetes authentication mechanisms.
func (config Config) New() (authenticator.Request, *spec.SecurityDefinitions, error) {
    // 用户认证
	var authenticators []authenticator.Request
    var tokenAuthenticators []authenticator.Token
    
	// 这里添加各种插件到上面列表中
	
	// Union 
    if len(tokenAuthenticators) > 0 {
        // Union the token authenticators
        tokenAuth := tokenunion.New(tokenAuthenticators...)
        
		// Union 后通过 beearToken 包装一下 转成 authenticator.Request 添加到 authenticators 列表里
        authenticators = append(authenticators, bearertoken.New(tokenAuth), websocket.NewProtocolAuthenticator(tokenAuth))
    }

	// Union 包装一下
	authenticator := union.New(authenticators...)
   
	// 重新 包装一下，添加 user.AllAuthenticated 到 用户组
    authenticator = group.NewAuthenticatedGroupAdder(authenticator)

	// 如果允许匿名访问
    if config.Anonymous {
        // If the authenticator chain returns an error, return an error (don't consider a bad bearer token
        // or invalid username/password combination anonymous).
		// Union 一下 添加 匿名访问
        authenticator = union.NewFailOnError(authenticator, anonymous.NewAuthenticator())
    }
	
	
}


}
```

两个核心变量 Slice：
- authenticators []authenticator.Request ： 用户认证插件列表
- tokenAuthenticators []authenticator.Token： Token认证插件列表

两个认证接口：
文件： /vendor/k8s.io/apiserver/pkg/authentication/authenticator/interfaces.go
```golang
// Token checks a string value against a backing authentication store and
// returns a Response or an error if the token could not be checked.
type Token interface {
	AuthenticateToken(ctx context.Context, token string) (*Response, bool, error)
}

// Request attempts to extract authentication information from a request and
// returns a Response or an error if the request could not be checked.
type Request interface {
	AuthenticateRequest(req *http.Request) (*Response, bool, error)
}

```

### 整体流程如下：

1. 然后把各种认证插件添加到上面两个列表中，
   - authenticator.Token：
     - BasicAuth
     - ServiceAccount methods
     - BootstrapToken
     - OIDCIssuer
     - WebhookTokenAuth
   - authenticator.Request
     - X509 methods

2. union tokenAuthenticators
   tokenAuth := tokenunion.New(tokenAuthenticators...)

3. 把 tokenAuth 用 bearertoken.New(tokenAuth) 包装一下转成 authenticator.Request 同时添加到  authenticators 列表里

4. authenticators 列表用 unionAuthRequestHandler 包装一下成一个 authenticator

5. 上面 authenticator 用 AuthenticatedGroupAdder 在包装一下 成另外一个 authenticator

6. 如果允许匿名访问， 上面 authenticator 和  anonymous.NewAuthenticator() union 成一个 authenticator


整体代码设计模式：  组合 + 装饰器模式，最终把所有认证插件包装一个 authenticator.Request 对外提供服务


### tokenunion.New(tokenAuthenticators...) 分析
文件： /vendor/k8s.io/apiserver/pkg/authentication/token/union/union.go

```golang

// unionAuthTokenHandler authenticates tokens using a chain of authenticator.Token objects
type unionAuthTokenHandler struct {
	// Handlers is a chain of request authenticators to delegate to
	Handlers []authenticator.Token
	// FailOnError determines whether an error returns short-circuits the chain
	FailOnError bool
}

// New returns a token authenticator that validates credentials using a chain of authenticator.Token objects.
// The entire chain is tried until one succeeds. If all fail, an aggregate error is returned.
func New(authTokenHandlers ...authenticator.Token) authenticator.Token {
	if len(authTokenHandlers) == 1 {
		return authTokenHandlers[0]
	}
	return &unionAuthTokenHandler{Handlers: authTokenHandlers, FailOnError: false}
}


// AuthenticateToken authenticates the token using a chain of authenticator.Token objects.
func (authHandler *unionAuthTokenHandler) AuthenticateToken(ctx context.Context, token string) (*authenticator.Response, bool, error) {
	var errlist []error
	for _, currAuthRequestHandler := range authHandler.Handlers {
		info, ok, err := currAuthRequestHandler.AuthenticateToken(ctx, token)
		if err != nil {
			if authHandler.FailOnError {
				return info, ok, err
			}
			errlist = append(errlist, err)
			continue
		}

		if ok {
			return info, ok, err
		}
	}

	return nil, false, utilerrors.NewAggregate(errlist)
}

```

unionAuthTokenHandler 这个 struct 实现了 Token interface， 在 AuthenticateToken 方法遍历所有authenticator.Token, 只要有一个返回认证成功，就返回。


### bearertoken.New(tokenAuth) 分析
文件：/vendor/k8s.io/apiserver/pkg/authentication/request/bearertoken/bearertoken.go

```golang

type Authenticator struct {
	auth authenticator.Token
}

func New(auth authenticator.Token) *Authenticator {
	return &Authenticator{auth}
}

var invalidToken = errors.New("invalid bearer token")

func (a *Authenticator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	auth := strings.TrimSpace(req.Header.Get("Authorization"))
	if auth == "" {
		return nil, false, nil
	}
	parts := strings.SplitN(auth, " ", 3)
	if len(parts) < 2 || strings.ToLower(parts[0]) != "bearer" {
		return nil, false, nil
	}

	token := parts[1]

	// Empty bearer tokens aren't valid
	if len(token) == 0 {
		// The space before the token case
		if len(parts) == 3 {
			warning.AddWarning(req.Context(), "", invalidTokenWithSpaceWarning)
		}
		return nil, false, nil
	}

	resp, ok, err := a.auth.AuthenticateToken(req.Context(), token)
	// if we authenticated successfully, go ahead and remove the bearer token so that no one
	// is ever tempted to use it inside of the API server
	if ok {
		req.Header.Del("Authorization")
	}

	// If the token authenticator didn't error, provide a default error
	if !ok && err == nil {
		err = invalidToken
	}

	return resp, ok, err
}
```

Authenticator 实现 Request interface，从 http Authorization 请求头中解析出 bearer token， 同时调用 authenticator.Token （上一步 Union 包装后插件认证）
认证通过后移除  bearer token。


### authenticator := union.New(authenticators...) 分析
文件： /vendor/k8s.io/apiserver/pkg/authentication/request/union/union.go

```golang


// unionAuthRequestHandler authenticates requests using a chain of authenticator.Requests
type unionAuthRequestHandler struct {
	// Handlers is a chain of request authenticators to delegate to
	Handlers []authenticator.Request
	// FailOnError determines whether an error returns short-circuits the chain
	FailOnError bool
}

// New returns a request authenticator that validates credentials using a chain of authenticator.Request objects.
// The entire chain is tried until one succeeds. If all fail, an aggregate error is returned.
func New(authRequestHandlers ...authenticator.Request) authenticator.Request {
	if len(authRequestHandlers) == 1 {
		return authRequestHandlers[0]
	}
	return &unionAuthRequestHandler{Handlers: authRequestHandlers, FailOnError: false}
}


// AuthenticateRequest authenticates the request using a chain of authenticator.Request objects.
func (authHandler *unionAuthRequestHandler) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	var errlist []error
	for _, currAuthRequestHandler := range authHandler.Handlers {
		resp, ok, err := currAuthRequestHandler.AuthenticateRequest(req)
		if err != nil {
			if authHandler.FailOnError {
				return resp, ok, err
			}
			errlist = append(errlist, err)
			continue
		}

		if ok {
			return resp, ok, err
		}
	}

	return nil, false, utilerrors.NewAggregate(errlist)
}

```

unionAuthRequestHandler 实现 Request interface，在 AuthenticateRequest 方法中遍历每个authenticator.Request， 只要有一个认证成功就返回


### authenticator = group.NewAuthenticatedGroupAdder(authenticator) 分析
文件： /vendor/k8s.io/apiserver/pkg/authentication/group/authenticated_group_adder.go

```golang

// AuthenticatedGroupAdder adds system:authenticated group when appropriate
type AuthenticatedGroupAdder struct {
	// Authenticator is delegated to make the authentication decision
	Authenticator authenticator.Request
}

// NewAuthenticatedGroupAdder wraps a request authenticator, and adds the system:authenticated group when appropriate.
// Authentication must succeed, the user must not be system:anonymous, the groups system:authenticated or system:unauthenticated must
// not be present
func NewAuthenticatedGroupAdder(auth authenticator.Request) authenticator.Request {
	return &AuthenticatedGroupAdder{auth}
}

func (g *AuthenticatedGroupAdder) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	r, ok, err := g.Authenticator.AuthenticateRequest(req)
	if err != nil || !ok {
		return nil, ok, err
	}

	if r.User.GetName() == user.Anonymous {
		return r, true, nil
	}
	for _, group := range r.User.GetGroups() {
		if group == user.AllAuthenticated || group == user.AllUnauthenticated {
			return r, true, nil
		}
	}

	newGroups := make([]string, 0, len(r.User.GetGroups())+1)
	newGroups = append(newGroups, r.User.GetGroups()...)
	newGroups = append(newGroups, user.AllAuthenticated)

	ret := *r // shallow copy
	ret.User = &user.DefaultInfo{
		Name:   r.User.GetName(),
		UID:    r.User.GetUID(),
		Groups: newGroups,
		Extra:  r.User.GetExtra(),
	}
	return &ret, true, nil
}

```

unionAuthRequestHandler 实现 Request interface，在 AuthenticateRequest 方法中认证成功后把 system:authenticated 添加到用户组里

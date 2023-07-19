# RESTStorage 初始化

KubeAPIServer 在核心路由注册过程中，主要分成两步:
- 第一步创建 RESTStorage 将后端存储与资源进行绑定
- 第二步才进行路由注册

## 入口位置

```golang
// /pkg/controlplane/instance.go #580
// InstallLegacyAPI will install the legacy APIs for the restStorageProviders if they are enabled.
func (m *Instance) InstallLegacyAPI(c *completedConfig, restOptionsGetter generic.RESTOptionsGetter) error {
	// ...
    // 将各种资源和对应的后端存储（etcd）的操作绑定
    legacyRESTStorage, apiGroupInfo, err := legacyRESTStorageProvider.NewLegacyRESTStorage(c.ExtraConfig.APIResourceConfigSource, restOptionsGetter)
	
	// ...
}
```


## 各种资源的 RESTStorage 初始化

```golang
// pkg/registry/core/rest/storage_core.go #109
func (c LegacyRESTStorageProvider) NewLegacyRESTStorage(apiResourceConfigSource serverstorage.APIResourceConfigSource, restOptionsGetter generic.RESTOptionsGetter) (LegacyRESTStorage, genericapiserver.APIGroupInfo, error) {
     // ...
     restStorage := LegacyRESTStorage{}
	 
     // PodTemplate 资源的 RESTStorage 初始化
     podTemplateStorage, err := podtemplatestore.NewREST(restOptionsGetter)
    
     // Event 资源的 RESTStorage 初始化
     eventStorage, err := eventstore.NewREST(restOptionsGetter, uint64(c.EventTTL.Seconds()))
    
     // LimitRange 资源的 RESTStorage 初始化
     limitRangeStorage, err := limitrangestore.NewREST(restOptionsGetter)
    
     // ResourceQuota 资源的 RESTStorage 初始化
     resourceQuotaStorage, resourceQuotaStatusStorage, err := resourcequotastore.NewREST(restOptionsGetter)
    
     // Secret 资源的 RESTStorage 初始化
     secretStorage, err := secretstore.NewREST(restOptionsGetter)
    
     // PersistentVolume 资源的 RESTStorage 初始化
     persistentVolumeStorage, persistentVolumeStatusStorage, err := pvstore.NewREST(restOptionsGetter)
    
     // PersistentVolumeClaim 资源的 RESTStorage 初始化
     persistentVolumeClaimStorage, persistentVolumeClaimStatusStorage, err := pvcstore.NewREST(restOptionsGetter)
    
     // ConfigMap 资源的 RESTStorage 初始化
     configMapStorage, err := configmapstore.NewREST(restOptionsGetter)
    
     // Namespace 资源的 RESTStorage 初始化
     namespaceStorage, namespaceStatusStorage, namespaceFinalizeStorage, err := namespacestore.NewREST(restOptionsGetter)
    
     // Endpoints 资源的 RESTStorage 初始化
     endpointsStorage, err := endpointsstore.NewREST(restOptionsGetter)
    
     // Node 资源的 RESTStorage 初始化
     nodeStorage, err := nodestore.NewStorage(restOptionsGetter, c.KubeletClientConfig, c.ProxyTransport)
    
     // Pod 资源的 RESTStorage 初始化
     podStorage, err := podstore.NewStorage(
         restOptionsGetter,
        nodeStorage.KubeletConnectionInfo,
        c.ProxyTransport,
        podDisruptionClient,
    )
    
     // ServiceAccount 资源的 RESTStorage 初始化
    var serviceAccountStorage *serviceaccountstore.REST
    if c.ServiceAccountIssuer != nil {
        serviceAccountStorage, err = serviceaccountstore.NewREST(restOptionsGetter, c.ServiceAccountIssuer, c.APIAudiences, c.ServiceAccountMaxExpiration, podStorage.Pod.Store, secretStorage.Store, c.ExtendExpiration)
    } else {
        serviceAccountStorage, err = serviceaccountstore.NewREST(restOptionsGetter, nil, nil, 0, nil, nil, false)
    }


    // ......
}

```

这里每个资源的初始化都传递了 generic.RESTOptionsGetter 类型参数，即存储接口的实现

RESTOptionsGetter 接口如下

```golang
// /vendor/k8s.io/apiserver/pkg/registry/generic/options.go #46
type RESTOptionsGetter interface {
 GetRESTOptions(resource schema.GroupResource) (RESTOptions, error)
}

```

跟踪一下存储接口的初始化发现是在 buildGenericConfig 初始化 ETCD 存储实现

```golang
// cmd/kube-apiserver/app/server.go #247 
genericConfig, versionedInformers, serviceResolver, pluginInitializers, admissionPostStartHook, storageFactory, err := buildGenericConfig(s.ServerRunOptions, proxyTransport)
	
```

```golang
// cmd/kube-apiserver/app/server.go #342
func buildGenericConfig(s *options.ServerRunOptions, proxyTransport *http.Transport, ) {
    // ...
	// 初始化 ETCD 存储
    storageFactoryConfig := kubeapiserver.NewStorageFactoryConfig()
    storageFactoryConfig.APIResourceConfig = genericConfig.MergedResourceConfig
    storageFactory, lastErr = storageFactoryConfig.Complete(s.Etcd).New()
    if lastErr != nil {
     return
    }
    if lastErr = s.Etcd.ApplyWithStorageFactoryTo(storageFactory, genericConfig); lastErr != nil {
        return
    }
	//...
}
````

```golang
// /vendor/k8s.io/apiserver/pkg/server/options/etcd.go #304
// ApplyWithStorageFactoryTo mutates the provided server.Config.  It must never mutate the receiver (EtcdOptions).
func (s *EtcdOptions) ApplyWithStorageFactoryTo(factory serverstorage.StorageFactory, c *server.Config) error {
	if s == nil {
		return nil
	}

	if !s.complete {
		return fmt.Errorf("EtcdOptions.Apply called without completion")
	}

	if !s.SkipHealthEndpoints {
		if err := s.addEtcdHealthEndpoint(c); err != nil {
			return err
		}
	}

	if s.resourceTransformers != nil {
		factory = &transformerStorageFactory{
			delegate:             factory,
			resourceTransformers: s.resourceTransformers,
		}
	}

	c.RESTOptionsGetter = &StorageFactoryRestOptionsFactory{Options: *s, StorageFactory: factory}
	return nil
}


```

这里返回 StorageFactoryRestOptionsFactory， 实现 RESTOptionsGetter 接口

```golang
// /vendor/k8s.io/apiserver/pkg/server/options/etcd.go #352
type StorageFactoryRestOptionsFactory struct {
	Options        EtcdOptions
	StorageFactory serverstorage.StorageFactory
}

```

## 将资源和对应的 RESTStorage 进行绑定

```golang
// pkg/registry/core/rest/storage_core.go #109
func (c LegacyRESTStorageProvider) NewLegacyRESTStorage(apiResourceConfigSource serverstorage.APIResourceConfigSource, restOptionsGetter generic.RESTOptionsGetter) (LegacyRESTStorage, genericapiserver.APIGroupInfo, error) {
 // ......

 // 利用 map 来保存资源的 http path 和对应的 RESTStorage
 storage := map[string]rest.Storage{}
 if resource := "pods"; apiResourceConfigSource.ResourceEnabled(corev1.SchemeGroupVersion.WithResource(resource)) {
  storage[resource] = podStorage.Pod

  // 对于 Pod 资源，有很多细分的 path 及其 RESTStorage
  storage[resource+"/attach"] = podStorage.Attach
  storage[resource+"/status"] = podStorage.Status
  storage[resource+"/log"] = podStorage.Log
  storage[resource+"/exec"] = podStorage.Exec
  storage[resource+"/portforward"] = podStorage.PortForward
  storage[resource+"/proxy"] = podStorage.Proxy
  storage[resource+"/binding"] = podStorage.Binding
  if podStorage.Eviction != nil {
   storage[resource+"/eviction"] = podStorage.Eviction
  }
  storage[resource+"/ephemeralcontainers"] = podStorage.EphemeralContainers

 }
 if resource := "bindings"; apiResourceConfigSource.ResourceEnabled(corev1.SchemeGroupVersion.WithResource(resource)) {
  storage[resource] = podStorage.LegacyBinding
 }

 if resource := "podtemplates"; apiResourceConfigSource.ResourceEnabled(corev1.SchemeGroupVersion.WithResource(resource)) {
  storage[resource] = podTemplateStorage
 }

 if resource := "replicationcontrollers"; apiResourceConfigSource.ResourceEnabled(corev1.SchemeGroupVersion.WithResource(resource)) {
  storage[resource] = controllerStorage.Controller
  storage[resource+"/status"] = controllerStorage.Status
  if legacyscheme.Scheme.IsVersionRegistered(schema.GroupVersion{Group: "autoscaling", Version: "v1"}) {
   storage[resource+"/scale"] = controllerStorage.Scale
  }
 }

 if resource := "services"; apiResourceConfigSource.ResourceEnabled(corev1.SchemeGroupVersion.WithResource(resource)) {
  storage[resource] = serviceRESTStorage
  storage[resource+"/proxy"] = serviceRESTProxy
  storage[resource+"/status"] = serviceStatusStorage
 }

 if resource := "endpoints"; apiResourceConfigSource.ResourceEnabled(corev1.SchemeGroupVersion.WithResource(resource)) {
  storage[resource] = endpointsStorage
 }

 if resource := "nodes"; apiResourceConfigSource.ResourceEnabled(corev1.SchemeGroupVersion.WithResource(resource)) {
  storage[resource] = nodeStorage.Node
  storage[resource+"/proxy"] = nodeStorage.Proxy
  storage[resource+"/status"] = nodeStorage.Status
 }

 if resource := "events"; apiResourceConfigSource.ResourceEnabled(corev1.SchemeGroupVersion.WithResource(resource)) {
  storage[resource] = eventStorage
 }

 if resource := "limitranges"; apiResourceConfigSource.ResourceEnabled(corev1.SchemeGroupVersion.WithResource(resource)) {
  storage[resource] = limitRangeStorage
 }

 if resource := "resourcequotas"; apiResourceConfigSource.ResourceEnabled(corev1.SchemeGroupVersion.WithResource(resource)) {
  storage[resource] = resourceQuotaStorage
  storage[resource+"/status"] = resourceQuotaStatusStorage
 }

 if resource := "namespaces"; apiResourceConfigSource.ResourceEnabled(corev1.SchemeGroupVersion.WithResource(resource)) {
  storage[resource] = namespaceStorage
  storage[resource+"/status"] = namespaceStatusStorage
  storage[resource+"/finalize"] = namespaceFinalizeStorage
 }

 if resource := "secrets"; apiResourceConfigSource.ResourceEnabled(corev1.SchemeGroupVersion.WithResource(resource)) {
  storage[resource] = secretStorage
 }

 if resource := "serviceaccounts"; apiResourceConfigSource.ResourceEnabled(corev1.SchemeGroupVersion.WithResource(resource)) {
  storage[resource] = serviceAccountStorage
  if serviceAccountStorage.Token != nil {
   storage[resource+"/token"] = serviceAccountStorage.Token
  }
 }

 if resource := "persistentvolumes"; apiResourceConfigSource.ResourceEnabled(corev1.SchemeGroupVersion.WithResource(resource)) {
  storage[resource] = persistentVolumeStorage
  storage[resource+"/status"] = persistentVolumeStatusStorage
 }

 if resource := "persistentvolumeclaims"; apiResourceConfigSource.ResourceEnabled(corev1.SchemeGroupVersion.WithResource(resource)) {
  storage[resource] = persistentVolumeClaimStorage
  storage[resource+"/status"] = persistentVolumeClaimStatusStorage
 }

 if resource := "configmaps"; apiResourceConfigSource.ResourceEnabled(corev1.SchemeGroupVersion.WithResource(resource)) {
  storage[resource] = configMapStorage
 }

 if resource := "componentstatuses"; apiResourceConfigSource.ResourceEnabled(corev1.SchemeGroupVersion.WithResource(resource)) {
  storage[resource] = componentstatus.NewStorage(componentStatusStorage{c.StorageFactory}.serversToValidate)
 }

 if len(storage) > 0 {
  apiGroupInfo.VersionedResourcesStorageMap["v1"] = storage
 }

 return restStorage, apiGroupInfo, nil
}

```

存储了所有资源路径和对应 RESTStorage 的 map 结构最后会保存在 apiGroupInfo 中并返回给后续的路由注册使用。 这样后续的 handler 就可以通过对应的 RESTStorage 来操作 etcd 存取数据，同时这种做法还可以划分每个资源对 Storage 操作的权限，保障数据的安全性。

# Pod RESTStorage

以 pod 的 RestStorage 为例来介绍 RestStorage 初始化，和 createPod 的流程

## pod RestStorage 初始化

```golang

// NewStorage returns a RESTStorage object that will work against pods.
func NewStorage(optsGetter generic.RESTOptionsGetter, k client.ConnectionInfoGetter, proxyTransport http.RoundTripper, podDisruptionBudgetClient policyclient.PodDisruptionBudgetsGetter) (PodStorage, error) {
    // 初始化 store
	store := &genericregistry.Store{
		NewFunc:                   func() runtime.Object { return &api.Pod{} },
		NewListFunc:               func() runtime.Object { return &api.PodList{} },
		PredicateFunc:             registrypod.MatchPod,
		DefaultQualifiedResource:  api.Resource("pods"),
		SingularQualifiedResource: api.Resource("pod"),

		CreateStrategy:      registrypod.Strategy,
		UpdateStrategy:      registrypod.Strategy,
		DeleteStrategy:      registrypod.Strategy,
		ResetFieldsStrategy: registrypod.Strategy,
		ReturnDeletedObject: true,

		TableConvertor: printerstorage.TableConvertor{TableGenerator: printers.NewTableGenerator().With(printersinternal.AddHandlers)},
	}
	
	// options 初始化
	options := &generic.StoreOptions{
		RESTOptions: optsGetter,
		AttrFunc:    registrypod.GetAttrs,
		TriggerFunc: map[string]storage.IndexerFunc{"spec.nodeName": registrypod.NodeNameTriggerFunc},
		Indexers:    registrypod.Indexers(),
	}
	if err := store.CompleteWithOptions(options); err != nil {
		return PodStorage{}, err
	}

	// ...
	
	// Pod 和其他子资源
	return PodStorage{
		Pod:                 &REST{store, proxyTransport},
		Binding:             &BindingREST{store: store},
		LegacyBinding:       &LegacyBindingREST{bindingREST},
		Eviction:            newEvictionStorage(&statusStore, podDisruptionBudgetClient),
		Status:              &StatusREST{store: &statusStore},
		EphemeralContainers: &EphemeralContainersREST{store: &ephemeralContainersStore},
		Log:                 &podrest.LogREST{Store: store, KubeletConn: k},
		Proxy:               &podrest.ProxyREST{Store: store, ProxyTransport: proxyTransport},
		Exec:                &podrest.ExecREST{Store: store, KubeletConn: k},
		Attach:              &podrest.AttachREST{Store: store, KubeletConn: k},
		PortForward:         &podrest.PortForwardREST{Store: store, KubeletConn: k},
	}, nil
}

```

这里返回 PodStorage, 包含 Pod 和其他子资源 RestStorage

```golang

// PodStorage includes storage for pods and all sub resources
type PodStorage struct {
	Pod                 *REST
	Binding             *BindingREST
	LegacyBinding       *LegacyBindingREST
	Eviction            *EvictionREST
	Status              *StatusREST
	EphemeralContainers *EphemeralContainersREST
	Log                 *podrest.LogREST
	Proxy               *podrest.ProxyREST
	Exec                *podrest.ExecREST
	Attach              *podrest.AttachREST
	PortForward         *podrest.PortForwardREST
}

```

## genericregistry.Store

所有 restStorage 初始化来子 genericregistry.Store 这个 struct

```golang
// k8s.io/apiserver/pkg/registry/generic/registry/store.go #96
type Store struct {
	// NewFunc returns a new instance of the type this registry returns for a
	// GET of a single object, e.g.:
	//
	// curl GET /apis/group/version/namespaces/my-ns/myresource/name-of-object
	NewFunc func() runtime.Object

	// NewListFunc returns a new list of the type this registry; it is the
	// type returned when the resource is listed, e.g.:
	//
	// curl GET /apis/group/version/namespaces/my-ns/myresource
	NewListFunc func() runtime.Object

	// DefaultQualifiedResource is the pluralized name of the resource.
	// This field is used if there is no request info present in the context.
	// See qualifiedResourceFromContext for details.
	DefaultQualifiedResource schema.GroupResource

	// SingularQualifiedResource is the singular name of the resource.
	SingularQualifiedResource schema.GroupResource

	// KeyRootFunc returns the root etcd key for this resource; should not
	// include trailing "/".  This is used for operations that work on the
	// entire collection (listing and watching).
	//
	// KeyRootFunc and KeyFunc must be supplied together or not at all.
	KeyRootFunc func(ctx context.Context) string

	// KeyFunc returns the key for a specific object in the collection.
	// KeyFunc is called for Create/Update/Get/Delete. Note that 'namespace'
	// can be gotten from ctx.
	//
	// KeyFunc and KeyRootFunc must be supplied together or not at all.
	KeyFunc func(ctx context.Context, name string) (string, error)

	// ObjectNameFunc returns the name of an object or an error.
	ObjectNameFunc func(obj runtime.Object) (string, error)

	// TTLFunc returns the TTL (time to live) that objects should be persisted
	// with. The existing parameter is the current TTL or the default for this
	// operation. The update parameter indicates whether this is an operation
	// against an existing object.
	//
	// Objects that are persisted with a TTL are evicted once the TTL expires.
	TTLFunc func(obj runtime.Object, existing uint64, update bool) (uint64, error)

	// PredicateFunc returns a matcher corresponding to the provided labels
	// and fields. The SelectionPredicate returned should return true if the
	// object matches the given field and label selectors.
	PredicateFunc func(label labels.Selector, field fields.Selector) storage.SelectionPredicate

	// EnableGarbageCollection affects the handling of Update and Delete
	// requests. Enabling garbage collection allows finalizers to do work to
	// finalize this object before the store deletes it.
	//
	// If any store has garbage collection enabled, it must also be enabled in
	// the kube-controller-manager.
	EnableGarbageCollection bool

	// DeleteCollectionWorkers is the maximum number of workers in a single
	// DeleteCollection call. Delete requests for the items in a collection
	// are issued in parallel.
	DeleteCollectionWorkers int

	// Decorator is an optional exit hook on an object returned from the
	// underlying storage. The returned object could be an individual object
	// (e.g. Pod) or a list type (e.g. PodList). Decorator is intended for
	// integrations that are above storage and should only be used for
	// specific cases where storage of the value is not appropriate, since
	// they cannot be watched.
	Decorator func(runtime.Object)

	// CreateStrategy implements resource-specific behavior during creation.
	CreateStrategy rest.RESTCreateStrategy
	// BeginCreate is an optional hook that returns a "transaction-like"
	// commit/revert function which will be called at the end of the operation,
	// but before AfterCreate and Decorator, indicating via the argument
	// whether the operation succeeded.  If this returns an error, the function
	// is not called.  Almost nobody should use this hook.
	BeginCreate BeginCreateFunc
	// AfterCreate implements a further operation to run after a resource is
	// created and before it is decorated, optional.
	AfterCreate AfterCreateFunc

	// UpdateStrategy implements resource-specific behavior during updates.
	UpdateStrategy rest.RESTUpdateStrategy
	// BeginUpdate is an optional hook that returns a "transaction-like"
	// commit/revert function which will be called at the end of the operation,
	// but before AfterUpdate and Decorator, indicating via the argument
	// whether the operation succeeded.  If this returns an error, the function
	// is not called.  Almost nobody should use this hook.
	BeginUpdate BeginUpdateFunc
	// AfterUpdate implements a further operation to run after a resource is
	// updated and before it is decorated, optional.
	AfterUpdate AfterUpdateFunc

	// DeleteStrategy implements resource-specific behavior during deletion.
	DeleteStrategy rest.RESTDeleteStrategy
	// AfterDelete implements a further operation to run after a resource is
	// deleted and before it is decorated, optional.
	AfterDelete AfterDeleteFunc
	// ReturnDeletedObject determines whether the Store returns the object
	// that was deleted. Otherwise, return a generic success status response.
	ReturnDeletedObject bool
	// ShouldDeleteDuringUpdate is an optional function to determine whether
	// an update from existing to obj should result in a delete.
	// If specified, this is checked in addition to standard finalizer,
	// deletionTimestamp, and deletionGracePeriodSeconds checks.
	ShouldDeleteDuringUpdate func(ctx context.Context, key string, obj, existing runtime.Object) bool

	// TableConvertor is an optional interface for transforming items or lists
	// of items into tabular output. If unset, the default will be used.
	TableConvertor rest.TableConvertor

	// ResetFieldsStrategy provides the fields reset by the strategy that
	// should not be modified by the user.
	ResetFieldsStrategy rest.ResetFieldsStrategy

	// Storage is the interface for the underlying storage for the
	// resource. It is wrapped into a "DryRunnableStorage" that will
	// either pass-through or simply dry-run.
	Storage DryRunnableStorage
	// StorageVersioner outputs the <group/version/kind> an object will be
	// converted to before persisted in etcd, given a list of possible
	// kinds of the object.
	// If the StorageVersioner is nil, apiserver will leave the
	// storageVersionHash as empty in the discovery document.
	StorageVersioner runtime.GroupVersioner

	// DestroyFunc cleans up clients used by the underlying Storage; optional.
	// If set, DestroyFunc has to be implemented in thread-safe way and
	// be prepared for being called more than once.
	DestroyFunc func()
}

```
其中
- NewFunc 表示 Get 一个对象的方法
- NewFuncList 表示 list 对象时的方法
- PredicateFunc 表示返回与提供标签对应的匹配器和字段，如果 object 匹配对应标签和字段，匹配器返回 True
- DefaultQualifiedResource 是资源复数名称
- CreateStrategy 表示创建的策略
- UpdateStrategy 表示更新的策略
- DeleteStrategy 表示删除的策略
- BeginCreate 表示对象开始创建回调 Hook 
- AfterCreate 表示对象创建完成回调 Hook
- BeginUpdate 表示对象开始更新回调 Hook
- AfterUpdate 表示对象完成更新后回调 Hook
- TableConvertor 表示输出为表格的方法

## Store.Create 方法

```golang
// Create inserts a new item according to the unique key from the object.
// Note that registries may mutate the input object (e.g. in the strategy
// hooks).  Tests which call this might want to call DeepCopy if they expect to
// be able to examine the input and output objects for differences.
func (e *Store) Create(ctx context.Context, obj runtime.Object, createValidation rest.ValidateObjectFunc, options *metav1.CreateOptions) (runtime.Object, error) {
	var finishCreate FinishFunc = finishNothing

	// Init metadata as early as possible.
	if objectMeta, err := meta.Accessor(obj); err != nil {
		return nil, err
	} else {
		rest.FillObjectMetaSystemFields(objectMeta)
		if len(objectMeta.GetGenerateName()) > 0 && len(objectMeta.GetName()) == 0 {
			objectMeta.SetName(e.CreateStrategy.GenerateName(objectMeta.GetGenerateName()))
		}
	}

	if e.BeginCreate != nil {
		fn, err := e.BeginCreate(ctx, obj, options)
		if err != nil {
			return nil, err
		}
		finishCreate = fn
		defer func() {
			finishCreate(ctx, false)
		}()
	}

	if err := rest.BeforeCreate(e.CreateStrategy, ctx, obj); err != nil {
		return nil, err
	}
	// at this point we have a fully formed object.  It is time to call the validators that the apiserver
	// handling chain wants to enforce.
	if createValidation != nil {
		if err := createValidation(ctx, obj.DeepCopyObject()); err != nil {
			return nil, err
		}
	}

	name, err := e.ObjectNameFunc(obj)
	if err != nil {
		return nil, err
	}
	key, err := e.KeyFunc(ctx, name)
	if err != nil {
		return nil, err
	}
	qualifiedResource := e.qualifiedResourceFromContext(ctx)
	ttl, err := e.calculateTTL(obj, 0, false)
	if err != nil {
		return nil, err
	}
	out := e.NewFunc()
	if err := e.Storage.Create(ctx, key, obj, out, ttl, dryrun.IsDryRun(options.DryRun)); err != nil {
		err = storeerr.InterpretCreateError(err, qualifiedResource, name)
		err = rest.CheckGeneratedNameError(ctx, e.CreateStrategy, err, obj)
		if !apierrors.IsAlreadyExists(err) {
			return nil, err
		}
		if errGet := e.Storage.Get(ctx, key, storage.GetOptions{}, out); errGet != nil {
			return nil, err
		}
		accessor, errGetAcc := meta.Accessor(out)
		if errGetAcc != nil {
			return nil, err
		}
		if accessor.GetDeletionTimestamp() != nil {
			msg := &err.(*apierrors.StatusError).ErrStatus.Message
			*msg = fmt.Sprintf("object is being deleted: %s", *msg)
		}
		return nil, err
	}
	// The operation has succeeded.  Call the finish function if there is one,
	// and then make sure the defer doesn't call it again.
	fn := finishCreate
	finishCreate = finishNothing
	fn(ctx, true)

	if e.AfterCreate != nil {
		e.AfterCreate(out, options)
	}
	if e.Decorator != nil {
		e.Decorator(out)
	}
	return out, nil
}
```

### 1. 先调用 BeginCreate

```golang
	if e.BeginCreate != nil {
		fn, err := e.BeginCreate(ctx, obj, options)
		if err != nil {
			return nil, err
		}
		finishCreate = fn
		defer func() {
			finishCreate(ctx, false)
		}()
	}
```

### 2. 然后调用 BeforeCreate

```golang
    if err := rest.BeforeCreate(e.CreateStrategy, ctx, obj); err != nil {
		return nil, err
	}
```

### 3. BeforeCreate 内部分析

```golang
// BeforeCreate ensures that common operations for all resources are performed on creation. It only returns
// errors that can be converted to api.Status. It invokes PrepareForCreate, then Validate.
// It returns nil if the object should be created.
func BeforeCreate(strategy RESTCreateStrategy, ctx context.Context, obj runtime.Object) error {
	
    // ...
	
	// 调用 POD strategy PrepareForCreate
	strategy.PrepareForCreate(ctx, obj)

	// 调用 POD strategy Validate
	if errs := strategy.Validate(ctx, obj); len(errs) > 0 {
		return errors.NewInvalid(kind.GroupKind(), objectMeta.GetName(), errs)
	}

	// Custom validation (including name validation) passed
	// Now run common validation on object meta
	// Do this *after* custom validation so that specific error messages are shown whenever possible
	if errs := genericvalidation.ValidateObjectMetaAccessor(objectMeta, strategy.NamespaceScoped(), path.ValidatePathSegmentName, field.NewPath("metadata")); len(errs) > 0 {
		return errors.NewInvalid(kind.GroupKind(), objectMeta.GetName(), errs)
	}

	// 调用 POD strategy WarningsOnCreate
	for _, w := range strategy.WarningsOnCreate(ctx, obj) {
		warning.AddWarning(ctx, "", w)
	}

    // 调用 POD strategy Canonicalize
	strategy.Canonicalize(obj)

	return nil
}
```
在 BeforeCreate 里回调用 Pod strategy
- PrepareForCreate
- Validate
- WarningsOnCreate
- Canonicalize

## podStrategy  PrepareForCreate

```golang

// PrepareForCreate clears fields that are not allowed to be set by end users on creation.
func (podStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	pod := obj.(*api.Pod)
	pod.Status = api.PodStatus{
		Phase:    api.PodPending,
		QOSClass: qos.GetPodQOS(pod),
	}

	podutil.DropDisabledPodFields(pod, nil)

	applyWaitingForSchedulingGatesCondition(pod)
}

```
主要做
- 设置 Pod phase 为 Pending
- 设置 POD QOSClass 
- 去掉一些字段


POD QOSClass: Guaranteed, Burstable, BestEffort

```golang
// PodQOSClass defines the supported qos classes of Pods.
type PodQOSClass string

// These are valid values for PodQOSClass
const (
	// PodQOSGuaranteed is the Guaranteed qos class.
	PodQOSGuaranteed PodQOSClass = "Guaranteed"
	// PodQOSBurstable is the Burstable qos class.
	PodQOSBurstable PodQOSClass = "Burstable"
	// PodQOSBestEffort is the BestEffort qos class.
	PodQOSBestEffort PodQOSClass = "BestEffort"
)

```

### e.Storage.Create

```golang
    out := e.NewFunc()
	if err := e.Storage.Create(ctx, key, obj, out, ttl, dryrun.IsDryRun(options.DryRun)); err != nil {
		// ...
	}
```

storage.Create 调用是 DryRunnableStorage 的 Create

```golang
// k8s.io/apiserver/pkg/registry/generic/registry/dryrun.go #36
func (s *DryRunnableStorage) Create(ctx context.Context, key string, obj, out runtime.Object, ttl uint64, dryRun bool) error {
	if dryRun {
		if err := s.Storage.Get(ctx, key, storage.GetOptions{}, out); err == nil {
			return storage.NewKeyExistsError(key, 0)
		}
		return s.copyInto(obj, out)
	}
	return s.Storage.Create(ctx, key, obj, out, ttl)
}
```

如果是 dryRun, 不保存到etcd中，只是返回资源

### etcdv3 的 Create

```golang
// k8s.io/apiserver/pkg/storage/etcd3/store.go #169
// Create implements storage.Interface.Create.
func (s *store) Create(ctx context.Context, key string, obj, out runtime.Object, ttl uint64) error {
	preparedKey, err := s.prepareKey(key)
	if err != nil {
		return err
	}
	ctx, span := tracing.Start(ctx, "Create etcd3",
		attribute.String("audit-id", audit.GetAuditIDTruncated(ctx)),
		attribute.String("key", key),
		attribute.String("type", getTypeName(obj)),
		attribute.String("resource", s.groupResourceString),
	)
	defer span.End(500 * time.Millisecond)
	if version, err := s.versioner.ObjectResourceVersion(obj); err == nil && version != 0 {
		return errors.New("resourceVersion should not be set on objects to be created")
	}
	if err := s.versioner.PrepareObjectForStorage(obj); err != nil {
		return fmt.Errorf("PrepareObjectForStorage failed: %v", err)
	}
	span.AddEvent("About to Encode")
	data, err := runtime.Encode(s.codec, obj)
	if err != nil {
		span.AddEvent("Encode failed", attribute.Int("len", len(data)), attribute.String("err", err.Error()))
		return err
	}
	span.AddEvent("Encode succeeded", attribute.Int("len", len(data)))

	opts, err := s.ttlOpts(ctx, int64(ttl))
	if err != nil {
		return err
	}

	newData, err := s.transformer.TransformToStorage(ctx, data, authenticatedDataString(preparedKey))
	if err != nil {
		span.AddEvent("TransformToStorage failed", attribute.String("err", err.Error()))
		return storage.NewInternalError(err.Error())
	}
	span.AddEvent("TransformToStorage succeeded")

	startTime := time.Now()
	txnResp, err := s.client.KV.Txn(ctx).If(
		notFound(preparedKey),
	).Then(
		clientv3.OpPut(preparedKey, string(newData), opts...),
	).Commit()
	metrics.RecordEtcdRequestLatency("create", s.groupResourceString, startTime)
	if err != nil {
		span.AddEvent("Txn call failed", attribute.String("err", err.Error()))
		return err
	}
	span.AddEvent("Txn call succeeded")

	if !txnResp.Succeeded {
		return storage.NewKeyExistsError(preparedKey, 0)
	}

	if out != nil {
		putResp := txnResp.Responses[0].GetResponsePut()
		err = decode(s.codec, s.versioner, data, out, putResp.Header.Revision)
		if err != nil {
			span.AddEvent("decode failed", attribute.Int("len", len(data)), attribute.String("err", err.Error()))
			recordDecodeError(s.groupResourceString, preparedKey)
			return err
		}
		span.AddEvent("decode succeeded", attribute.Int("len", len(data)))
	}
	return nil
}
```
## 执行 AfterCreate

```golang
    if e.AfterCreate != nil {
		e.AfterCreate(out, options)
	}
```


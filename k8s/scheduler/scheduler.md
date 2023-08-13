# Scheduler

## 1. 调度定义

### 1.1. Scheduler定义

Scheduler要想实现调度一个Pod的全流程，那么必须有[调度队列]、[调度缓存]、[调度框架]、[调度插件]、[调度算法]等模块的支持。所以，Scheduler的成员变量势必会包含这些类型。

```golang
// /pkg/scheduler/scheduler.go
// Scheduler watches for new unscheduled pods. It attempts to find
// nodes that they fit on and writes bindings back to the api server.
// Scheduler监视未调度的Pod并尝试找到适合的Node，并将绑定信息写回到apiserver。
type Scheduler struct {
	// It is expected that changes made via Cache will be observed
	// by NodeLister and Algorithm.
	// 调度缓存，用来缓存所有的Node状态，
	// 了解过调度算法的读者应该知道，调度缓存也会被调度算法使用，用来每次调度前更新快照
	Cache internalcache.Cache
	

	Extenders []framework.Extender
	
	// NextPod should be a function that blocks until the next pod
	// is available. We don't use a channel for this, because scheduling
	// a pod may take some amount of time and we don't want pods to get
	// stale while they sit in a channel.
    // NextPod()获取下一个需要调度的Pod(如果没有则阻塞当前协程)，是不是想到了调度队列(SchedulingQueue)?
    // 那么问题来了，为什么不直接使用调度队列(下面有调度队列的成员变量)而是注入一个函数呢？
	NextPod func() *framework.QueuedPodInfo

	// FailureHandler is called upon a scheduling failure.
	// 当调度一个Pod出现错误的时候调用函数，是Scheduler使用者注入的错误函数，相当于回调。
	FailureHandler FailureHandlerFn

	// SchedulePod tries to schedule the given pod to one of the nodes in the node list.
	// Return a struct of ScheduleResult with the name of suggested host on success,
	// otherwise will return a FitError with reasons.
	// 调度算法
	SchedulePod func(ctx context.Context, fwk framework.Framework, state *framework.CycleState, pod *v1.Pod) (ScheduleResult, error)

	// Close this to shut down the scheduler.
    // 关闭Scheduler的信号
	StopEverything <-chan struct{}

	// SchedulingQueue holds pods to be scheduled
	// 调度队列，用来缓存等待调度的Pod
	SchedulingQueue internalqueue.SchedulingQueue

	// Profiles are the scheduling profiles.
	// 别看名字是Profile，其实是Framework，因为Profile和Framework是一对一的，Profile只是Framework的配置而已
	Profiles profile.Map

	// apiserver的客户端，主要用来向apiserver执行写操作，比如写Pod的绑定信息
	client clientset.Interface

	// nodeInfo 快照
	nodeInfoSnapshot *internalcache.Snapshot

	percentageOfNodesToScore int32

	// 下个节点开始位置
	nextStartNodeIndex int
}


```

### 1.2 应用默认值

```golang
func (s *Scheduler) applyDefaultHandlers() {
	s.SchedulePod = s.schedulePod
	s.FailureHandler = s.handleSchedulingFailure
}
```
这里设置 调度算法 和 调度错误回调两个函数默认实现是 Scheduler的 schedulePod 和 handleSchedulingFailure 方法

### 1.3 schedulerOptions

虽然Scheduler的成员变量包含了很多模块，但是不代表这些模块都是Scheduler构造的，按照惯例很多都是通过参数、选项传入进来的，schedulerOptions就是构造Scheduler的选项类型

```golang
type schedulerOptions struct {
	componentConfigVersion string
	//  kubeconfig 配置
	kubeConfig             *restclient.Config
	// Overridden by profile level percentageOfNodesToScore if set in v1.
	// 这个是调度算法计算最多可行节点的比例阈值
	percentageOfNodesToScore          int32
    // 这两个是调度队列的初始/最大退避时间
	podInitialBackoffSeconds          int64
	podMaxBackoffSeconds              int64
	podMaxInUnschedulablePodsDuration time.Duration
	// Contains out-of-tree plugins to be merged with the in-tree registry.
    // kube-scheduler的插件注册表分为InTree和OutTree两种，前一种就是在调度插件包内静态编译的，后一种是通过选项传入的。
    // frameworkOutOfTreeRegistry就是通过选项传入的插件工厂注册表，当前版本这个选项还是空的，没有用。
    // 但是scheduler-plugin这个兴趣组项目就是采用该选项实现插件的扩展: https://github.com/kubernetes-sigs/scheduler-plugins
    frameworkOutOfTreeRegistry frameworkruntime.Registry
    // 这个是调度框架(Framework)的配置，每个KubeSchedulerProfile对应一个调度框架，本文不会对该类型做详细解析，因为会有单独的文章专门解析kube-scheduler的配置。
    // 此处只需要知道一点：每个KubeSchedulerProfile配置了一个调度框架每个扩展点使用哪些调度插件以及调度插件的参数。
    profiles                   []schedulerapi.KubeSchedulerProfile
	// 调度扩展程序的配置
	extenders                  []schedulerapi.Extender
	// 调度框架捕获器
	frameworkCapturer          FrameworkCapturer
	// 最大并行度，调度算法是多协程过滤、评分，其中最大协程数就是通过该选项配置的
	parallelism                int32
	applyDefaultProfile        bool
}

```

知道了Scheduler的定义，以及构造Scheduler的选项类型，接下来看看kube-scheduler是如何构造Scheduler的

```golang

// New returns a Scheduler
// New()是Scheduler的构造函数，其中clientset,informerFactory,recorderFactory是调用者传入的，所以构造函数不用创建。
// opts就是在默认的schedulerOptions叠加的选项，golang这种用法非常普遍，不需要多解释。
func New(client clientset.Interface,
	informerFactory informers.SharedInformerFactory,
	dynInformerFactory dynamicinformer.DynamicSharedInformerFactory,
	recorderFactory profile.RecorderFactory,
	stopCh <-chan struct{},
	opts ...Option) (*Scheduler, error) {

   // 初始化stopEverything，如果调用者没有指定则永远不会停止(这种情况不可能，因为要优雅退出)。
	stopEverything := stopCh
	if stopEverything == nil {
		stopEverything = wait.NeverStop
	}

	// 在默认的schedulerOptions基础上应用所有的opts，其中defaultSchedulerOptions 建议自己看一下
	options := defaultSchedulerOptions
	for _, opt := range opts {
		opt(&options)
	}

	if options.applyDefaultProfile {
		var versionedCfg configv1.KubeSchedulerConfiguration
		scheme.Scheme.Default(&versionedCfg)
		cfg := schedulerapi.KubeSchedulerConfiguration{}
		if err := scheme.Scheme.Convert(&versionedCfg, &cfg, nil); err != nil {
			return nil, err
		}
		options.profiles = cfg.Profiles
	}


    // 创建InTree的插件工厂注册表，并与OutTree的插件工厂注册表合并，形成最终的插件工厂注册表。
    // registry是一个map类型，键是插件名称，值是插件的工厂(构造函数)
	registry := frameworkplugins.NewInTreeRegistry()
	if err := registry.Merge(options.frameworkOutOfTreeRegistry); err != nil {
		return nil, err
	}

	metrics.Register()
    // 初始化调度扩展
	extenders, err := buildExtenders(options.extenders, options.profiles)
	if err != nil {
		return nil, fmt.Errorf("couldn't build extenders: %w", err)
	}

	// 获取 POD 和 Node Lister
	podLister := informerFactory.Core().V1().Pods().Lister()
	nodeLister := informerFactory.Core().V1().Nodes().Lister()

	// 初始化Cache的快照
	snapshot := internalcache.NewEmptySnapshot()
	clusterEventMap := make(map[framework.ClusterEvent]sets.String)
	metricsRecorder := metrics.NewMetricsAsyncRecorder(1000, time.Second, stopCh)

	// 初始化调度框架 Profiles
	profiles, err := profile.NewMap(options.profiles, registry, recorderFactory, stopCh,
		frameworkruntime.WithComponentConfigVersion(options.componentConfigVersion),
		frameworkruntime.WithClientSet(client),
		frameworkruntime.WithKubeConfig(options.kubeConfig),
		frameworkruntime.WithInformerFactory(informerFactory),
		frameworkruntime.WithSnapshotSharedLister(snapshot),
		frameworkruntime.WithCaptureProfile(frameworkruntime.CaptureProfile(options.frameworkCapturer)),
		frameworkruntime.WithClusterEventMap(clusterEventMap),
		frameworkruntime.WithClusterEventMap(clusterEventMap),
		frameworkruntime.WithParallelism(int(options.parallelism)),
		frameworkruntime.WithExtenders(extenders),
		frameworkruntime.WithMetricsRecorder(metricsRecorder),
	)
	if err != nil {
		return nil, fmt.Errorf("initializing profiles: %v", err)
	}

	if len(profiles) == 0 {
		return nil, errors.New("at least one profile is required")
	}

	preEnqueuePluginMap := make(map[string][]framework.PreEnqueuePlugin)
	for profileName, profile := range profiles {
		preEnqueuePluginMap[profileName] = profile.PreEnqueuePlugins()
	}
	// 初始化调度队列
	podQueue := internalqueue.NewSchedulingQueue(
		profiles[options.profiles[0].SchedulerName].QueueSortFunc(),
		informerFactory,
		internalqueue.WithPodInitialBackoffDuration(time.Duration(options.podInitialBackoffSeconds)*time.Second),
		internalqueue.WithPodMaxBackoffDuration(time.Duration(options.podMaxBackoffSeconds)*time.Second),
		internalqueue.WithPodLister(podLister),
		internalqueue.WithClusterEventMap(clusterEventMap),
		internalqueue.WithPodMaxInUnschedulablePodsDuration(options.podMaxInUnschedulablePodsDuration),
		internalqueue.WithPreEnqueuePluginMap(preEnqueuePluginMap),
		internalqueue.WithPluginMetricsSamplePercent(pluginMetricsSamplePercent),
		internalqueue.WithMetricsRecorder(*metricsRecorder),
	)

	for _, fwk := range profiles {
		fwk.SetPodNominator(podQueue)
	}

	// 构造调度缓存，第一个参数 30秒 是绑定的超时阈值(TTL)，指定时间内没有确认假定调度的Pod将会被从Cache中移除
	schedulerCache := internalcache.New(durationToExpireAssumedPod, stopEverything)

	// Setup cache debugger.
	debugger := cachedebugger.New(nodeLister, podLister, schedulerCache, podQueue)
	debugger.ListenForSignal(stopEverything)

	// 设置 scheduler
	sched := &Scheduler{
		Cache:                    schedulerCache,
		client:                   client,
		nodeInfoSnapshot:         snapshot,
		percentageOfNodesToScore: options.percentageOfNodesToScore,
		Extenders:                extenders,
		NextPod:                  internalqueue.MakeNextPodFunc(podQueue),
		StopEverything:           stopEverything,
		SchedulingQueue:          podQueue,
		Profiles:                 profiles,
	}
	// 设置默认调度算法 和 调度错误回调函数
	sched.applyDefaultHandlers()

	// 注册事件处理函数，就是通过SharedIndexInformer监视Pod、Node、Service等调度依赖的API对象，并根据事件类型执行相应的操作。
	// 其中新建的Pod放入调度队列、Pod绑定成功更新调度缓存都是事件处理函数做的。
	addAllEventHandlers(sched, informerFactory, dynInformerFactory, unionedGVKs(clusterEventMap))

	return sched, nil
}


```

## 2. 调度一个Pod的全流程实现

现在可以开始解析Scheduler调度一个Pod的全流程实现，并且函数名字 scheduleOne 也非常应景，

### 2.1. 调度器运行入口

```golang
// /pkg/scheduler/schedule_one.go
// Run begins watching and scheduling. It starts scheduling and blocked until the context is done.
func (sched *Scheduler) Run(ctx context.Context) {
	// 启动调度队列
	sched.SchedulingQueue.Run()

	// We need to start scheduleOne loop in a dedicated goroutine,
	// because scheduleOne function hangs on getting the next item
	// from the SchedulingQueue.
	// If there are no new pods to schedule, it will be hanging there
	// and if done in this goroutine it will be blocking closing
	// SchedulingQueue, in effect causing a deadlock on shutdown.
	go wait.UntilWithContext(ctx, sched.scheduleOne, 0)

	<-ctx.Done()
	sched.SchedulingQueue.Close()
}
```


### 2.2 scheduleOne()

在调度队列准备运行后， 会执行  sched.scheduleOne 进行调度， 具体代码如下：

```golang
// /pkg/scheduler/schedule_one.go
// scheduleOne()调度一个Pod。
// scheduleOne does the entire scheduling workflow for a single pod. It is serialized on the scheduling algorithm's host fitting.
func (sched *Scheduler) scheduleOne(ctx context.Context) {
    // 获取下一个需要调度Pod，可以理解为从调用ScheduleingQueuePop()，为什么要注入一个函数呢？下面会有注入函数的解析
	podInfo := sched.NextPod()
	// 调度队列关闭的时候返回空的Pod，说明收到了关闭的信号，所以直接退出就行了，不用再判断ctx
	if podInfo == nil || podInfo.Pod == nil {
	    return
	}
	// ...
	// 根据Pod指定的调度器名字(Pod.Spec.SchedulerName)选择Framework。
	pod := podInfo.Pod
	fwk, err := sched.frameworkForPod(pod)
	// ...
    // 是否需要忽略这个Pod，至于什么样的Pod会被忽略，后面有相关函数的注释。
	if sched.skipPodSchedule(fwk, pod) {
		return
	}

	// Synchronously attempt to find a fit for the pod.
	// 为调度Pod做准备，包括计时、创建CycleState以及调度上下文(schedulingCycleCtx)
	start := time.Now()
	state := framework.NewCycleState()
	state.SetRecordPluginMetrics(rand.Intn(100) < pluginMetricsSamplePercent)

	// Initialize an empty podsToActivate struct, which will be filled up by plugins or stay empty.
	podsToActivate := framework.NewPodsToActivate()
	state.Write(framework.PodsToActivateKey, podsToActivate)

	schedulingCycleCtx, cancel := context.WithCancel(ctx)
	defer cancel()
    // 进入调度周期
	scheduleResult, assumedPodInfo, status := sched.schedulingCycle(schedulingCycleCtx, state, fwk, podInfo, start, podsToActivate)
    if !status.IsSuccess() {
        // FailureHandler()用于统一实现调度Pod失败的处理。
        // 也就是我们用kubectl describe pod xxx时，Events部分描述Pod因为什么原因不可调度的，所以参数有错误代码、不可调度原因等就很容易理解了。
        // 需要注意的是，即便抢占成功，Pod当前依然是不可调度状态，因为需要等待被强占的Pod退出，所以nominatedNode是否为空就可以判断是否抢占成功了。
        // 下面有FailureHandler()函数注释，届时会揭开它神秘的面纱。
        sched.FailureHandler(schedulingCycleCtx, fwk, assumedPodInfo, status, scheduleResult.nominatingInfo, start)
        return
    }

    //...
	// bind the pod to its host asynchronously (we can do this b/c of the assumption step above).
	// 进入绑定周期，创建一个协程异步绑定，因为绑定是一个相对比较耗时的操作，至少包含一次向apiserver写入绑定信息的操作。
	go func() {
		bindingCycleCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		// ...
		// 绑定周期
		status := sched.bindingCycle(bindingCycleCtx, state, fwk, scheduleResult, assumedPodInfo, start, podsToActivate)
		if !status.IsSuccess() {
			// 绑定失败处理
			sched.handleBindingCycleError(bindingCycleCtx, state, fwk, assumedPodInfo, start, scheduleResult, status)
		}
	}()
}

```
核心流程如下：
- 调用sched.NextPod()从activeQ中获取一个优先级最高的待调度Pod，该过程是阻塞的，当activeQ中不存在任何Pod资源对象时，sched.NextPod()处于等待状态
- 调用 sched.frameworkForPod 获取 POD 调度框架
- 调用 sched.skipPodSchedule 是否 POD 可以跳过调度
- 调用 sched.schedulingCycle 方法执行调度算法，为Pod选择一个合适的节点
- 调用 sched.bindingCycle 方法进行绑定，为Pod设置NodeName字段

### 2.2. sched.NextPod()

scheduleOne()方法在最开始调用 sched.NextPod() 方法来获取下一个要调度的Pod，就是从 activeQ 活动队列中Pop出来元素，创建Scheduler对象时指定了NextPod函数 internalqueue.MakeNextPodFunc(podQueue)：

```golang
// /pkg/scheduler/internal/queue/scheduling_queue.go
// MakeNextPodFunc returns a function to retrieve the next pod from a given
// scheduling queue
func MakeNextPodFunc(queue SchedulingQueue) func() *framework.QueuedPodInfo {
	return func() *framework.QueuedPodInfo {
		podInfo, err := queue.Pop()
	    //...
		return nil
	}
}
```

这里调用优先级队列的 Pop() 方法来弹出队列中的Pod进行调度处理

```golang
// /pkg/scheduler/internal/queue/scheduling_queue.go
// Pop removes the head of the active queue and returns it. It blocks if the
// activeQ is empty and waits until a new item is added to the queue. It
// increments scheduling cycle when a pod is popped.
func (p *PriorityQueue) Pop() (*framework.QueuedPodInfo, error) {
	p.lock.Lock()
	defer p.lock.Unlock()
	for p.activeQ.Len() == 0 {
		// When the queue is empty, invocation of Pop() is blocked until new item is enqueued.
		// When Close() is called, the p.closed is set and the condition is broadcast,
		// which causes this loop to continue and return from the Pop().
		if p.closed {
			return nil, fmt.Errorf(queueClosed)
		}
		p.cond.Wait()
	}
	obj, err := p.activeQ.Pop()
	if err != nil {
		return nil, err
	}
	pInfo := obj.(*framework.QueuedPodInfo)
	pInfo.Attempts++
	p.schedulingCycle++
	return pInfo, nil
}
```

### 2.3 skipPodSchedule

在调度一个Pod时，第一件事情就是判断Pod是否需要忽略调度，那么有哪些情况需要忽略调度呢？

```golang
// skipPodSchedule returns true if we could skip scheduling the pod for specified cases.
func (sched *Scheduler) skipPodSchedule(fwk framework.Framework, pod *v1.Pod) bool {
	// 第一种情况，Pod被删除了，这个应该好理解，已经删除的Pod不需要要调度。
    // 那么问题来了，是不是可以在Pod更新事件中感知Pod.DeletionTimestamp的变化，然后从调度队列中删除呢？
	// Case 1: pod is being deleted.
	if pod.DeletionTimestamp != nil {
		fwk.EventRecorder().Eventf(pod, nil, v1.EventTypeWarning, "FailedScheduling", "Scheduling", "skip schedule deleting pod: %v/%v", pod.Namespace, pod.Name)
		klog.V(3).InfoS("Skip schedule deleting pod", "pod", klog.KObj(pod))
		return true
	}

	// Case 2: pod that has been assumed could be skipped.
	// An assumed pod can be added again to the scheduling queue if it got an update event
	// during its previous scheduling cycle but before getting assumed.
	// 第二种情况，Pod被更新但是已经假定被调度了，为什么会出现这种情况？
	// 这是深度拷贝的原因，scheduleOne()函数在调用假定调度的时候会深度拷贝Pod，然后设置Pod.Status.NodeName。
	// 在绑定周期，使用的Pod就是假定调度Pod，而不是调度队列中的Pod，此时如果更新了Pod，就会出现这种情况。
	// 因为还没有绑定完成，此时apiserver中Pod依然还是未调度状态，更新Pod势必会将Pod放入调度队列。
	// 放入调度队列就会被再次调度，所以需要跳过它。需要注意的是，并不是所有的更新都会忽略调度。
	// 比如Pod的资源需求、标签、亲和性等调整了，势必会影响调度，而以前调度的结果应该被覆盖，所以不能够忽略。
	isAssumed, err := sched.Cache.IsAssumedPod(pod)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to check whether pod %s/%s is assumed: %v", pod.Namespace, pod.Name, err))
		return false
	}
	return isAssumed
}
```

### 2.4. 获取 Pod 对应调度框架

调用 sched.frameworkForPod 获取 pod.Spec.SchedulerName 对应调度框架

```golang
func (sched *Scheduler) frameworkForPod(pod *v1.Pod) (framework.Framework, error) {
	fwk, ok := sched.Profiles[pod.Spec.SchedulerName]
	if !ok {
		return nil, fmt.Errorf("profile not found for scheduler name %q", pod.Spec.SchedulerName)
	}
	return fwk, nil
}
```

### 2.5 handleSchedulingFailure

前面笔者提到了FailureHandler()至少包含记录调度失败事件的功能，还应该有什么功能？

```golang

// handleSchedulingFailure records an event for the pod that indicates the
// pod has failed to schedule. Also, update the pod condition and nominated node name if set.
func (sched *Scheduler) handleSchedulingFailure(ctx context.Context, fwk framework.Framework, podInfo *framework.QueuedPodInfo, status *framework.Status, nominatingInfo *framework.NominatingInfo, start time.Time) {
	reason := v1.PodReasonSchedulerError
	if status.IsUnschedulable() {
		reason = v1.PodReasonUnschedulable
	}

	switch reason {
	case v1.PodReasonUnschedulable:
		metrics.PodUnschedulable(fwk.ProfileName(), metrics.SinceInSeconds(start))
	case v1.PodReasonSchedulerError:
		metrics.PodScheduleError(fwk.ProfileName(), metrics.SinceInSeconds(start))
	}

	pod := podInfo.Pod
	err := status.AsError()
	errMsg := status.Message()

	// 根据错误代码类型打印日志
	if err == ErrNoNodesAvailable {
		klog.V(2).InfoS("Unable to schedule pod; no nodes are registered to the cluster; waiting", "pod", klog.KObj(pod), "err", err)
	} else if fitError, ok := err.(*framework.FitError); ok {
		// Inject UnschedulablePlugins to PodInfo, which will be used later for moving Pods between queues efficiently.
		podInfo.UnschedulablePlugins = fitError.Diagnosis.UnschedulablePlugins
		klog.V(2).InfoS("Unable to schedule pod; no fit; waiting", "pod", klog.KObj(pod), "err", errMsg)
	} else if apierrors.IsNotFound(err) {
		klog.V(2).InfoS("Unable to schedule pod, possibly due to node not found; waiting", "pod", klog.KObj(pod), "err", errMsg)
		if errStatus, ok := err.(apierrors.APIStatus); ok && errStatus.Status().Details.Kind == "node" {
            // 看看是不是因为未找到Node引起的错误
			nodeName := errStatus.Status().Details.Name
			// when node is not found, We do not remove the node right away. Trying again to get
			// the node and if the node is still not found, then remove it from the scheduler cache.
            // 当找不到Node时，不会立即删除该Node。再次尝试通过apiservers获取该节点，如果仍然找不到该节点，则将其从调度缓存中删除。
			_, err := fwk.ClientSet().CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
			if err != nil && apierrors.IsNotFound(err) {
				node := v1.Node{ObjectMeta: metav1.ObjectMeta{Name: nodeName}}
				if err := sched.Cache.RemoveNode(&node); err != nil {
					klog.V(4).InfoS("Node is not found; failed to remove it from the cache", "node", node.Name)
				}
			}
		}
	} else {
		klog.ErrorS(err, "Error scheduling pod; retrying", "pod", klog.KObj(pod))
	}

	// Check if the Pod exists in informer cache.
	podLister := fwk.SharedInformerFactory().Core().V1().Pods().Lister()
	// 参数pod是从调度队列中获取的，此时需要校验一下SharedIndexInformer的缓存中是否存在，如果不存在说明已经被删除了，也就不用再放回队列了。
	// 其实判断不存在并不是核心目的，只是顺带手的事，核心目的是缓存(非调度缓存)的Pod状态最新的，这样才能将最新的Pod放回到调度队列中。
    // 即便此时判断Pod存在，但是与此同时如果收到了Pod删除事件，那么很可能出现刚刚删除的Pod又被添加到调度队列中。
	cachedPod, e := podLister.Pods(pod.Namespace).Get(pod.Name)
	if e != nil {
		klog.InfoS("Pod doesn't exist in informer cache", "pod", klog.KObj(pod), "err", e)
	} else {
		// In the case of extender, the pod may have been bound successfully, but timed out returning its response to the scheduler.
		// It could result in the live version to carry .spec.nodeName, and that's inconsistent with the internal-queued version.
		// 在Extender的情况下，Pod可能已成功绑定，但是其响应超时返回给Scheduler。这可能会导致实时版本带有Pod.Spec.NodeName，并且与内部排队版本不一致。
		if len(cachedPod.Spec.NodeName) != 0 {
			klog.InfoS("Pod has been assigned to node. Abort adding it back to queue.", "pod", klog.KObj(pod), "node", cachedPod.Spec.NodeName)
		} else {
			// As <cachedPod> is from SharedInformer, we need to do a DeepCopy() here.
			// ignore this err since apiserver doesn't properly validate affinity terms
			// and we can't fix the validation for backwards compatibility.
			// 备份缓存中的Pod，然后将其放回到调度队列中，当然是以不可调度的名义放回的。
			podInfo.PodInfo, _ = framework.NewPodInfo(cachedPod.DeepCopy())
			if err := sched.SchedulingQueue.AddUnschedulableIfNotPresent(podInfo, sched.SchedulingQueue.SchedulingCycle()); err != nil {
				klog.ErrorS(err, "Error occurred")
			}
		}
	}

	// Update the scheduling queue with the nominated pod information. Without
	// this, there would be a race condition between the next scheduling cycle
	// and the time the scheduler receives a Pod Update for the nominated pod.
	// Here we check for nil only for tests.
	// 此处需要考虑两种情况：
	// 1. Pod已经提名Node，现在调度失败了，此时的nominatedNode == ""，AddNominatedPod()会恢复以前的提名
	// 2. Pod抢占调度成功，但是需要等待被强占的Pod退出，所以Pod提名了Node，此时的nominatedNode != ""
	if sched.SchedulingQueue != nil {
		sched.SchedulingQueue.AddNominatedPod(podInfo.PodInfo, nominatingInfo)
	}

	if err == nil {
		// Only tests can reach here.
		return
	}

	msg := truncateMessage(errMsg)
	fwk.EventRecorder().Eventf(pod, nil, v1.EventTypeWarning, "FailedScheduling", "Scheduling", msg)
	if err := updatePod(ctx, sched.client, pod, &v1.PodCondition{
		Type:    v1.PodScheduled,
		Status:  v1.ConditionFalse,
		Reason:  reason,
		Message: errMsg,
	}, nominatingInfo); err != nil {
		klog.ErrorS(err, "Error updating pod", "pod", klog.KObj(pod))
	}
}

```


### 2.6 调度周期

调用 sched.schedulingCycle 进入 POD 调度周期

```golang
// schedulingCycle tries to schedule a single Pod.
func (sched *Scheduler) schedulingCycle(
	ctx context.Context,
	state *framework.CycleState,
	fwk framework.Framework,
	podInfo *framework.QueuedPodInfo,
	start time.Time,
	podsToActivate *framework.PodsToActivate,
) (ScheduleResult, *framework.QueuedPodInfo, *framework.Status) {
	pod := podInfo.Pod
	// 真正执行调度
	scheduleResult, err := sched.SchedulePod(ctx, fwk, state, pod)
	if err != nil {
		// 调度失败， 执行 fwk.RunPostFilterPlugins 尝试抢占
	    // ...
		// SchedulePod() may have failed because the pod would not fit on any host, so we try to
		// preempt, with the expectation that the next time the pod is tried for scheduling it
		// will fit due to the preemption. It is also possible that a different pod will schedule
		// into the resources that were preempted, but this is harmless.

		if !fwk.HasPostFilterPlugins() {
			klog.V(3).InfoS("No PostFilter plugins are registered, so no preemption will be performed")
			return ScheduleResult{}, podInfo, framework.NewStatus(framework.Unschedulable).WithError(err)
		}

        // 运行所有的PostFilterPlugin，尝试让Pod可在以未来调度周期中进行调度。
        // 为什么是未来的调度周期？道理很简单，需要等被强占Pod的退出。
		// Run PostFilter plugins to attempt to make the pod schedulable in a future scheduling cycle.
		result, status := fwk.RunPostFilterPlugins(ctx, state, pod, fitError.Diagnosis.NodeToStatusMap)
		msg := status.Message()
		// ...
		var nominatingInfo *framework.NominatingInfo
		if result != nil {
            // 如果抢占成功，则记录提名Node的名字。
			nominatingInfo = result.NominatingInfo
		}
		return ScheduleResult{nominatingInfo: nominatingInfo}, podInfo, framework.NewStatus(framework.Unschedulable).WithError(err)
	}
	
	// 执行 sched.assume 方法进行预绑定，为Pod设置NodeName字段，更新Scheduler缓存
    // ...
	// Tell the cache to assume that a pod now is running on a given node, even though it hasn't been bound yet.
	// This allows us to keep scheduling without waiting on binding to occur.
	// 深度拷贝PodInfo赋值给假定调度Pod，为什么深度拷贝Pod？因为assume()会设置Pod.Status.NodeName = scheduleResult.SuggestedHost
	assumedPodInfo := podInfo.DeepCopy()
	assumedPod := assumedPodInfo.Pod
	// assume modifies `assumedPod` by setting NodeName=scheduleResult.SuggestedHost
    // assume()会调用Cache.AssumePod()假定调度Pod，assume()函数下面有注释，此处暂时认为Cache.AssumePod()就行了。
    // 需要再解释一下为什么要假定调度Pod，因为Scheduler不用等到绑定结果就可以调度下一个Pod，如果无法理解,建议阅读关于调度缓存的内容。
    err = sched.assume(assumedPod, scheduleResult.SuggestedHost)
	if err != nil {
		// This is most probably result of a BUG in retrying logic.
		// We report an error here so that pod scheduling can be retried.
		// This relies on the fact that Error will check if the pod has been bound
		// to a node and if so will not add it back to the unscheduled pods queue
		// (otherwise this would cause an infinite loop).
		return ScheduleResult{nominatingInfo: clearNominatedNode},
			assumedPodInfo,
			framework.AsStatus(err)
	}
    // 调用fwk.RunReservePluginsReserve()方法运行Reserve插件的Reserve()方法
	// Run the Reserve method of reserve plugins.
	// 为Pod预留需要的全局资源，比如PV
	if sts := fwk.RunReservePluginsReserve(ctx, state, assumedPod, scheduleResult.SuggestedHost); !sts.IsSuccess() {
		// trigger un-reserve to clean up state associated with the reserved Pod
		// 即便预留资源失败了，还是调用一次恢复，可以清理一些状态，认为理论上RunReservePluginsReserve()应该保证一定的事务性
		fwk.RunReservePluginsUnreserve(ctx, state, assumedPod, scheduleResult.SuggestedHost)
        // 因为Cache中已经假定Pod调度了，此处就应该删除假定调度的Pod
		if forgetErr := sched.Cache.ForgetPod(assumedPod); forgetErr != nil {
			klog.ErrorS(forgetErr, "Scheduler cache ForgetPod failed")
		}

		return ScheduleResult{nominatingInfo: clearNominatedNode},
			assumedPodInfo,
			sts
	}
    // 调用fwk.RunPermitPlugins()方法运行Permit插件
	// Run "permit" plugins.
	// 判断Pod是否可以进入绑定阶段。
	runPermitStatus := fwk.RunPermitPlugins(ctx, state, assumedPod, scheduleResult.SuggestedHost)
    // 有插件没有批准Pod并且不是等待，那只能是拒绝或者发生了内部错误
	if !runPermitStatus.IsWait() && !runPermitStatus.IsSuccess() {
		// trigger un-reserve to clean up state associated with the reserved Pod
		// 从此处开始，一旦调度失败，都会做如下几个事情 ：
		// 1. 恢复预留的资源：fwk.RunReservePluginsUnreserve()；
		// 2. 删除假定调度的Pod：sched.SchedulerCache.ForgetPod()；
		fwk.RunReservePluginsUnreserve(ctx, state, assumedPod, scheduleResult.SuggestedHost)
		if forgetErr := sched.Cache.ForgetPod(assumedPod); forgetErr != nil {
			klog.ErrorS(forgetErr, "Scheduler cache ForgetPod failed")
		}

		return ScheduleResult{nominatingInfo: clearNominatedNode},
			assumedPodInfo,
			runPermitStatus
	}
    // 
	// At the end of a successful scheduling cycle, pop and move up Pods if needed.
	if len(podsToActivate.Map) != 0 {
		sched.SchedulingQueue.Activate(podsToActivate.Map)
		// Clear the entries after activation.
		podsToActivate.Map = make(map[string]*v1.Pod)
	}

	return scheduleResult, assumedPodInfo, nil
}
```

核心流程如下：
- 执行 sched.SchedulePod 进行调度
- 调度失败， 执行 fwk.RunPostFilterPlugins 尝试抢占
- 执行 sched.assume 方法进行预绑定，为Pod设置NodeName字段，更新Scheduler缓存
- 调用fwk.RunReservePluginsReserve()方法运行Reserve插件的Reserve()方法
- 调用fwk.RunPermitPlugins()方法运行Permit插件，判断Pod是否可以进入绑定阶段
- 调用 sched.SchedulingQueue.Activate

### 2.7 assume

```golang
// assume signals to the cache that a pod is already in the cache, so that binding can be asynchronous.
// assume modifies `assumed`.
// assume()是Scheduler假定Pod调度的处理函数
func (sched *Scheduler) assume(assumed *v1.Pod, host string) error {
	// Optimistically assume that the binding will succeed and send it to apiserver
	// in the background.
	// If the binding fails, scheduler will release resources allocated to assumed pod
	// immediately.
	// 通知Cache假定调度到'host'指定的Node上
	assumed.Spec.NodeName = host

	if err := sched.Cache.AssumePod(assumed); err != nil {
		klog.ErrorS(err, "Scheduler cache AssumePod failed")
		return err
	}
	// if "assumed" is a nominated pod, we should remove it from internal cache
	// 仅仅通知Cache就完了么？如果Pod被提名了Node呢？因为Pod已经被(假定)调度了，相关的提名就要去掉，因为只有未调度的Pod才能被提名。
	// 调度队列实现了PodNominator，所以通过调度队列删除Pod的提名状态。这就可以理解单独封装assume()函数的目的了。
	if sched.SchedulingQueue != nil {
		sched.SchedulingQueue.DeleteNominatedPodIfExists(assumed)
	}

	return nil
}
```

### 2.8 绑定周期

```golang
// bindingCycle tries to bind an assumed Pod.
func (sched *Scheduler) bindingCycle(
	ctx context.Context,
	state *framework.CycleState,
	fwk framework.Framework,
	scheduleResult ScheduleResult,
	assumedPodInfo *framework.QueuedPodInfo,
	start time.Time,
	podsToActivate *framework.PodsToActivate) *framework.Status {

	assumedPod := assumedPodInfo.Pod

	// Run "permit" plugins.
	// 等待Pod批准通过，如果有PermitPlugin返回等待，Pod就会被放入waitingPodsMap直到所有的PermitPlug批准通过。
	// 好在调度框架帮我们实现了这些功能，此处只需要调用一个接口就全部搞定了。
	if status := fwk.WaitOnPermit(ctx, assumedPod); !status.IsSuccess() {
		return status
	}

	// Run "prebind" plugins.
	// 绑定预处理
	if status := fwk.RunPreBindPlugins(ctx, state, assumedPod, scheduleResult.SuggestedHost); !status.IsSuccess() {
		return status
	}

	// Run "bind" plugins.
	// 执行绑定操作，所谓绑定，就是向apiserver写入Pod的子资源/bind，里面包含有选择的Node。
	// 单独封装bind()函数用意是什么呢？下面有bind()函数的注释，到时候就明白了。
	if status := sched.bind(ctx, fwk, assumedPod, scheduleResult.SuggestedHost, state); !status.IsSuccess() {
		return status
	}

	// Calculating nodeResourceString can be heavy. Avoid it if klog verbosity is below 2.
	klog.V(2).InfoS("Successfully bound pod to node", "pod", klog.KObj(assumedPod), "node", scheduleResult.SuggestedHost, "evaluatedNodes", scheduleResult.EvaluatedNodes, "feasibleNodes", scheduleResult.FeasibleNodes)
	metrics.PodScheduled(fwk.ProfileName(), metrics.SinceInSeconds(start))
	metrics.PodSchedulingAttempts.Observe(float64(assumedPodInfo.Attempts))
	metrics.PodSchedulingDuration.WithLabelValues(getAttemptsLabel(assumedPodInfo)).Observe(metrics.SinceInSeconds(assumedPodInfo.InitialAttemptTimestamp))

	// Run "postbind" plugins.
	// 绑定后处理
	fwk.RunPostBindPlugins(ctx, state, assumedPod, scheduleResult.SuggestedHost)

	// At the end of a successful binding cycle, move up Pods if needed.
	if len(podsToActivate.Map) != 0 {
		sched.SchedulingQueue.Activate(podsToActivate.Map)
		// Unlike the logic in schedulingCycle(), we don't bother deleting the entries
		// as `podsToActivate.Map` is no longer consumed.
	}

	return nil
}
```

### 2.9 绑定

```golang
// bind binds a pod to a given node defined in a binding object.
// The precedence for binding is: (1) extenders and (2) framework plugins.
// We expect this to run asynchronously, so we handle binding metrics internally.
// 将Pod绑定到的给定Node，绑定的优先级是：（1）Extender和（2）BindPlugin。
func (sched *Scheduler) bind(ctx context.Context, fwk framework.Framework, assumed *v1.Pod, targetNode string, state *framework.CycleState) (status *framework.Status) {
	defer func() {
		// finishBinding()函数下面有注释。
		sched.finishBinding(fwk, assumed, targetNode, status)
	}()

    // 优先用Extender来绑定，下面有extendersBinding()的源码注释
	bound, err := sched.extendersBinding(assumed, targetNode)
	if bound {
		return framework.AsStatus(err)
	}
    // 如果所有Extender都没有绑定能力，则用BindPlugin
	return fwk.RunBindPlugins(ctx, state, assumed, targetNode)
}

// TODO(#87159): Move this to a Plugin.
// extendersBinding()调用Extender执行绑定，返回值为是否绑定成功和错误代码。
func (sched *Scheduler) extendersBinding(pod *v1.Pod, node string) (bool, error) {
    // 遍历所有的Extender.
	for _, extender := range sched.Extenders {
		// 不是Extender有绑定能力(extender.IsBinder() == true)就可以绑定任何Pod，
        // 还有一个前提条件就是Pod的资源是Extender管理的才行，比如只申请CPU和内存的Pod用BindPlugin绑定它不香么？
		if !extender.IsBinder() || !extender.IsInterested(pod) {
			continue
		}
        // 调用Extender执行绑定。
		return true, extender.Bind(&v1.Binding{
			ObjectMeta: metav1.ObjectMeta{Namespace: pod.Namespace, Name: pod.Name, UID: pod.UID},
			Target:     v1.ObjectReference{Kind: "Node", Name: node},
		})
	}
	return false, nil
}

// finishBinding()是绑定结束后的处理。
func (sched *Scheduler) finishBinding(fwk framework.Framework, assumed *v1.Pod, targetNode string, status *framework.Status) {
    // 通知Cache绑定结束，
	if finErr := sched.Cache.FinishBinding(assumed); finErr != nil {
		klog.ErrorS(finErr, "Scheduler cache FinishBinding failed")
	}
    // 如果绑定出现错误，则写日志
	if !status.IsSuccess() {
		klog.V(1).InfoS("Failed to bind pod", "pod", klog.KObj(assumed))
		return
	}

    // 记录绑定成功事件
	fwk.EventRecorder().Eventf(assumed, nil, v1.EventTypeNormal, "Scheduled", "Binding", "Successfully assigned %v/%v to %v", assumed.Namespace, assumed.Name, targetNode)
}

```

### 2.10 handleBindingCycleError

绑定失败处理

```golang
func (sched *Scheduler) handleBindingCycleError(
	ctx context.Context,
	state *framework.CycleState,
	fwk framework.Framework,
	podInfo *framework.QueuedPodInfo,
	start time.Time,
	scheduleResult ScheduleResult,
	status *framework.Status) {

	assumedPod := podInfo.Pod
	// trigger un-reserve plugins to clean up state associated with the reserved Pod
	fwk.RunReservePluginsUnreserve(ctx, state, assumedPod, scheduleResult.SuggestedHost)
	if forgetErr := sched.Cache.ForgetPod(assumedPod); forgetErr != nil {
		klog.ErrorS(forgetErr, "scheduler cache ForgetPod failed")
	} else {
		// "Forget"ing an assumed Pod in binding cycle should be treated as a PodDelete event,
		// as the assumed Pod had occupied a certain amount of resources in scheduler cache.
		//
		// Avoid moving the assumed Pod itself as it's always Unschedulable.
		// It's intentional to "defer" this operation; otherwise MoveAllToActiveOrBackoffQueue() would
		// update `q.moveRequest` and thus move the assumed pod to backoffQ anyways.
		if status.IsUnschedulable() {
			defer sched.SchedulingQueue.MoveAllToActiveOrBackoffQueue(internalqueue.AssignedPodDelete, func(pod *v1.Pod) bool {
				return assumedPod.UID != pod.UID
			})
		} else {
			sched.SchedulingQueue.MoveAllToActiveOrBackoffQueue(internalqueue.AssignedPodDelete, nil)
		}
	}

	sched.FailureHandler(ctx, fwk, podInfo, status, clearNominatedNode, start)
}

```


# 插件

插件涉及内容：
1. 宿主应用(Host Application): kube-scheduler就是宿主应用，因为所有插件都是给kube-scheduler使用的。
2. 插件管理器(Plugin Manager): 原理上讲Scheduling Profile(后文会有介绍)充当了插件管理器的角色，因为它可以配置使能/禁止插件的使用。
3. 插件接口(Plugin Interface): kube-scheduler抽象了调度框架（framework），将调度分为不同的阶段(phase)，每个阶段都定义了一种插件接口。
4. 服务接口(Services Interface): kube-scheduler在创建插件的时候传入了FrameworkHandle，就是一种服务接口，插件通过FrameworkHandle可以获取Clientset和SharedInformerFactory，或者从Cache中读取数据。该句柄还将提供列举、批准或拒绝等待的Pod的接口。

[Scheduling Framework](https://github.com/kubernetes/enhancements/blob/master/keps/sig-scheduling/624-scheduling-framework/README.md)

# 插件状态

kube-scheduler中插件是无状态的，如果插件需要存储状态，则依赖外部实现。这就需要宿主应用为插件提供相关的能力，并以服务接口的形式开放给插件，比如插件A想引用插件B的输出数据。kube-scheduler的实现方法略有不同，开放给插件一个变量，该变量会在所有插件接口函数中引用，其实开放服务接口和开放变量本质上是一样的。

现在来看看存储插件状态的类型定义：

```golang
// /pkg/scheduler/framework/cycle_state.go
// StateData is a generic type for arbitrary data stored in CycleState.
type StateData interface {
	// Clone is an interface to make a copy of StateData. For performance reasons,
	// clone should make shallow copies for members (e.g., slices or maps) that are not
	// impacted by PreFilter's optional AddPod/RemovePod methods.
	// StateData就是具有拷贝能力
	Clone() StateData
}

// StateKey is the type of keys stored in CycleState.
// StateKey就是字符串
type StateKey string

// CycleState provides a mechanism for plugins to store and retrieve arbitrary data.
// StateData stored by one plugin can be read, altered, or deleted by another plugin.
// CycleState does not provide any data protection, as all plugins are assumed to be
// trusted.
// Note: CycleState uses a sync.Map to back the storage. It's aimed to optimize for the "write once and read many times" scenarios.
// It is the recommended pattern used in all in-tree plugins - plugin-specific state is written once in PreFilter/PreScore and afterwards read many times in Filter/Score.
// 其中Cycle是什么意思？很简单，就是每调度一个Pod算周期，那么CycleState就是某一周期的状态。官方称之为Pod调度上下文。
type CycleState struct {
	// storage is keyed with StateKey, and valued with StateData.
	// sync.Map 这给插件保存数据。
	storage sync.Map
	// if recordPluginMetrics is true, PluginExecutionDuration will be recorded for this cycle.
	// 和监控相关
	recordPluginMetrics bool
	// SkipFilterPlugins are plugins that will be skipped in the Filter extension point.
	SkipFilterPlugins sets.Set[string]
	// SkipScorePlugins are plugins that will be skipped in the Score extension point.
	SkipScorePlugins sets.Set[string]
}

```

# 插件类型

kube-scheduler将调度抽象为一种框架（framework），关于调度框架的解释请参看<https://kubernetes.io/zh/docs/concepts/scheduling-eviction/scheduling-framework/>，这对于理解本文非常重要，所以建议读者仔细阅读。

现在需要进入插件的定义:

```golang
// Plugin is the parent type for all the scheduling framework plugins.
// 至少知道一个事情，插件有名字，应该是唯一名字，否则出现重复就无法区分彼此了。
type Plugin interface {
	Name() string
}
```

别看Plugin本身没什么功能，那是因为framework为每个阶段定义了相应的插件功能，而这些插件接口都继承了Plugin。

## QueueSortPlugin

### QueueSortPlugin定义

还记得[调度队列](./scheduling_queue.md)中提到过的lessFunc么？引用的就是QueueSortPlugin.Less()接口。QueueSortPlugin插件是调度队列用来按照指定规则排序Pod时使用的，比如按照优先级排序。

```golang
// QueueSortPlugin is an interface that must be implemented by "QueueSort" plugins.
// These plugins are used to sort pods in the scheduling queue. Only one queue sort
// plugin may be enabled at a time.
type QueueSortPlugin interface {
	Plugin
	// Less are used to sort pods in the scheduling queue.
	Less(*QueuedPodInfo, *QueuedPodInfo) bool
}
```

### QueueSortPlugin实现

QueueSortPlugin插件实现只有PrioritySort，按照Pod的优先级从高到底排序。因为只有PrioritySort一种实现，所以没得选择(笔者注：一个插件接口可以有多种实现)。

```golang
// /pkg/scheduler/framework/plugins/queuesort/priority_sort.go
// PrioritySort is a plugin that implements Priority based sorting.
type PrioritySort struct{}

var _ framework.QueueSortPlugin = &PrioritySort{}

// Name returns name of the plugin.
func (pl *PrioritySort) Name() string {
	return Name
}

// Less is the function used by the activeQ heap algorithm to sort pods.
// It sorts pods based on their priority. When priorities are equal, it uses
// PodQueueInfo.timestamp.
// Less()实现了QueueSortPlugin.Less()接口。
func (pl *PrioritySort) Less(pInfo1, pInfo2 *framework.QueuedPodInfo) bool {
    // PodPriority函数获取Pod的优先级Pod.Spec.Priority，如果为空则优先级为0
	p1 := corev1helpers.PodPriority(pInfo1.Pod)
	p2 := corev1helpers.PodPriority(pInfo2.Pod)
   // 值越大优先级越高，如果优先级相同，则创建时间早的优先级高。
	return (p1 > p2) || (p1 == p2 && pInfo1.Timestamp.Before(pInfo2.Timestamp))
}

// New initializes a new plugin and returns it.
func New(_ runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	return &PrioritySort{}, nil
}
```

## PreFilterPlugin

### PreFilterPlugin定义

这里需要简单说明一下FilterPlugin是用来过滤掉无法运行Pod的Node，这个概念在kube-scheduler中曾经被称为‘predicate’。PreFilter不是预过滤，而是过滤前处理，后面章节的PostFilter就是过滤后处理。因为预过滤有过滤的能力，而过滤前处理是为过滤做准备工作，二者意义不同。

过滤前处理主要是为了过滤准备数据，并将数据存储在CycleState中供FilterPlugin使用。

```golang
// PreFilterPlugin is an interface that must be implemented by "PreFilter" plugins.
// These plugins are called at the beginning of the scheduling cycle.
type PreFilterPlugin interface {
	Plugin
	// PreFilter is called at the beginning of the scheduling cycle. All PreFilter
	// plugins must return success or the pod will be rejected. PreFilter could optionally
	// return a PreFilterResult to influence which nodes to evaluate downstream. This is useful
	// for cases where it is possible to determine the subset of nodes to process in O(1) time.
	// When it returns Skip status, returned PreFilterResult and other fields in status are just ignored,
	// and coupled Filter plugin/PreFilterExtensions() will be skipped in this scheduling cycle.

	// state前面已经说明了，用来存储插件状态的，除此之外，PreFilter()的参数只需要Pod。
	// 所以PreFilter()只能计算一些与Pod相关的数据，这也直接的证明了不是预过滤，因为预过滤应该需要Node。
	// PreFilter()处理的数据会存储在state中供FilterPlugin使用，这样的解释符合前处理的定义。
	PreFilter(ctx context.Context, state *CycleState, p *v1.Pod) (*PreFilterResult, *Status)
	// PreFilterExtensions returns a PreFilterExtensions interface if the plugin implements one,
	// or nil if it does not. A Pre-filter plugin can provide extensions to incrementally
	// modify its pre-processed info. The framework guarantees that the extensions
	// AddPod/RemovePod will only be called after PreFilter, possibly on a cloned
	// CycleState, and may call those functions more than once before calling
	// Filter again on a specific node.
    // 过滤前处理扩展接口，PreFilterExtensions类型下面有注释。这里需要思考一个问题：为什么单独返回一个接口而不是在当前的接口扩展函数？
    // 本质上讲，返回一个扩展接口和在当前接口扩展函数是一样的，而这种设计的目的是：
    // 1. 扩展接口可能不是运行在过滤前处理阶段；
    // 2. 扩展接口需要影响到过滤前处理的结果，否则就称不上过滤前处理的扩展接口了；
    // 说的准确一点就是在过滤前处理与过滤处理之间会调用扩展接口更新前处理的数据，然后再为过滤提供依据。
	PreFilterExtensions() PreFilterExtensions
}


// PreFilterExtensions is an interface that is included in plugins that allow specifying
// callbacks to make incremental updates to its supposedly pre-calculated
// state.
// 过滤前处理插件扩展接口
type PreFilterExtensions interface {
	// AddPod is called by the framework while trying to evaluate the impact
	// of adding podToAdd to the node while scheduling podToSchedule.
	// 尝试评估将podToAdd添加到Node后对调度podToSchedule的影响时，会调用这个接口
	AddPod(ctx context.Context, state *CycleState, podToSchedule *v1.Pod, podInfoToAdd *PodInfo, nodeInfo *NodeInfo) *Status
	// RemovePod is called by the framework while trying to evaluate the impact
	// of removing podToRemove from the node while scheduling podToSchedule.
    // 尝试评估从Node删除podToRemove后对调度podToSchedule的影响时，会调用这个接口
	RemovePod(ctx context.Context, state *CycleState, podToSchedule *v1.Pod, podInfoToRemove *PodInfo, nodeInfo *NodeInfo) *Status
}
```

那么问题来了，为什么不在FilterPlugin做PreFilterPlugin的处理，非要在PreFilterPlugin中处理放到CycleState中，然后FilterPlugin再取出来呢？PreFilterPlugin这个插件真的有必要存在么？答案是肯定的，原因有两个：

1. PreFilterExtensions.AddPod()会造成PreFilterPlugin计算的状态更新，所以在FilterPlugin取出的数据并不是PreFilterPlugin计算出来的结果。那什么时候会调用PreFilterExtensions？还记得[PodNominator](./preempt.md)么？他记录了所有抢占还未调度的Pod，调度器在过滤之前需要假设这些Pod已经放置到了指定的Node上，就会调用PreFilterExtensions.AddPod()接口更新状态。
2. 因为PreFilterPlugin可能会有很多个，每个PreFilterPlugin.PreFilter()都有可能出错，如果最后一个插件在执行PreFilterPlugin.PreFilter()处理的时候出错，那么前面所有插件执行的过滤都白做了，所以把所有可能出错的过滤前处理都放在PreFilterPlugin里，这会减少不必要的计算。

### PreFilterPlugin实现

PreFilterPlugin插件的实现以及他们的功能如下：

1. InterPodAffinity: 实现Pod之间的亲和性和反亲和性，InterPodAffinity实现了PreFilterExtensions，因为抢占调度的Pod可能与当前的Pod具有亲和性或者反亲和性；
2. NodePorts: 检查Pod请求的端口在Node是否可用，NodePorts未实现PreFilterExtensions;
3. NodeResourcesFit: 检查Node是否拥有Pod请求的所有资源，NodeResourcesFit未实现PreFilterEtensions;
4. PodTopologySpread: 实现Pod拓扑分布，关于Pod拓扑分布的解释请查看：<https://kubernetes.io/zh/docs/concepts/workloads/pods/pod-topology-spread-constraints/>，PodTopologySpread实现了PreFilterExtensions接口，因为抢占调度的Pod可能会影响Pod的拓扑分布；
5. ServiceAffinity: 检查属于某个服务(Service)的Pod与配置的标签所定义的Node集合是否适配，这个插件还支持将属于某个服务的Pod分散到各个Node，ServiceAffinity实现了PreFilterExtensions接口；
6. VolumeBinding: 检查Node是否有请求的卷，是否可以绑定请求的卷，VolumeBinding未实现PreFilterExtensions接口；

本文选择NodeResourcesFit作为示例分析PreFilterPlugin的实现，因为资源适配绝大部分调度系统都有，更容易理解，

```golang
// Fit is a plugin that checks if a node has sufficient resources.
type Fit struct {
	ignoredResources                sets.String
	ignoredResourceGroups           sets.String
	enableInPlacePodVerticalScaling bool
	handle                          framework.Handle
	resourceAllocationScorer
}


// computePodResourceRequest returns a framework.Resource that covers the largest
// width in each resource dimension. Because init-containers run sequentially, we collect
// the max in each dimension iteratively. In contrast, we sum the resource vectors for
// regular containers since they run simultaneously.
//
// # The resources defined for Overhead should be added to the calculated Resource request sum
//
// Example:
//
// Pod:
//
//	InitContainers
//	  IC1:
//	    CPU: 2
//	    Memory: 1G
//	  IC2:
//	    CPU: 2
//	    Memory: 3G
//	Containers
//	  C1:
//	    CPU: 2
//	    Memory: 1G
//	  C2:
//	    CPU: 1
//	    Memory: 1G
//
// Result: CPU: 3, Memory: 3G

func computePodResourceRequest(pod *v1.Pod) *preFilterState {
	// pod hasn't scheduled yet so we don't need to worry about InPlacePodVerticalScalingEnabled
	// 所有资源需求量与InitContainers的资源需求量取最大值，这个比较好理解，毕竟InitContainers是串行运行的
	// 累加Pod开销
	// 关于Pod开销请查看链接：https://kubernetes.io/zh/docs/concepts/scheduling-eviction/pod-overhead/
	reqs := resource.PodRequests(pod, resource.PodResourcesOptions{})
	result := &preFilterState{}
	result.SetMaxResource(reqs)
	return result
}

// PreFilter invoked at the prefilter extension point.
// PreFilter实现来了PreFilterPlugin.PreFilter()接口
func (f *Fit) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod) (*framework.PreFilterResult, *framework.Status) {
    // 仅仅是在CycleState记录了Pod的资源需求，computePodResourceRequest见下面注释
	// preFilterStateKey = "PreFilter" + "NodeResourcesFit"
	cycleState.Write(preFilterStateKey, computePodResourceRequest(pod))
	return nil, nil
}

```

## FilterPlugin

### FilterPlugin定义

```golang

// FilterPlugin is an interface for Filter plugins. These plugins are called at the
// filter extension point for filtering out hosts that cannot run a pod.
// This concept used to be called 'predicate' in the original scheduler.
// These plugins should return "Success", "Unschedulable" or "Error" in Status.code.
// However, the scheduler accepts other valid codes as well.
// Anything other than "Success" will lead to exclusion of the given host from
// running the pod.
type FilterPlugin interface {
	Plugin
	// Filter is called by the scheduling framework.
	// All FilterPlugins should return "Success" to declare that
	// the given node fits the pod. If Filter doesn't return "Success",
	// it will return "Unschedulable", "UnschedulableAndUnresolvable" or "Error".
	// For the node being evaluated, Filter plugins should look at the passed
	// nodeInfo reference for this particular node's information (e.g., pods
	// considered to be running on the node) instead of looking it up in the
	// NodeInfoSnapshot because we don't guarantee that they will be the same.
	// For example, during preemption, we may pass a copy of the original
	// nodeInfo object that has some pods removed from it to evaluate the
	// possibility of preempting them to schedule the target pod.
	// 判断Pod是否可以调度到Node上
	Filter(ctx context.Context, state *CycleState, pod *v1.Pod, nodeInfo *NodeInfo) *Status
}
```

过滤插件用于过滤不能运行该Pod的Node，

### FilterPlugin实现

FilterPlugin插件的实现以及它们的功能如下：

1. InterPodAffinity: 实现Pod之间的亲和性和反亲和性；
2. NodeAffinity: 实现了Node[选择器](https://kubernetes.io/zh/docs/concepts/scheduling-eviction/assign-pod-node/#nodeselector)和[节点亲和性](https://kubernetes.io/zh/docs/concepts/scheduling-eviction/assign-pod-node/#node-affinity)
3. NodeLabel: 根据配置的标签过滤Node；
4. NodeName: 检查Pod指定的Node名称与当前Node是否匹配；
5. NodePorts: 检查Pod请求的端口在Node是否可用；
6. NodeResourcesFit: 检查Node是否拥有Pod请求的所有资源；
7. NodeUnscheduleable: 过滤Node.Spec.Unschedulable值为true的Node；
8. NodeVolumeLimits: 检查Node是否满足CSI卷限制；
9. PodTopologySpread: 实现Pod拓扑分布;
10. ServiceAffinity: 检查属于某个服务(Service)的Pod与配置的标签所定义的Node集合是否适配，这个插件还支持将属于某个服务的Pod分散到各个Node；
11. TaintToleration: 实现了[污点和容忍度](https://kubernetes.io/zh/docs/concepts/scheduling-eviction/taint-adntoleration/)；
12. VolumeBinding: 检查Node是否有请求的卷，是否可以绑定请求的卷；
13. VolumeRestrictions: 检查挂载到Node上的卷是否满足卷Provider的限制；
14. VolumeZone: 检查请求的卷是否在任何区域都满足；

实现FilterPlugin的插件数量远多于PreFilterPlugin，说明有不少插件的前处理都放在FilterPlugin实现。需要PreFilterPlugin的观点，就是有些过滤的前处理不会出错亦或抢占调度的Pod对于当前Pod没有影响，比如NodeUnscheduleable。
本文选择NodeResourcesFit作为示例，这样可以与PreFilterPlugin形成连续性。

```golang
// /pkg/scheduler/framework/plugins/noderesources/fit.go
// Filter invoked at the filter extension point.
// Checks if a node has sufficient resources, such as cpu, memory, gpu, opaque int resources etc to run a pod.
// It returns a list of insufficient resources, if empty, then the node has all the resources requested by the pod.
// Filter实现了FilterPlugin.Filter()接口
func (f *Fit) Filter(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
   // 获取PreFilter()计算的状态
	s, err := getPreFilterState(cycleState)
	if err != nil {
		return framework.AsStatus(err)
	}

    // 从函数的返回值的名字可推断出来函数在计算有哪些资源无法满足Pod需求。
    // fitsRequest建议读者自己查看
    // 就是判断Node上剩余的资源是否满足Pod的需求，需要注意每个Node允许调度的Pod数量也是有限制的。
	insufficientResources := fitsRequest(s, nodeInfo, f.ignoredResources, f.ignoredResourceGroups)

	if len(insufficientResources) != 0 {
		// We will keep all failure reasons.
		// 记录所有失败的原因，即因为那些资源造成Pod无法调度到Node上
		failureReasons := make([]string, 0, len(insufficientResources))
		for i := range insufficientResources {
			failureReasons = append(failureReasons, insufficientResources[i].Reason)
		}
		return framework.NewStatus(framework.Unschedulable, failureReasons...)
	}
	return nil
}

```

## PostFilterPlugin

### PostFilterPlugin定义

PostFilterPlugin插件在过滤后调用，但仅在Pod没有满足需求的Node时调用。典型的过滤后处理的实现是抢占，试图通过抢占其他Pod的资源使该Pod可以调度。

```golang
// PostFilterPlugin is an interface for "PostFilter" plugins. These plugins are called
// after a pod cannot be scheduled.
type PostFilterPlugin interface {
	Plugin
	// PostFilter is called by the scheduling framework.
	// A PostFilter plugin should return one of the following statuses:
	// - Unschedulable: the plugin gets executed successfully but the pod cannot be made schedulable.
	// - Success: the plugin gets executed successfully and the pod can be made schedulable.
	// - Error: the plugin aborts due to some internal error.
	//
	// Informational plugins should be configured ahead of other ones, and always return Unschedulable status.
	// Optionally, a non-nil PostFilterResult may be returned along with a Success status. For example,
	// a preemption plugin may choose to return nominatedNodeName, so that framework can reuse that to update the
	// preemptor pod's .spec.status.nominatedNodeName field.
	// 这里有一个问题：为什么参数只有Pod而没有Node？道理很简单，所有的Node都已经被过滤掉了。
	// 所以只需要传入每个Node失败的原因，这样能有助于抢占调度的实现。
	PostFilter(ctx context.Context, state *CycleState, pod *v1.Pod, filteredNodeStatusMap NodeToStatusMap) (*PostFilterResult, *Status)
}
```
### PostFilterPlugin实现
过滤后处理只有DefaultPreemption一种实现，用来实现抢占调度。

```golang
// /pkg/scheduler/framework/plugins/defaultpreemption/default_preemption.go
// PostFilter invoked at the postFilter extension point.
// PostFilter实现了PostFilterPlugin.PostFilter()接口
func (pl *DefaultPreemption) PostFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, m framework.NodeToStatusMap) (*framework.PostFilterResult, *framework.Status) {
	defer func() {
		metrics.PreemptionAttempts.Inc()
	}()

	pe := preemption.Evaluator{
		PluginName: names.DefaultPreemption,
		Handler:    pl.fh,
		PodLister:  pl.podLister,
		PdbLister:  pl.pdbLister,
		State:      state,
		Interface:  pl,
	}

   // preempt()会返回合适的Node的名字，参数'm'记录了每个Node被过滤掉的原因，这样可以结合Node上已经运行Pod，就可以找到在哪个Node上抢占哪些Pod。
	result, status := pe.Preempt(ctx, pod, m)
   // 如果没有找到合适的Node，则返回不可调度状态码
	if status.Message() != "" {
		return result, framework.NewStatus(status.Code(), "preemption: "+status.Message())
	}
	return result, status
}

```

抢占调度是一个比较复杂的过程，会单独分析DefaultPreemption的实现，上面截取的源码只是为了说明PostFilterPlugin的一个也是唯一一个实现是DefaultPreemption。


## PreScorePlugin

### PreScorePlugin定义

```golang
// PreScorePlugin is an interface for "PreScore" plugin. PreScore is an
// informational extension point. Plugins will be called with a list of nodes
// that passed the filtering phase. A plugin may use this data to update internal
// state or to generate logs/metrics.
type PreScorePlugin interface {
	Plugin
	// PreScore is called by the scheduling framework after a list of nodes
	// passed the filtering phase. All prescore plugins must return success or
	// the pod will be rejected
	// When it returns Skip status, other fields in status are just ignored,
	// and coupled Score plugin will be skipped in this scheduling cycle.
	PreScore(ctx context.Context, state *CycleState, pod *v1.Pod, nodes []*v1.Node) *Status
}

```

同样的道理，PreScorePlugin也是有必要的，因为有一些前处理可能会出错，而评分计算可能计算量比较大，如果去掉PreScorePlugin可能会造成大量的无效计算。
例如：总共有10个插件，直到第十个插件在执行预处理的过程中出错，造成前面9个插件的评分计算无效。

### PreScorePlugin实现

PreScorePlugin插件的实现与它们的功能如下：

1. InterPodAffinity: 实现Pod之间的亲和性和反亲和性；
2. PodTopologySpread: 实现Pod拓扑分布；
3. SelectorSpread: 对于属于Services、ReplicaSets和StatefulSets的Pod，偏好跨多节点部署；
4. TaintToleration: 实现了[污点和容忍度](https://kubernetes.io/zh/docs/concepts/scheduling-eviction/taint-adntoleration/)；

本文选择的示例是TaintToleration

```golang
// PreScore builds and writes cycle state used by Score and NormalizeScore.
// PreScore实现了PreScorePlugin.PreScore()接口。
func (pl *TaintToleration) PreScore(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod, nodes []*v1.Node) *framework.Status {
	if len(nodes) == 0 {
		return nil
	}
    // 获取所有PreferNoSchedule类型的污点容忍度，容忍度是Pod指定的。getAllTolerationPreferNoSchedule()下面有注释。
    // 为什么没有NoSchedule和NoExecute？因为这两个是在FilterPlugin使用的，用来过滤无法容忍污点的Node。
    // 当前阶段的Node要么没有NoSchedule和NoExecute，要么能够容忍，所以只需要PreferNoSchedule类型的容忍度。
	tolerationsPreferNoSchedule := getAllTolerationPreferNoSchedule(pod.Spec.Tolerations)
	state := &preScoreState{
		tolerationsPreferNoSchedule: tolerationsPreferNoSchedule,
	}
    // 写入CycleState
	cycleState.Write(preScoreStateKey, state)
	return nil
}

// getAllTolerationEffectPreferNoSchedule gets the list of all Tolerations with Effect PreferNoSchedule or with no effect.
func getAllTolerationPreferNoSchedule(tolerations []v1.Toleration) (tolerationList []v1.Toleration) {
	for _, toleration := range tolerations {
		// Empty effect means all effects which includes PreferNoSchedule, so we need to collect it as well.
		// 有一种特殊的情况，就是容忍度的Effect为空，只需要匹配Key就可以，至于污点的Effect是什么都可以，包括PreferNoSchedule，所以需要考虑这种情况
		if len(toleration.Effect) == 0 || toleration.Effect == v1.TaintEffectPreferNoSchedule {
			tolerationList = append(tolerationList, toleration)
		}
	}
	return
}

```

## ScorePlugin

### ScorePlugin定义

kube-scheduler会调用ScorePlugin对通过FilterPlugin的Node评分，所有ScorePlugin的评分都有一个明确的整数范围，比如[0, 100]，这个过程称之为标准化评分。在标准化评分之后，kube-scheduler将根据配置的插件权重合并所有插件的Node评分得出Node的最终评分。根据Node的最终评分对Node进行排序，得分最高者就是最合适Pod的Node。

```golang
// ScorePlugin is an interface that must be implemented by "Score" plugins to rank
// nodes that passed the filtering phase.
type ScorePlugin interface {
	Plugin
	// Score is called on each filtered node. It must return success and an integer
	// indicating the rank of the node. All scoring plugins must return success or
	// the pod will be rejected.
	// 计算节点的评分，此时需要注意的是参数Node名字，而不是Node对象。
	// 如果实现了PreScorePlugin就从CycleState获取状态， 如果没实现，调度框架在创建插件的时候传入了句柄，可以获取指定的Node。
	// 返回值的评分是一个64位整数，是一个由插件自定义实现取值范围的分数。
	Score(ctx context.Context, state *CycleState, p *v1.Pod, nodeName string) (int64, *Status)

	// ScoreExtensions returns a ScoreExtensions interface if it implements one, or nil if does not.
    // 返回ScoreExtensions接口，此类设计与PreFilterPlugin相似
	ScoreExtensions() ScoreExtensions
}

// ScoreExtensions is an interface for Score extended functionality.
type ScoreExtensions interface {
	// NormalizeScore is called for all node scores produced by the same plugin's "Score"
	// method. A successful run of NormalizeScore will update the scores list and return
	// a success status.
	// ScorePlugin().Score()返回的分数没有任何约束，但是多个ScorePlugin之间需要标准化分数范围，否则无法合并分数。
	// 比如ScorePluginA的分数范围是[0, 10]，ScorePluginB的分数范围是[0, 100]，那么ScorePluginA的分数再高对于ScorePluginB的影响也是非常有限的。
	NormalizeScore(ctx context.Context, state *CycleState, p *v1.Pod, scores NodeScoreList) *Status
}
```

### ScorePlugin实现


1. ImageLocality: 选择已经存在Pod运行所需容器镜像的Node，这样可以省去下载镜像的过程，对于镜像非常大的容器是一个非常有价值的特性，因为启动时间可以节约几秒甚至是几十秒；
2. InterPodAffinity: 实现Pod之间的亲和性和反亲和性；
3. NodeAffinity: 实现了Node[选择器](https://kubernetes.io/zh/docs/concepts/scheduling-eviction/assign-pod-node/#nodeselector)和[节点亲和性](https://kubernetes.io/zh/docs/concepts/scheduling-eviction/assign-pod-node/#node-affinity)
4. NodeLabel: 根据配置的标签过滤Node；
5. NodePreferAvoidPods: 基于Node的[注解](https://kubernetes.io/zh/docs/concepts/overview/working-with-objects/annotations/) **scheduler.alpha.kubernetes.io/preferAvoidPods**打分；
6. NodeResourcesBalancedAllocation: 调度Pod时，选择资源分配更为均匀的Node；
7. NodeResourcesLeastAllocation: 调度Pod时，选择资源分配较少的Node；
8. NodeResourcesMostAllocation: 调度Pod时，选择资源分配较多的Node；
9. RequestedToCapacityRatio: 根据已分配资源的配置函数选择偏爱Node；
10. PodTopologySpread: 实现Pod拓扑分布；
11. SelectorSpread: 对于属于Services、ReplicaSets和StatefulSets的Pod，偏好跨多节点部署；
12. ServiceAffinity: 检查属于某个服务(Service)的Pod与配置的标签所定义的Node集合是否适配，这个插件还支持将属于某个服务的Pod分散到各个Node；
13. TaintToleration: 实现了[污点和容忍度](https://kubernetes.io/zh/docs/concepts/scheduling-eviction/taint-adntoleration/)；

为了保持与PreScorePlugin实现代码的连续性，本文选择的示例是TaintToleration

```golang
// Score invoked at the Score extension point.
// Score()实现了ScorePlugin.Score()接口。
func (pl *TaintToleration) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
    // 获得NodeInfo，此处就是通过调度框架提供的句柄handle获取指定的Node
	nodeInfo, err := pl.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return 0, framework.AsStatus(fmt.Errorf("getting node %q from Snapshot: %w", nodeName, err))
	}
	node := nodeInfo.Node()

	// 获取PreScore()计算的状态
	s, err := getPreScoreState(state)
	if err != nil {
		return 0, framework.AsStatus(err)
	}

    // 建议充分阅读https://kubernetes.io/zh/docs/concepts/scheduling-eviction/taint-and-toleration，理解污点和容忍度，这会影响下面代码的理解。
    // 统计无法容忍的污点数量，从代码上看无法容忍的数量越多分数越高，这不是有问题么？无法容忍就代表尽量不要把Pod放置到Node上，应该分数更低才对啊。
    // 这就要继续看ScoreExtensions部分的实现了
	score := int64(countIntolerableTaintsPreferNoSchedule(node.Spec.Taints, s.tolerationsPreferNoSchedule))
	return score, nil
}


// NormalizeScore invoked after scoring all nodes.
// NormalizeScore()实现了ScoreExtensions.NormalizeScore()接口。
func (pl *TaintToleration) NormalizeScore(ctx context.Context, _ *framework.CycleState, pod *v1.Pod, scores framework.NodeScoreList) *framework.Status {
    // pluginhelper.DefaultNormalizeScore()源码注释翻译如下：
    // 1. 生成可将分数标准化为[0, maxPriority(第一个参数)]的标准化分数，framework.MaxNodeScore为100，所以标准化分数范围是[0, 100]。
    // 2. 如果将reverse(第二个参数)设置为true，则通过maxPriority减去分数来翻转分数。
    // 因为在TaintToleration.NormalizeScore()翻转了分数，所以无法容忍的污点数量越多标准化分数越低
	return helper.DefaultNormalizeScore(framework.MaxNodeScore, true, scores)
}

// ScoreExtensions of the Score plugin.
func (pl *TaintToleration) ScoreExtensions() framework.ScoreExtensions {
	return pl
}

// CountIntolerableTaintsPreferNoSchedule gives the count of intolerable taints of a pod with effect PreferNoSchedule
func countIntolerableTaintsPreferNoSchedule(taints []v1.Taint, tolerations []v1.Toleration) (intolerableTaints int) {
	for _, taint := range taints {
		// check only on taints that have effect PreferNoSchedule
		if taint.Effect != v1.TaintEffectPreferNoSchedule {
			continue
		}
        // toleration 和 taint 不匹配
		if !v1helper.TolerationsTolerateTaint(tolerations, &taint) {
			intolerableTaints++
		}
	}
	return
}
```

从TaintToleration示例可以看出ScorePlugin.Score()返回的分数是一个由插件自定义的值，甚至是翻转值，这给插件实现比较大的自由度。

## ReservePlugin

### ReservePlugin定义

kube-scheduler实际将Pod绑定到其指定的Node之前调用ReservePlugin，它的存在是为了防止kube-scheduler在等待绑定的成功前出现争用的情况(因为绑定是异步执行的，调度下一个Pod可能发生在绑定完成之前)。所以ReservePlugin是维护运行时状态的插件，也称之为有状态插件。这里需要说明一点，此处的状态不是插件的状态，而是全局的状态，只是再调度过程中由某些插件维护而已。

```golang
// ReservePlugin is an interface for plugins with Reserve and Unreserve
// methods. These are meant to update the state of the plugin. This concept
// used to be called 'assume' in the original scheduler. These plugins should
// return only Success or Error in Status.code. However, the scheduler accepts
// other valid codes as well. Anything other than Success will lead to
// rejection of the pod.
type ReservePlugin interface {
	Plugin
	// Reserve is called by the scheduling framework when the scheduler cache is
	// updated. If this method returns a failed Status, the scheduler will call
	// the Unreserve method for all enabled ReservePlugins.
	// 在绑定Pod到Node之前为Pod预留资源
	Reserve(ctx context.Context, state *CycleState, p *v1.Pod, nodeName string) *Status
	// Unreserve is called by the scheduling framework when a reserved pod was
	// rejected, an error occurred during reservation of subsequent plugins, or
	// in a later phase. The Unreserve method implementation must be idempotent
	// and may be called by the scheduler even if the corresponding Reserve
	// method for the same plugin was not called.
    // 在Reserve()与绑定成功之间有任何错误恢复预留的资源
	Unreserve(ctx context.Context, state *CycleState, p *v1.Pod, nodeName string)
}

```

### ReservePlugin实现

ReservePlugin的实现只有VolumeBinding，用来实现PVC和PV绑定，众所周知PV是kubernetes的全局资源，不是某个插件自己的状态。关于PVC和PV的概念请查看链接：<https://kubernetes.io/zh/docs/concepts/storage/persistent-volumes/>。

如果Pod声明了PVC，调度过程中需要找到匹配的PV，然后再完成绑定。想一想为什么只有PV，Node的CPU、内存什么的为什么不预留？因为这些是通过[调度缓存](./cache)实现的预留。

因为只有VolumeBinding一个实现

```golang
// /pkg/scheduler/framework/plugins/volumebinding/volume_binding.go
// Reserve reserves volumes of pod and saves binding status in cycle state.
// Reserve()实现了ReservePlugin.Reserve()接口。
func (pl *VolumeBinding) Reserve(ctx context.Context, cs *framework.CycleState, pod *v1.Pod, nodeName string) *framework.Status {
    // 获取存储在CycleState的状态，也就是Pod的PVC声明
	state, err := getStateData(cs)
	if err != nil {
		return framework.AsStatus(err)
	}
	// we don't need to hold the lock as only one node will be reserved for the given pod
	podVolumes, ok := state.podVolumesByNode[nodeName]
	if ok {
        // AssumePodVolumes()会匹配未绑定的PVC和PV，假定PVC和PV绑定并更新PV缓存，这一点和调度缓存类似
		allBound, err := pl.Binder.AssumePodVolumes(pod, nodeName, podVolumes)
		if err != nil {
			return framework.AsStatus(err)
		}
		state.allBound = allBound
	} else {
		// may not exist if the pod does not reference any PVC
		state.allBound = true
	}
	return nil
}

// Unreserve clears assumed PV and PVC cache.
// It's idempotent, and does nothing if no cache found for the given pod.
// Unreserve()实现了ReservePlugin.Unreserve()接口。
func (pl *VolumeBinding) Unreserve(ctx context.Context, cs *framework.CycleState, pod *v1.Pod, nodeName string) {
    // 获取存储在CycleState中的状态
	s, err := getStateData(cs)
	if err != nil {
		return
	}
	// we don't need to hold the lock as only one node may be unreserved
	podVolumes, ok := s.podVolumesByNode[nodeName]
	if !ok {
		return
	}
    // 恢复假定绑定的PVC和PV
	pl.Binder.RevertAssumedPodVolumes(podVolumes)
}

```

其实VolumeBinding不仅实现了实现了ReservePlugin，它还实现的插件和功能如下：

1. PreFilterPlugin: 在CycleState记录Pod的PVC；
2. FilterPlugin: 判断Node是否能够满足Pod的PVC；
3. ReservePlugin: 为Pod预留PV，亦或称之为假定(assume)绑定PVC和PV，此处的假定与Pod假定调度到Node类似；
4. PreBindPlugin: 执行PV和PVC绑定

## PermitPlugin 

### PermitPlugin定义

PermitPlugin插件用于阻止或延迟Pod绑定，可以做以下三个事情之一：

1. approve: 所有PermitPlugin都批准了Pod后，Pod才可以绑定；
2. deny: 如果任何PermitPlugin拒绝Pod，将返回到调度队列，这将触发ReservePlugin.Unreserve()；
3. wait(带超时): 如果PermitPlugin返回‘wait’，则Pod将保持在许可阶段，直到插件批准它为止；如果超时，等待将变成拒绝，并将Pod返回到调度队列，同时触发ReservePlugin.Unreserve()；

PermitPlugin是在调度周期(调度框架将调度过程分为调度周期和绑定周期，详情参看调度框架)的最后一步执行，但是在许可阶段中的等待发生在绑定周期的开始，即PreBind插件执行之前。

```golang
// PermitPlugin is an interface that must be implemented by "Permit" plugins.
// These plugins are called before a pod is bound to a node.
type PermitPlugin interface {
	Plugin
	// Permit is called before binding a pod (and before prebind plugins). Permit
	// plugins are used to prevent or delay the binding of a Pod. A permit plugin
	// must return success or wait with timeout duration, or the pod will be rejected.
	// The pod will also be rejected if the wait timeout or the pod is rejected while
	// waiting. Note that if the plugin returns "wait", the framework will wait only
	// after running the remaining plugins given that no other plugin rejects the pod.
	Permit(ctx context.Context, state *CycleState, p *v1.Pod, nodeName string) (*Status, time.Duration)
}
```

kubernetes没有实现PermitPlugin.

## PreBindPlugin

### PreBindPlugin定义

PreBindPlugin插件用于执行绑定Pod之前所需要的工作，例如：可以设置网络卷并将其安装在目标Node上，然后在允许Pod在节点上运行。

```golang
// PreBindPlugin is an interface that must be implemented by "PreBind" plugins.
// These plugins are called before a pod being scheduled.
type PreBindPlugin interface {
	Plugin
	// PreBind is called before binding a pod. All prebind plugins must return
	// success or the pod will be rejected and won't be sent for binding.
	PreBind(ctx context.Context, state *CycleState, p *v1.Pod, nodeName string) *Status
}

```

### PreBindPlugin实现

PreBindPlugin的实现只有VolumeBinding，用来实现PVC和PV绑定，关于PVC和PV的概念请查看链接：<https://kubernetes.io/zh/docs/concepts/storage/persistent-volumes/>。

只有VolumeBinding一种实现

```golang

// PreBind will make the API update with the assumed bindings and wait until
// the PV controller has completely finished the binding operation.
//
// If binding errors, times out or gets undone, then an error will be returned to
// retry scheduling.
// PreBind()实现了PreBindPlugin.PreBind()接口
func (pl *VolumeBinding) PreBind(ctx context.Context, cs *framework.CycleState, pod *v1.Pod, nodeName string) *framework.Status {
    // 获取存储在CycleState的状态
	s, err := getStateData(cs)
	if err != nil {
		return framework.AsStatus(err)
	}
    // 如果所有的PVC和PV已经绑定完成直接OK
	if s.allBound {
		// no need to bind volumes
		return nil
	}
	// we don't need to hold the lock as only one node will be pre-bound for the given pod
	podVolumes, ok := s.podVolumesByNode[nodeName]
	if !ok {
		return framework.AsStatus(fmt.Errorf("no pod volumes found for node %q", nodeName))
	}
	klog.V(5).InfoS("Trying to bind volumes for pod", "pod", klog.KObj(pod))
    // 绑定PVC和PV
	err = pl.Binder.BindPodVolumes(ctx, pod, podVolumes)
	if err != nil {
		klog.V(1).InfoS("Failed to bind volumes for pod", "pod", klog.KObj(pod), "err", err)
		return framework.AsStatus(err)
	}
	klog.V(5).InfoS("Success binding volumes for pod", "pod", klog.KObj(pod))
	return nil
}
```

## BindPlugin

### BindPlugin定义

BindPlugin用于将Pod绑定到Node，在所有PreBind完成之前，不会调用BindPlugin。按照BindPlugin配置顺序调用，BindPlugin可以选择是否处理指定的Pod，如果选择处理Pod，则会跳过其余的插件。


```golang
// BindPlugin is an interface that must be implemented by "Bind" plugins. Bind
// plugins are used to bind a pod to a Node.
type BindPlugin interface {
	Plugin
	// Bind plugins will not be called until all pre-bind plugins have completed. Each
	// bind plugin is called in the configured order. A bind plugin may choose whether
	// or not to handle the given Pod. If a bind plugin chooses to handle a Pod, the
	// remaining bind plugins are skipped. When a bind plugin does not handle a pod,
	// it must return Skip in its Status code. If a bind plugin returns an Error, the
	// pod is rejected and will not be bound.
	Bind(ctx context.Context, state *CycleState, p *v1.Pod, nodeName string) *Status
}
```

### BindPlugin实现

BindPlugin的实现只有DefaultBinder一种.

```golang
// Bind binds pods to nodes using the k8s client.
// Bind()实现了BindPlugin.Bind()接口。
func (b DefaultBinder) Bind(ctx context.Context, state *framework.CycleState, p *v1.Pod, nodeName string) *framework.Status {
	logger := klog.FromContext(ctx)
	logger.V(3).Info("Attempting to bind pod to node", "pod", klog.KObj(p), "node", klog.KRef("", nodeName))
	binding := &v1.Binding{
		ObjectMeta: metav1.ObjectMeta{Namespace: p.Namespace, Name: p.Name, UID: p.UID},
		Target:     v1.ObjectReference{Kind: "Node", Name: nodeName},
	}
    // 利用Clientset提供的接口完成绑定。
	err := b.handle.ClientSet().CoreV1().Pods(binding.Namespace).Bind(ctx, binding, metav1.CreateOptions{})
	if err != nil {
		return framework.AsStatus(err)
	}
	return nil
}

```

## PostBindPlugin

### PostBindPlugin定义

成功绑定Pod后，将调用PostBindPlugin(比如用来清理关联的资源），绑定周期到此结束。

```golang
// PostBindPlugin is an interface that must be implemented by "PostBind" plugins.
// These plugins are called after a pod is successfully bound to a node.
type PostBindPlugin interface {
	Plugin
	// PostBind is called after a pod is successfully bound. These plugins are
	// informational. A common application of this extension point is for cleaning
	// up. If a plugin needs to clean-up its state after a pod is scheduled and
	// bound, PostBind is the extension point that it should register.
	PostBind(ctx context.Context, state *CycleState, p *v1.Pod, nodeName string)
}

```

kubernetes没有实现PostBindPlugin.

# 插件注册表

所有的插件统一注册在runtime.Registry的对象中 

```golang
// /pkg/scheduler/framework/runtime/registry.go
// PluginFactory is a function that builds a plugin.
// 插件工厂用来创建插件，创建插件需要输入插件配置参数和框架句柄，其中runtime.Object可以想象为interface{}，可以是结构。
// framework.Handle就是调度框架句柄，为插件开放的接口
type PluginFactory = func(configuration runtime.Object, f framework.Handle) (framework.Plugin, error)

// Registry is a collection of all available plugins. The framework uses a
// registry to enable and initialize configured plugins.
// All plugins must be in the registry before initializing the framework.
// 插件注册表就是插件名字与插件工厂的映射，也就是可以根据插件名字创建插件。
type Registry map[string]PluginFactory

// Register adds a new plugin to the registry. If a plugin with the same name
// exists, it returns an error.
func (r Registry) Register(name string, factory PluginFactory) error {
	if _, ok := r[name]; ok {
		return fmt.Errorf("a plugin named %v already exists", name)
	}
	r[name] = factory
	return nil
}

// Unregister removes an existing plugin from the registry. If no plugin with
// the provided name exists, it returns an error.
func (r Registry) Unregister(name string) error {
	if _, ok := r[name]; !ok {
		return fmt.Errorf("no plugin named %v exists", name)
	}
	delete(r, name)
	return nil
}
```

kube-scheuler插件注册表是静态编译的， 

```golang
// /pkg/scheduler/framework/plugins/registry.go
// NewInTreeRegistry builds the registry with all the in-tree plugins.
// A scheduler that runs out of tree plugins can register additional plugins
// through the WithFrameworkOutOfTreeRegistry option.
func NewInTreeRegistry() runtime.Registry {
	fts := plfeature.Features{
		EnableDynamicResourceAllocation:              feature.DefaultFeatureGate.Enabled(features.DynamicResourceAllocation),
		EnableReadWriteOncePod:                       feature.DefaultFeatureGate.Enabled(features.ReadWriteOncePod),
		EnableVolumeCapacityPriority:                 feature.DefaultFeatureGate.Enabled(features.VolumeCapacityPriority),
		EnableMinDomainsInPodTopologySpread:          feature.DefaultFeatureGate.Enabled(features.MinDomainsInPodTopologySpread),
		EnableNodeInclusionPolicyInPodTopologySpread: feature.DefaultFeatureGate.Enabled(features.NodeInclusionPolicyInPodTopologySpread),
		EnableMatchLabelKeysInPodTopologySpread:      feature.DefaultFeatureGate.Enabled(features.MatchLabelKeysInPodTopologySpread),
		EnablePodSchedulingReadiness:                 feature.DefaultFeatureGate.Enabled(features.PodSchedulingReadiness),
		EnablePodDisruptionConditions:                feature.DefaultFeatureGate.Enabled(features.PodDisruptionConditions),
		EnableInPlacePodVerticalScaling:              feature.DefaultFeatureGate.Enabled(features.InPlacePodVerticalScaling),
	}

	registry := runtime.Registry{
		dynamicresources.Name:                runtime.FactoryAdapter(fts, dynamicresources.New),
		selectorspread.Name:                  selectorspread.New,
		imagelocality.Name:                   imagelocality.New,
		tainttoleration.Name:                 tainttoleration.New,
		nodename.Name:                        nodename.New,
		nodeports.Name:                       nodeports.New,
		nodeaffinity.Name:                    nodeaffinity.New,
		podtopologyspread.Name:               runtime.FactoryAdapter(fts, podtopologyspread.New),
		nodeunschedulable.Name:               nodeunschedulable.New,
		noderesources.Name:                   runtime.FactoryAdapter(fts, noderesources.NewFit),
		noderesources.BalancedAllocationName: runtime.FactoryAdapter(fts, noderesources.NewBalancedAllocation),
		volumebinding.Name:                   runtime.FactoryAdapter(fts, volumebinding.New),
		volumerestrictions.Name:              runtime.FactoryAdapter(fts, volumerestrictions.New),
		volumezone.Name:                      volumezone.New,
		nodevolumelimits.CSIName:             runtime.FactoryAdapter(fts, nodevolumelimits.NewCSI),
		nodevolumelimits.EBSName:             runtime.FactoryAdapter(fts, nodevolumelimits.NewEBS),
		nodevolumelimits.GCEPDName:           runtime.FactoryAdapter(fts, nodevolumelimits.NewGCEPD),
		nodevolumelimits.AzureDiskName:       runtime.FactoryAdapter(fts, nodevolumelimits.NewAzureDisk),
		nodevolumelimits.CinderName:          runtime.FactoryAdapter(fts, nodevolumelimits.NewCinder),
		interpodaffinity.Name:                interpodaffinity.New,
		queuesort.Name:                       queuesort.New,
		defaultbinder.Name:                   defaultbinder.New,
		defaultpreemption.Name:               runtime.FactoryAdapter(fts, defaultpreemption.New),
		schedulinggates.Name:                 runtime.FactoryAdapter(fts, schedulinggates.New),
	}

	return registry
}

```

# 插件配置

那具体都配置什么呢？则就要引入[Scheduling Profile](https://v1-18.docs.kubernetes.io/docs/reference/scheduling/profiles)概念。如果实在不想了解，就简单理解为kube-scheduler内部运行着多个不同配置的调度器，每个调度器有一个名字，调度器是通过Scheduling Profile配置的。
关于Scheduling Profile的定义:

```golang

// KubeSchedulerProfile is a scheduling profile.
type KubeSchedulerProfile struct {
	// SchedulerName is the name of the scheduler associated to this profile.
	// If SchedulerName matches with the pod's "spec.schedulerName", then the pod
	// is scheduled with this profile.
	// 调度器名字
	SchedulerName string

	// PercentageOfNodesToScore is the percentage of all nodes that once found feasible
	// for running a pod, the scheduler stops its search for more feasible nodes in
	// the cluster. This helps improve scheduler's performance. Scheduler always tries to find
	// at least "minFeasibleNodesToFind" feasible nodes no matter what the value of this flag is.
	// Example: if the cluster size is 500 nodes and the value of this flag is 30,
	// then scheduler stops finding further feasible nodes once it finds 150 feasible ones.
	// When the value is 0, default percentage (5%--50% based on the size of the cluster) of the
	// nodes will be scored. It will override global PercentageOfNodesToScore. If it is empty,
	// global PercentageOfNodesToScore will be used.
	PercentageOfNodesToScore *int32

	// Plugins specify the set of plugins that should be enabled or disabled.
	// Enabled plugins are the ones that should be enabled in addition to the
	// default plugins. Disabled plugins are any of the default plugins that
	// should be disabled.
	// When no enabled or disabled plugin is specified for an extension point,
	// default plugins for that extension point will be used if there is any.
	// If a QueueSort plugin is specified, the same QueueSort Plugin and
	// PluginConfig must be specified for all profiles.
	// 指定应启用或禁止的插件集合，下面有代码注释
	Plugins *Plugins

	// PluginConfig is an optional set of custom plugin arguments for each plugin.
	// Omitting config args for a plugin is equivalent to using the default config
	// for that plugin.
	// 一组可选的自定义插件参数，无参数等于使用该插件的默认参数
	PluginConfig []PluginConfig
}

// Plugins include multiple extension points. When specified, the list of plugins for
// a particular extension point are the only ones enabled. If an extension point is
// omitted from the config, then the default set of plugins is used for that extension point.
// Enabled plugins are called in the order specified here, after default plugins. If they need to
// be invoked before default plugins, default plugins must be disabled and re-enabled here in desired order.
// Plugins包含了各个阶段插件的配置，PluginSet见下面代码注释
type Plugins struct {
	// PreEnqueue is a list of plugins that should be invoked before adding pods to the scheduling queue.
	PreEnqueue PluginSet

	// QueueSort is a list of plugins that should be invoked when sorting pods in the scheduling queue.
	QueueSort PluginSet

	// PreFilter is a list of plugins that should be invoked at "PreFilter" extension point of the scheduling framework.
	PreFilter PluginSet

	// Filter is a list of plugins that should be invoked when filtering out nodes that cannot run the Pod.
	Filter PluginSet

	// PostFilter is a list of plugins that are invoked after filtering phase, but only when no feasible nodes were found for the pod.
	PostFilter PluginSet

	// PreScore is a list of plugins that are invoked before scoring.
	PreScore PluginSet

	// Score is a list of plugins that should be invoked when ranking nodes that have passed the filtering phase.
	Score PluginSet

	// Reserve is a list of plugins invoked when reserving/unreserving resources
	// after a node is assigned to run the pod.
	Reserve PluginSet

	// Permit is a list of plugins that control binding of a Pod. These plugins can prevent or delay binding of a Pod.
	Permit PluginSet

	// PreBind is a list of plugins that should be invoked before a pod is bound.
	PreBind PluginSet

	// Bind is a list of plugins that should be invoked at "Bind" extension point of the scheduling framework.
	// The scheduler call these plugins in order. Scheduler skips the rest of these plugins as soon as one returns success.
	Bind PluginSet

	// PostBind is a list of plugins that should be invoked after a pod is successfully bound.
	PostBind PluginSet

	// MultiPoint is a simplified config field for enabling plugins for all valid extension points
	MultiPoint PluginSet
}

// PluginSet specifies enabled and disabled plugins for an extension point.
// If an array is empty, missing, or nil, default plugins at that extension point will be used.
type PluginSet struct {
	// Enabled specifies plugins that should be enabled in addition to default plugins.
	// These are called after default plugins and in the same order specified here.
	// 指定默认插件外还应启用的插件，这些将在默认插件之后以此处指定的顺序调用
	Enabled []Plugin
	// Disabled specifies default plugins that should be disabled.
	// When all default plugins need to be disabled, an array containing only one "*" should be provided.
	// 指定应禁用的默认插件，当需要禁用所有的默认插件时，应提供仅包含一个"*"的数组
	Disabled []Plugin
}

// Plugin specifies a plugin name and its weight when applicable. Weight is used only for Score plugins.
type Plugin struct {
	// Name defines the name of plugin
	// 插件名字
	Name string
	// Weight defines the weight of plugin, only used for Score plugins.
	// 插件的权重，仅用于ScorePlugin插件
	Weight int32
}

// PluginConfig specifies arguments that should be passed to a plugin at the time of initialization.
// A plugin that is invoked at multiple extension points is initialized once. Args can have arbitrary structure.
// It is up to the plugin to process these Args.
type PluginConfig struct {
	// Name defines the name of plugin being configured
	// 插件名字
	Name string
	// Args defines the arguments passed to the plugins at the time of initialization. Args can have arbitrary structure.
	// 插件初始化时的参数，可以是任意结构
	Args runtime.Object
}
```

上面提到了好多次默认插件，那么默认插件是什么？Provider和Policy都有一套默认插件，本文引用Provider的默认插件配置，因为它比较简单。

```golang
// /pkg/scheduler/apis/config/v1beta2/default_plugins.go
// getDefaultPlugins returns the default set of plugins.
func getDefaultPlugins() *v1beta2.Plugins {
	plugins := &v1beta2.Plugins{
		QueueSort: v1beta2.PluginSet{
			Enabled: []v1beta2.Plugin{
				{Name: names.PrioritySort},
			},
		},
		PreFilter: v1beta2.PluginSet{
			Enabled: []v1beta2.Plugin{
				{Name: names.NodeResourcesFit},
				{Name: names.NodePorts},
				{Name: names.VolumeRestrictions},
				{Name: names.PodTopologySpread},
				{Name: names.InterPodAffinity},
				{Name: names.VolumeBinding},
				{Name: names.VolumeZone},
				{Name: names.NodeAffinity},
			},
		},
		Filter: v1beta2.PluginSet{
			Enabled: []v1beta2.Plugin{
				{Name: names.NodeUnschedulable},
				{Name: names.NodeName},
				{Name: names.TaintToleration},
				{Name: names.NodeAffinity},
				{Name: names.NodePorts},
				{Name: names.NodeResourcesFit},
				{Name: names.VolumeRestrictions},
				{Name: names.EBSLimits},
				{Name: names.GCEPDLimits},
				{Name: names.NodeVolumeLimits},
				{Name: names.AzureDiskLimits},
				{Name: names.VolumeBinding},
				{Name: names.VolumeZone},
				{Name: names.PodTopologySpread},
				{Name: names.InterPodAffinity},
			},
		},
		PostFilter: v1beta2.PluginSet{
			Enabled: []v1beta2.Plugin{
				{Name: names.DefaultPreemption},
			},
		},
		PreScore: v1beta2.PluginSet{
			Enabled: []v1beta2.Plugin{
				{Name: names.InterPodAffinity},
				{Name: names.PodTopologySpread},
				{Name: names.TaintToleration},
				{Name: names.NodeAffinity},
				{Name: names.NodeResourcesFit},
				{Name: names.NodeResourcesBalancedAllocation},
			},
		},
		Score: v1beta2.PluginSet{
			Enabled: []v1beta2.Plugin{
				{Name: names.NodeResourcesBalancedAllocation, Weight: pointer.Int32(1)},
				{Name: names.ImageLocality, Weight: pointer.Int32(1)},
				{Name: names.InterPodAffinity, Weight: pointer.Int32(1)},
				{Name: names.NodeResourcesFit, Weight: pointer.Int32(1)},
				{Name: names.NodeAffinity, Weight: pointer.Int32(1)},
				// Weight is doubled because:
				// - This is a score coming from user preference.
				// - It makes its signal comparable to NodeResourcesFit.LeastAllocated.
				{Name: names.PodTopologySpread, Weight: pointer.Int32(2)},
				{Name: names.TaintToleration, Weight: pointer.Int32(1)},
			},
		},
		Reserve: v1beta2.PluginSet{
			Enabled: []v1beta2.Plugin{
				{Name: names.VolumeBinding},
			},
		},
		PreBind: v1beta2.PluginSet{
			Enabled: []v1beta2.Plugin{
				{Name: names.VolumeBinding},
			},
		},
		Bind: v1beta2.PluginSet{
			Enabled: []v1beta2.Plugin{
				{Name: names.DefaultBinder},
			},
		},
	}
	applyFeatureGates(plugins)

	return plugins
}


```

# 总结

1. 从调度队列的排序到最终Pod绑定到Node，整个过程分为QueueSortPlugin、PreFilterPlugin、FilterPlugin、PostFilterPlugin、PreScorePlugin、ScorePlugin、ReservePlugin、PermitPlugin、PreBindPlugin、BindPlugin、PostBindPlugin总共11个接口，官方称这些接口为调度框架的11个扩展点，可以按需扩展调度能力；
2. 每种类型的插件可以有多种实现，比如PreFiltePlugin就有InterPodAffinity、PodTopologySpread、SelectorSpread和TaintToleration 4种实现，每种实现都对应一种调度需求；
3. 一个类型可以实现多种类型插件接口（多继承），比如VolumeBinding就实现了PreFilterPlugin、FilterPlugin、ReservePlugin和PreBindPlugin 4种插件接口，因为PVC与PV绑定需要在调度过程的不同阶段做不同的事情；
4. 所有插件都需要静态编译注册到注册表中，可以通过Provider、Policy(File、ConfigMap)方式配置插件；
5. 每个配置都有一个名字，对应一个Scheduling Profile，调度Pod的时候可以指定调度器名字；




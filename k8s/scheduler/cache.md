# 前言

在[调度队列](./scheduling_queue.md),调度队列（SchedulingQueue）中都是Pending状态的Pod，也就是未调度的Pod。
而本文分析的Cache中都是已经调度的Pod（包括假定调度的Pod）。而Cache并不是仅仅为了存储已调度的Pod方便查找，而是为调度提供能非常重要的状态信息，甚至已经超越了Cache本身定义范畴。

既然定义为Cache，需要回答如下几个问题：

1. cache谁？kubernetes的信息都存储在etcd中，而访问kubernetes的etcd的唯一方法是通过apiserver，所以准确的说是缓存etcd的信息。
2. cache哪些信息？调度器需要将Pod调度到满足需求的Node上，所以cache至少要缓存Pod和Node信息，这样才能提高kube-scheduler访问apiserver的性能。
3. 为什么要cache？为了调度。本文的Cache不仅缓存了Pod和Node信息，更关键的是聚合了调度结果，让调度变得更容易，也就是本文重点内容。

# Cache

## Cache的接口

```golang
// /pkg/scheduler/internal/cache/interface.go
type Cache interface {
	// NodeCount returns the number of nodes in the cache.
	// DO NOT use outside of tests.
	// 获取node的数量，用于单元测试使用
	NodeCount() int

	// PodCount returns the number of pods in the cache (including those from deleted nodes).
	// DO NOT use outside of tests.
	// 获取Pod的数量，用于单元测试使用
	PodCount() (int, error)

	// AssumePod assumes a pod scheduled and aggregates the pod's information into its node.
	// The implementation also decides the policy to expire pod before being confirmed (receiving Add event).
	// After expiration, its information would be subtracted.
    // 此处需要给出一个概念：假定Pod,就是将Pod假定调度到指定的Node,但还没有Bind完成。
    // 为什么要这么设计？因为kube-scheduler是通过异步的方式实现Bind，在Bind完成前，
    // 调度器还要调度新的Pod，此时就先假定Pod调度完成了。至于什么是Bind？为什么Bind？
    // 怎么Bind?笔者会在其他文章中解析，此处简单理解为：需要将Pod的调度结果写入etcd,
    // 持久化调度结果，所以也是相对比较耗时的操作。
    // AssumePod会将Pod的资源需求累加到Node上，这样kube-scheduler在调度其他Pod的时候,
    // 就不会占用这部分资源。
	AssumePod(pod *v1.Pod) error

	// FinishBinding signals that cache for assumed pod can be expired
    // 前面提到了，Bind是一个异步过程，当Bind完成后需要调用这个接口通知Cache，
    // 如果完成Bind的Pod长时间没有被确认(确认方法是AddPod)，那么Cache就会清理掉假定过期的Pod。
	FinishBinding(pod *v1.Pod) error

	// ForgetPod removes an assumed pod from cache.
    // 删除假定的Pod，kube-scheduler在调用AssumePod后如果遇到其他错误，就需要调用这个接口
	ForgetPod(pod *v1.Pod) error

	// AddPod either confirms a pod if it's assumed, or adds it back if it's expired.
	// If added back, the pod's information would be added again.
    // 添加Pod既确认了假定的Pod，也会将假定过期的Pod重新添加回来。
	AddPod(pod *v1.Pod) error

	// UpdatePod removes oldPod's information and adds newPod's information.
    // 更新Pod，其实就是删除再添加
	UpdatePod(oldPod, newPod *v1.Pod) error

	// RemovePod removes a pod. The pod's information would be subtracted from assigned node.
    // 删除Pod.
	RemovePod(pod *v1.Pod) error

	// GetPod returns the pod from the cache with the same namespace and the
	// same name of the specified pod.
    // 获取Pod.
	GetPod(pod *v1.Pod) (*v1.Pod, error)

	// IsAssumedPod returns true if the pod is assumed and not expired.
    // 判断Pod是否假定调度
	IsAssumedPod(pod *v1.Pod) (bool, error)

	// AddNode adds overall information about node.
	// It returns a clone of added NodeInfo object.
    // 添加Node的全部信息
	AddNode(node *v1.Node) *framework.NodeInfo

	// UpdateNode updates overall information about node.
	// It returns a clone of updated NodeInfo object.
	// 更新Node的全部信息
	UpdateNode(oldNode, newNode *v1.Node) *framework.NodeInfo

	// RemoveNode removes overall information about node.
	// 删除Node的全部信息
	RemoveNode(node *v1.Node) error

	// UpdateSnapshot updates the passed infoSnapshot to the current contents of Cache.
	// The node info contains aggregated information of pods scheduled (including assumed to be)
	// on this node.
	// The snapshot only includes Nodes that are not deleted at the time this function is called.
	// nodeinfo.Node() is guaranteed to be not nil for all the nodes in the snapshot.
    // 其实就是产生Cache的快照并输出到nodeSnapshot中，那为什么是更新呢？
    // 因为快照比较大，产生快照也是一个比较重的任务，如果能够基于上次快照把增量的部分更新到上一次快照中，
    // 就会变得没那么重了，这就是接口名字是更新快照的原因。文章后面会重点分析这个函数，
    // 因为其他接口非常简单，理解了这个接口基本上就理解了Cache的精髓所在。
	UpdateSnapshot(nodeSnapshot *Snapshot) error

	// Dump produces a dump of the current cache.
    // Dump会快照Cache，用于调试使用
	Dump() *Dump
}

// Dump is a dump of the cache state.
type Dump struct {
	AssumedPods sets.String
	Nodes       map[string]*framework.NodeInfo
}

```

从Cache的接口设计上可以看出，Cache只缓存了Pod和Node信息，而Pod和Node信息存储在etcd中(可以通过kubectl增删改查)，所以可以确认Cache缓存了etcd中的Pod和Node信息。

Pod状态机如下：

```
// Cache collects pods' information and provides node-level aggregated information.
// It's intended for generic scheduler to do efficient lookup.
// Cache's operations are pod centric. It does incremental updates based on pod events.
// Pod events are sent via network. We don't have guaranteed delivery of all events:
// We use Reflector to list and watch from remote.
// Reflector might be slow and do a relist, which would lead to missing events.
//
// State Machine of a pod's events in scheduler's cache:
//
//	+-------------------------------------------+  +----+
//	|                            Add            |  |    |
//	|                                           |  |    | Update
//	+      Assume                Add            v  v    |
//
// Initial +--------> Assumed +------------+---> Added <--+
//
//	^                +   +               |       +
//	|                |   |               |       |
//	|                |   |           Add |       | Remove
//	|                |   |               |       |
//	|                |   |               +       |
//	+----------------+   +-----------> Expired   +----> Deleted
//	      Forget             Expire
//
// Note that an assumed pod can expire, because if we haven't received Add event notifying us
// for a while, there might be some problems and we shouldn't keep the pod in cache anymore.
//
// Note that "Initial", "Expired", and "Deleted" pods do not actually exist in cache.
// Based on existing use cases, we are making the following assumptions:
//   - No pod would be assumed twice
//   - A pod could be added without going through scheduler. In this case, we will see Add but not Assume event.
//   - If a pod wasn't added, it wouldn't be removed or updated.
//   - Both "Expired" and "Deleted" are valid end states. In case of some problems, e.g. network issue,
//     a pod might have changed its state (e.g. added and deleted) without delivering notification to the cache.
```

## NodeInfo的定义

```golang
// /pkg/scheduler/framework/types.go
// NodeInfo is node level aggregated information.
type NodeInfo struct {
	// Overall node information.
	// Node API对象
	node *v1.Node

	// Pods running on the node.
	// 运行在Node上的所有Pod
	Pods []*PodInfo

	// The subset of pods with affinity.
    // PodsWithAffinity是Pods的子集，所有的Pod都声明了亲和性
	PodsWithAffinity []*PodInfo

	// The subset of pods with required anti-affinity.
   // PodsWithRequiredAntiAffinity是Pods子集，所有的Pod都声明了反亲和性
	PodsWithRequiredAntiAffinity []*PodInfo

	// Ports allocated on the node.
   // 本文无关，忽略
	UsedPorts HostPortInfo

	// Total requested resources of all pods on this node. This includes assumed
	// pods, which scheduler has sent for binding, but may not be scheduled yet.
   // 此Node上所有Pod的总Request资源，包括假定的Pod，调度器已发送该Pod进行绑定，但可能尚未对其进行调度。
	Requested *Resource
	// Total requested resources of all pods on this node with a minimum value
	// applied to each container's CPU and memory requests. This does not reflect
	// the actual resource requests for this node, but is used to avoid scheduling
	// many zero-request pods onto one node.
	// Pod的容器资源请求有的时候是0，kube-scheduler为这类容器设置默认的资源最小值，并累加到NonZeroRequested.
	// 也就是说，NonZeroRequested等于Requested加上所有按照默认最小值累加的零资源
	// 这并不反映此节点的实际资源请求，而是用于避免将许多零资源请求的Pod调度到一个Node上。
	NonZeroRequested *Resource
	// We store allocatedResources (which is Node.Status.Allocatable.*) explicitly
	// as int64, to avoid conversions and accessing map.
	// Node的可分配的资源量
	Allocatable *Resource

	// ImageStates holds the entry of an image if and only if this image is on the node. The entry can be used for
	// checking an image's existence and advanced usage (e.g., image locality scheduling policy) based on the image
	// state information.
	// 镜像状态，比如Node上有哪些镜像，镜像的大小，有多少Node相应的镜像等。
	ImageStates map[string]*ImageStateSummary

	// PVCRefCounts contains a mapping of PVC names to the number of pods on the node using it.
	// Keys are in the format "namespace/name".
	PVCRefCounts map[string]int

	// Whenever NodeInfo changes, generation is bumped.
	// This is used to avoid cloning it if the object didn't change.
   // 类似于版本，NodeInfo的任何状态变化都会使得Generation增加，比如有新的Pod调度到Node上
   // 这个Generation很重要，可以用于只复制变化的Node对象，后面更新镜像的时候会详细说明
	Generation int64
}

```

## nodeTree

nodeTree是按照区域(zone)将Node组织成树状结构，当需要按区域列举或者全量列举按照区域排序，nodeTree就会用的上。为什么有这个需求，还是那句话，调度需要。举一个可能不恰当的例子：比如多个Pod的副本需要部署在同一个区域亦或是不同的区域。

```golang
// nodeTree is a tree-like data structure that holds node names in each zone. Zone names are
// keys to "NodeTree.tree" and values of "NodeTree.tree" are arrays of node names.
// NodeTree is NOT thread-safe, any concurrent updates/reads from it must be synchronized by the caller.
// It is used only by schedulerCache, and should stay as such.
type nodeTree struct {
    // map的键是zone名字，map的值是该区域内所有Node的名字。
	tree     map[string][]string // a map from zone (region-zone) to an array of nodes in the zone.
	// 所有的zone的名字
	zones    []string            // a list of all the zones in the tree (keys)
	//  Node的数量
	numNodes int
}

```

nodeTree只是把Node名字组织成树状，如果需要NodeInfo还需要根据Node的名字查找NodeInfo。

## 快照

快照是对Cache某一时刻的复制，随着时间的推移，Cache的状态在持续更新，kube-scheduler在调度一个Pod的时候需要获取Cache的快照。相比于直接访问Cache，快照可以解决如下几个问题：

快照不会再有任何变化，可以理解为只读，那么访问快照不需要加锁保证保证原子性；
快照和Cache让读写分离，可以避免大范围的锁造成Cache访问性能下降；
虽然快照的状态从创建开始就落后于(因为Cache可能随时都会更新)Cache，但是对于kube-scheduler调度一个Pod来说是没问题的，至于原因笔者会在解析调度流程中加以说明。

```golang
// Snapshot is a snapshot of cache NodeInfo and NodeTree order. The scheduler takes a
// snapshot at the beginning of each scheduling cycle and uses it for its operations in that cycle.
type Snapshot struct {
	// nodeInfoMap a map of node name to a snapshot of its NodeInfo.
	// nodeInfoMap用于根据Node的key(NS+Name)快速查找Node
	nodeInfoMap map[string]*framework.NodeInfo
	// nodeInfoList is the list of nodes as ordered in the cache's nodeTree.
	// nodeInfoList是Cache中Node全集列表（不包含已删除的Node），按照nodeTree排序.
	nodeInfoList []*framework.NodeInfo
	// havePodsWithAffinityNodeInfoList is the list of nodes with at least one pod declaring affinity terms.
	// 只要Node上有任何Pod声明了亲和性，那么该Node就要放入havePodsWithAffinityNodeInfoList。
	// 为什么要有这个变量？当然是为了调度，比如PodA需要和PodB调度在一个Node上。
	havePodsWithAffinityNodeInfoList []*framework.NodeInfo
	// havePodsWithRequiredAntiAffinityNodeInfoList is the list of nodes with at least one pod declaring
	// required anti-affinity terms.
    // havePodsWithRequiredAntiAffinityNodeInfoList和havePodsWithAffinityNodeInfoList相似，
    // 只是Pod声明了反亲和，比如PodA不能和PodB调度在一个Node上
	havePodsWithRequiredAntiAffinityNodeInfoList []*framework.NodeInfo
	// usedPVCSet contains a set of PVC names that have one or more scheduled pods using them,
	// keyed in the format "namespace/name".
	usedPVCSet sets.String
    // generation是所有NodeInfo.Generation的最大值，因为所有NodeInfo.Generation都源于一个全局的Generation变量，
    // 那么Cache中的NodeInfo.Gerneraion大于该值的就是在快照产生后更新过的Node。
    // kube-scheduler调用Cache.UpdateSnapshot的时候只需要更新快照之后有变化的Node即可
	generation int64
}

```

## Cache实现

前面铺垫了已经足够了，现在开始进入重点内容，先来看看Cache实现类cacheImpl的定义。

```golang
type cacheImpl struct {
    // 这个比较好理解，用来通知schedulerCache停止的chan，说明schedulerCache有自己的协程
	stop   <-chan struct{}
    // 假定Pod一旦完成绑定，就要在指定的时间内确认，否则就会超时，ttl就是指定的过期时间，默认30秒
	ttl    time.Duration
	// 定时清理“假定过期”的Pod，period就是定时周期，默认是1秒钟
	// 前面提到了schedulerCache有自己的协程，就是定时清理超时的假定Pod.
	period time.Duration

	// This mutex guards all fields within this cache struct.
	// 锁，说明schedulerCache利用互斥锁实现协程安全，而不是用chan与其他协程交互。
	mu sync.RWMutex
	// a set of assumed pod keys.
	// The key could further be used to get an entry in podStates.
	// 假定Pod集合，map的key与podStates相同，都是Pod的NS+NAME
	assumedPods sets.String
	// a map from pod key to podState.
	// 所有的Pod，此处用的是podState
	// podState继承了Pod的API定义，增加了Cache需要的属性
	podStates map[string]*podState
	// 所有的Node，键是Node.Name，值是nodeInfoListItem
	nodes     map[string]*nodeInfoListItem
	// headNode points to the most recently updated NodeInfo in "nodes". It is the
	// head of the linked list.
    // 所有的Node再通过双向链表连接起来
	headNode *nodeInfoListItem
    // 节点按照zone组织成树状，前面提到用nodeTree中Node的名字再到nodes中就可以查找到NodeInfo.
	nodeTree *nodeTree
	// A map from image name to its imageState.
	// 镜像状态，本文不做重点说明，只需要知道Cache还统计了镜像的信息就可以了。
	imageStates map[string]*imageState
}


// nodeInfoListItem holds a NodeInfo pointer and acts as an item in a doubly
// linked list. When a NodeInfo is updated, it goes to the head of the list.
// The items closer to the head are the most recently updated items.
type nodeInfoListItem struct {
	info *framework.NodeInfo
	next *nodeInfoListItem
	prev *nodeInfoListItem
}

type podState struct {
	pod *v1.Pod
	// Used by assumedPod to determinate expiration.
	// If deadline is nil, assumedPod will never expire.
	// 假定Pod的超时截止时间，用于判断假定Pod是否过期。
	deadline *time.Time
	// Used to block cache from expiring assumedPod if binding still runs
    // 调用Cache.AssumePod的假定Pod不是所有的都需要判断是否过期，因为有些假定Pod可能还在Binding
    // bindingFinished就是用于标记已经Bind完成的Pod，然后开始计时，计时的方法就是设置deadline
    // 还记得Cache.FinishBinding接口么？就是用来设置bindingFinished和deadline的，后面代码会有解析
	bindingFinished bool
}

type imageState struct {
	// Size of the image
	size int64
	// A set of node names for nodes having this image present
	nodes sets.String
}
```

问题来了，既然已经有了nodes(map类型)变量，为什么还要再加一个headNode(list类型)的变量？这不是多此一举么？其实不然，nodes可以根据Node的名字快速找到Node，而headNode则是根据某个规则排过序的。这一点和SchedulingQueue中介绍的用map/slice实现队列是一个道理，至于为什么用list而不是slice，肯定是排序方法链表的效率高于slice，后面在更新headNode的地方再做说明，此处先排除疑虑。

从cacheImpl的定义基本可以猜到大部分Cache接口的实现，本文对于比较简单的接口实现只做简要说明，将文字落在一些重点的函数上。PodCount和NodeCount两个函数因为用于单元测试使用，本文不做说明。

### AssumePod

当kube-scheduler找到最优的Node调度Pod的时候会调用AssumePod假定Pod调度，在通过另一个协程异步Bind。假定其实就是预先占住资源，kube-scheduler调度下一个Pod的时候不会把这部分资源抢走，直到收到确认消息AddPod确认调度成功，亦或是Bind失败ForgetPod取消假定调度。

```golang
func (cache *cacheImpl) AssumePod(pod *v1.Pod) error {
    // 获取Pod的唯一key，就是NS+Name，因为kube-scheduler调度整个集群的Pod
	key, err := framework.GetPodKey(pod)
	if err != nil {
		return err
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()
    // 如果Pod已经存在，则不能假定调度。因为在Cache中的Pod要么是假定调度的，要么是完成调度的
	if _, ok := cache.podStates[key]; ok {
		return fmt.Errorf("pod %v(%v) is in the cache, so can't be assumed", key, klog.KObj(pod))
	}
    // 见下面代码注释
	return cache.addPod(pod, true)
}

// Assumes that lock is already acquired.
func (cache *cacheImpl) addPod(pod *v1.Pod, assumePod bool) error {
	key, err := framework.GetPodKey(pod)
	if err != nil {
		return err
	}
    // 查找Pod调度的Node，如果不存在则创建一个虚Node，虚Node只是没有Node API对象。
    // 为什么会这样？可能kube-scheduler调度Pod的时候Node被删除了，可能很快还会添加回来
    // 也可能就彻底删除了，此时先放在这个虚的Node上，如果Node不存在后期还会被迁移。
	n, ok := cache.nodes[pod.Spec.NodeName]
	if !ok {
		n = newNodeInfoListItem(framework.NewNodeInfo())
		cache.nodes[pod.Spec.NodeName] = n
	}
    // AddPod就是把Pod的资源累加到NodeInfo中，本文不做详细说明，感兴趣的读者自行查看源码
    // 但需要知道的是n.info.AddPod(pod)会更新NodeInfo.Generation，表示NodeInfo是最新的
	n.info.AddPod(pod)
    // 将Node放到cacheImpl.headNode队列头部，因为NodeInfo当前是最新的，所以放在头部。
    // 此处可以解答为什么用list而不是slice，因为每次都是将Node直接放在第一个位置，明显list效率更高
    // 所以headNode是按照最近更新排序的
	cache.moveNodeInfoToHead(pod.Spec.NodeName)
	ps := &podState{
		pod: pod,
	}
	// 更新 podStates
	cache.podStates[key] = ps
	if assumePod {
        // 把Pod key 添加到set中
		cache.assumedPods.Insert(key)
	}
	return nil
}
```

### ForgetPod

假定Pod预先占用了一些资源，如果之后的操作(比如Bind)有什么错误，就需要取消假定调度，释放出资源。

```golang

func (cache *cacheImpl) ForgetPod(pod *v1.Pod) error {
	key, err := framework.GetPodKey(pod)
	if err != nil {
		return err
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()

    // 这里有意思了，也就是说Cache假定Pod的Node名字与传入的Pod的Node名字不一致，则返回错误
    // 这种情况会不会发生呢?有可能，但是可能性不大，毕竟多协程修改Pod调度状态会有各种可能性。
	currState, ok := cache.podStates[key]
	if ok && currState.pod.Spec.NodeName != pod.Spec.NodeName {
		return fmt.Errorf("pod %v(%v) was assumed on %v but assigned to %v", key, klog.KObj(pod), pod.Spec.NodeName, currState.pod.Spec.NodeName)
	}

	// Only assumed pod can be forgotten.
	// 只有假定Pod可以被Forget，因为Forget就是为了取消假定Pod的。
	if ok && cache.assumedPods.Has(key) {
		// removePod()就是把假定Pod的资源从NodeInfo中减去
		return cache.removePod(pod)
	}
	return fmt.Errorf("pod %v(%v) wasn't assumed so cannot be forgotten", key, klog.KObj(pod))
}

// Assumes that lock is already acquired.
// Removes a pod from the cached node info. If the node information was already
// removed and there are no more pods left in the node, cleans up the node from
// the cache.
func (cache *cacheImpl) removePod(pod *v1.Pod) error {
	key, err := framework.GetPodKey(pod)
	if err != nil {
		return err
	}

    // 找到假定Pod调度的Node
	n, ok := cache.nodes[pod.Spec.NodeName]
	if !ok {
		klog.ErrorS(nil, "Node not found when trying to remove pod", "node", klog.KRef("", pod.Spec.NodeName), "podKey", key, "pod", klog.KObj(pod))

	} else {
       // 减去假定Pod的资源，并从NodeInfo的Pod列表移除假定Pod
       // 和n.info.AddPod相同，也会更新NodeInfo.Generation
		if err := n.info.RemovePod(pod); err != nil {
			return err
		}
        // 如果NodeInfo的Pod列表没有任何Pod并且Node被删除，则Node从Cache中删除
        // 否则将NodeInfo移到列表头，因为NodeInfo被更新，需要放到表头
        // 这里需要知道的是，Node被删除Cache不会立刻删除该Node，需要等到Node上所有的Pod从Node中迁移后才删除，
		if len(n.info.Pods) == 0 && n.info.Node() == nil {
			cache.removeNodeInfoFromList(pod.Spec.NodeName)
		} else {
			cache.moveNodeInfoToHead(pod.Spec.NodeName)
		}
	}

    // 删除Pod和假定状态
	delete(cache.podStates, key)
	delete(cache.assumedPods, key)
	return nil
}
```

### FinishBinding

当假定Pod绑定完成后，需要调用FinishBinding通知Cache开始计时，直到假定Pod过期如果依然没有收到AddPod的请求，则将过期假定Pod删除。

```golang

func (cache *cacheImpl) FinishBinding(pod *v1.Pod) error {
    // 取当前时间
	return cache.finishBinding(pod, time.Now())
}

// finishBinding exists to make tests deterministic by injecting now as an argument
func (cache *cacheImpl) finishBinding(pod *v1.Pod, now time.Time) error {
    // 获取Pod唯一key
	key, err := framework.GetPodKey(pod)
	if err != nil {
		return err
	}

	cache.mu.RLock()
	defer cache.mu.RUnlock()

	klog.V(5).InfoS("Finished binding for pod, can be expired", "podKey", key, "pod", klog.KObj(pod))
	currState, ok := cache.podStates[key]
    // Pod存在并且是假定状态才行
	if ok && cache.assumedPods.Has(key) {
        // 标记为完成Binding，并且设置过期时间，ttl默认是30秒。
		if cache.ttl == time.Duration(0) {
			currState.deadline = nil
		} else {
			dl := now.Add(cache.ttl)
			currState.deadline = &dl
		}
		currState.bindingFinished = true
	}
	return nil
}
```

### AddPod

当Pod Bind成功，kube-scheduler会收到消息，然后调用AddPod确认调度结果。

```golang

func (cache *cacheImpl) AddPod(pod *v1.Pod) error {
    // 获取Pod唯一key
	key, err := framework.GetPodKey(pod)
	if err != nil {
		return err
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()

   // 以下是根据Pod在Cache中的状态决定需要如何处理
	currState, ok := cache.podStates[key]
	switch {
    // Pod是假定调度
	case ok && cache.assumedPods.Has(key):
		// When assuming, we've already added the Pod to cache,
		// Just update here to make sure the Pod's status is up-to-date.
		// 更新Pod ,先删除POD， 然后添加POD
		if err = cache.updatePod(currState.pod, pod); err != nil {
			klog.ErrorS(err, "Error occurred while updating pod")
		}
		if currState.pod.Spec.NodeName != pod.Spec.NodeName {
			// The pod was added to a different node than it was assumed to.
			klog.InfoS("Pod was added to a different node than it was assumed", "podKey", key, "pod", klog.KObj(pod), "assumedNode", klog.KRef("", pod.Spec.NodeName), "currentNode", klog.KRef("", currState.pod.Spec.NodeName))
			return nil
		}
	case !ok:
		// Pod was expired. We should add it back.
	    // Pod可能已经假定过期被删除了，需要重新添加回来
		if err = cache.addPod(pod, false); err != nil {
			klog.ErrorS(err, "Error occurred while adding pod")
		}
	default:
		return fmt.Errorf("pod %v(%v) was already in added state", key, klog.KObj(pod))
	}
	return nil
}

// Assumes that lock is already acquired.
func (cache *cacheImpl) updatePod(oldPod, newPod *v1.Pod) error {
	if err := cache.removePod(oldPod); err != nil {
		return err
	}
	return cache.addPod(newPod, false)
}

```

### RemovePod
kube-scheduler收到删除Pod的请求，如果Pod在Cache中，就需要调用RemovePod。

```golang

func (cache *cacheImpl) RemovePod(pod *v1.Pod) error {
   // 获取Pod唯一key
	key, err := framework.GetPodKey(pod)
	if err != nil {
		return err
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()

	currState, ok := cache.podStates[key]
	if !ok {
		return fmt.Errorf("pod %v(%v) is not found in scheduler cache, so cannot be removed from it", key, klog.KObj(pod))
	}
    // 卧槽，Pod的Node和AddPod时的Node不一样？这回的选择非常直接，奔溃，已经超时异常解决范围了
    // 如果再继续下去可能会造成调度状态的混乱，不如重启再来。
	if currState.pod.Spec.NodeName != pod.Spec.NodeName {
		klog.ErrorS(nil, "Pod was added to a different node than it was assumed", "podKey", key, "pod", klog.KObj(pod), "assumedNode", klog.KRef("", pod.Spec.NodeName), "currentNode", klog.KRef("", currState.pod.Spec.NodeName))
		if pod.Spec.NodeName != "" {
			// An empty NodeName is possible when the scheduler misses a Delete
			// event and it gets the last known state from the informer cache.
			klog.ErrorS(nil, "scheduler cache is corrupted and can badly affect scheduling decisions")
			klog.FlushAndExit(klog.ExitFlushTimeout, 1)
		}
	}
	// 删除POD
	return cache.removePod(currState.pod)
}
```

### AddNode

有新的Node添加到集群，kube-scheduler调用该接口通知Cache。

```golang

func (cache *cacheImpl) AddNode(node *v1.Node) *framework.NodeInfo {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	n, ok := cache.nodes[node.Name]
	if !ok {
        // 如果NodeInfo不存在则创建
		n = newNodeInfoListItem(framework.NewNodeInfo())
		cache.nodes[node.Name] = n
	} else {
        // 已存在，先删除镜像状态，因为后面还会在添加回来
		cache.removeNodeImageStates(n.info.Node())
	}
    // 将Node放到列表头
	cache.moveNodeInfoToHead(node.Name)
    // 添加到nodeTree中
	cache.nodeTree.addNode(node)
	// 添加Node的镜像状态
	cache.addNodeImageStates(node, n.info)
   // 只有SetNode的NodeInfo才是真实的Node，否则就是前文提到的虚的Node
	n.info.SetNode(node)
	return n.info.Clone()
}

```

### RemoveNode

Node从集群中删除，kube-scheduler调用该接口通知Cache。

```golang
// RemoveNode removes a node from the cache's tree.
// The node might still have pods because their deletion events didn't arrive
// yet. Those pods are considered removed from the cache, being the node tree
// the source of truth.
// However, we keep a ghost node with the list of pods until all pod deletion
// events have arrived. A ghost node is skipped from snapshots.
func (cache *cacheImpl) RemoveNode(node *v1.Node) error {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	n, ok := cache.nodes[node.Name]
	if !ok {
        // 如果Node不存在返回错误
		return fmt.Errorf("node %v is not found", node.Name)
	}
	// RemoveNode就是将*v1.Node设置为nil，此时Node就是虚的了
	n.info.RemoveNode()
	// We remove NodeInfo for this node only if there aren't any pods on this node.
	// We can't do it unconditionally, because notifications about pods are delivered
	// in a different watch, and thus can potentially be observed later, even though
	// they happened before node removal.
	// 当Node上没有运行Pod的时候删除Node，否则把Node放在列表头，因为Node状态更新了
	// 熟悉etcd的同学会知道，watch两个路径(Node和Pod)是两个通道，这样会造成两个通道的事件不会按照严格时序到达
	// 这应该是存在虚Node的原因之一。
	if len(n.info.Pods) == 0 {
		cache.removeNodeInfoFromList(node.Name)
	} else {
		cache.moveNodeInfoToHead(node.Name)
	}
    // 虽然nodes只有在NodeInfo中Pod数量为零的时候才会被删除，但是nodeTree会直接删除
    // 说明nodeTree中体现了实际的Node状态，kube-scheduler调度Pod的时候也是利用nodeTree
    // 这样就不会将Pod调度到已经删除的Node上了。
	if err := cache.nodeTree.removeNode(node); err != nil {
		return err
	}
	cache.removeNodeImageStates(node)
	return nil
}

```

### 后期清理协程函数run

前文提到过，Cache有自己的协程，就是用来清理假定到期的Pod。

```golang

func (cache *cacheImpl) run() {
    // 定时1秒钟执行一次cleanupExpiredAssumedPods
	go wait.Until(cache.cleanupExpiredAssumedPods, cache.period, cache.stop)
}

func (cache *cacheImpl) cleanupExpiredAssumedPods() {
	cache.cleanupAssumedPods(time.Now())
}

// cleanupAssumedPods exists for making test deterministic by taking time as input argument.
// It also reports metrics on the cache size for nodes, pods, and assumed pods.
func (cache *cacheImpl) cleanupAssumedPods(now time.Time) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	defer cache.updateMetrics()

	// The size of assumedPods should be small
	// 遍历假定Pod
	for key := range cache.assumedPods {
		ps, ok := cache.podStates[key]
		if !ok {
			klog.ErrorS(nil, "Key found in assumed set but not in podStates, potentially a logical error")
			klog.FlushAndExit(klog.ExitFlushTimeout, 1)
		}
       // 如果Pod没有标记为结束Binding，则忽略，说明Pod还在Binding中
	   // 说白了就是没有调用FinishBinding的Pod不用处理
		if !ps.bindingFinished {
			klog.V(5).InfoS("Could not expire cache for pod as binding is still in progress", "podKey", key, "pod", klog.KObj(ps.pod))
			continue
		}
        // 如果当前时间已经超过了Pod假定过期时间，说明Pod假定时间已过期
		if cache.ttl != 0 && now.After(*ps.deadline) {
			klog.InfoS("Pod expired", "podKey", key, "pod", klog.KObj(ps.pod))
            // 清理假定过期的Pod
			if err := cache.removePod(ps.pod); err != nil {
				klog.ErrorS(err, "ExpirePod failed", "podKey", key, "pod", klog.KObj(ps.pod))
			}
		}
	}
}
```

### UpdateSnapshot

好了，前文那么多的铺垫，都是为了UpdateSnapshot，因为Cache存在的核心目的就是给kube-scheduler提供Node镜像，让kube-scheduler根据Node的状态调度新的Pod。而Cache中的Pod是为了计算Node的资源状态存在的，毕竟二者在etcd中是两个路径。

```golang
// UpdateSnapshot takes a snapshot of cached NodeInfo map. This is called at
// beginning of every scheduling cycle.
// The snapshot only includes Nodes that are not deleted at the time this function is called.
// nodeInfo.Node() is guaranteed to be not nil for all the nodes in the snapshot.
// This function tracks generation number of NodeInfo and updates only the
// entries of an existing snapshot that have changed after the snapshot was taken.
// UpdateSnapshot更新的是参数nodeSnapshot，不是更新Cache.
// 也就是Cache需要找到当前与nodeSnapshot的差异，然后更新它，这样nodeSnapshot就与Cache状态一致了
// 至少从函数执行完毕后是一致的。
func (cache *cacheImpl) UpdateSnapshot(nodeSnapshot *Snapshot) error {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	// Get the last generation of the snapshot.
    // 获取nodeSnapshot的版本
    // 此处需要多说一点：kube-scheudler为Node定义了全局的generation变量，每个Node状态变化都会造成generation+=1然后赋值给该Node
    // nodeSnapshot.generation就是最新NodeInfo.Generation，就是表头的那个NodeInfo。
	snapshotGeneration := nodeSnapshot.generation

	// NodeInfoList and HavePodsWithAffinityNodeInfoList must be re-created if a node was added
	// or removed from the cache.
    // 介绍Snapshot的时候提到了，快照中有三个列表，分别是全量、亲和性和反亲和性列表
    // 全量列表在没有Node添加或者删除的时候，是不需要更新的
	updateAllLists := false
	// HavePodsWithAffinityNodeInfoList must be re-created if a node changed its
	// status from having pods with affinity to NOT having pods with affinity or the other
	// way around.
    // 当有Node的亲和性状态发生了变化(以前没有任何Pod有亲和性声明现在有了，抑或反过来)，
    // 则需要更新快照中的亲和性列表
	updateNodesHavePodsWithAffinity := false
	// HavePodsWithRequiredAntiAffinityNodeInfoList must be re-created if a node changed its
	// status from having pods with required anti-affinity to NOT having pods with required
	// anti-affinity or the other way around.
	// 当有Node的反亲和性状态发生了变化(以前没有任何Pod有反亲和性声明现在有了，抑或反过来)，
	// 则需要更新快照中的反亲和性列表
	updateNodesHavePodsWithRequiredAntiAffinity := false
	// usedPVCSet must be re-created whenever the head node generation is greater than
	// last snapshot generation.
	updateUsedPVCSet := false

	// Start from the head of the NodeInfo doubly linked list and update snapshot
	// of NodeInfos updated after the last snapshot.
	// 遍历Node列表，为什么不遍历Node的map？因为Node列表是按照Generation排序的
	// 只要找到大于nodeSnapshot.generation的所有Node然后把他们更新到nodeSnapshot中就可以了
	for node := cache.headNode; node != nil; node = node.next {
        // 说明Node的状态已经在nodeSnapshot中了，因为但凡Node有任何更新，那么NodeInfo.Generation 
        // 肯定会大于snapshotGeneration，同时该Node后面的所有Node也不用在遍历了，因为他们的版本更低
		if node.info.Generation <= snapshotGeneration {
			// all the nodes are updated before the existing snapshot. We are done.
			break
		}
		if np := node.info.Node(); np != nil {
            // 如果nodeSnapshot中没有该Node，则在nodeSnapshot中创建Node，并标记更新全量列表，因为创建了新的Node
			existing, ok := nodeSnapshot.nodeInfoMap[np.Name]
			if !ok {
				updateAllLists = true
				existing = &framework.NodeInfo{}
				nodeSnapshot.nodeInfoMap[np.Name] = existing
			}
            // 克隆NodeInfo，这个比较好理解，肯定不能简单的把指针设置过去，这样会造成多协程读写同一个对象
            // 因为克隆操作比较重，所以能少做就少做，这也是利用Generation实现增量更新的原因
			clone := node.info.Clone()
			// We track nodes that have pods with affinity, here we check if this node changed its
			// status from having pods with affinity to NOT having pods with affinity or the other
			// way around.
			// 如果Pod以前或者现在有任何亲和性声明，则需要更新nodeSnapshot中的亲和性列表
			if (len(existing.PodsWithAffinity) > 0) != (len(clone.PodsWithAffinity) > 0) {
				updateNodesHavePodsWithAffinity = true
			}
            // 同上，需要更新非亲和性列表
			if (len(existing.PodsWithRequiredAntiAffinity) > 0) != (len(clone.PodsWithRequiredAntiAffinity) > 0) {
				updateNodesHavePodsWithRequiredAntiAffinity = true
			}
			if !updateUsedPVCSet {
				if len(existing.PVCRefCounts) != len(clone.PVCRefCounts) {
					updateUsedPVCSet = true
				} else {
					for pvcKey := range clone.PVCRefCounts {
						if _, found := existing.PVCRefCounts[pvcKey]; !found {
							updateUsedPVCSet = true
							break
						}
					}
				}
			}
			// We need to preserve the original pointer of the NodeInfo struct since it
			// is used in the NodeInfoList, which we may not update.
			// 将NodeInfo的拷贝更新到nodeSnapshot中
			*existing = *clone
		}
	}
	// Update the snapshot generation with the latest NodeInfo generation.
	// Cache的表头Node的版本是最新的，所以也就代表了此时更新镜像后镜像的版本了
	if cache.headNode != nil {
		nodeSnapshot.generation = cache.headNode.info.Generation
	}

	// Comparing to pods in nodeTree.
	// Deleted nodes get removed from the tree, but they might remain in the nodes map
	// if they still have non-deleted Pods.
	// 如果nodeSnapshot中node的数量大于nodeTree中的数量，说明有node被删除
	// 所以要从快照的nodeInfoMap中删除已删除的Node，同时标记需要更新node的全量列表
	if len(nodeSnapshot.nodeInfoMap) > cache.nodeTree.numNodes {
		cache.removeDeletedNodesFromSnapshot(nodeSnapshot)
		updateAllLists = true
	}

    // 如果需要更新Node的全量或者亲和性或者反亲和性列表，则更新nodeSnapshot中的Node列表
	if updateAllLists || updateNodesHavePodsWithAffinity || updateNodesHavePodsWithRequiredAntiAffinity || updateUsedPVCSet {
		cache.updateNodeInfoSnapshotList(nodeSnapshot, updateAllLists)
	}
    // 如果此时nodeSnapshot的node列表与nodeTree的数量还不一致，需要再做一次node全列表更新
    // 此处应该是一个保险操作，理论上不会发生，谁知道会不会有Bug发生呢？多一些容错没有坏处
	if len(nodeSnapshot.nodeInfoList) != cache.nodeTree.numNodes {
		errMsg := fmt.Sprintf("snapshot state is not consistent, length of NodeInfoList=%v not equal to length of nodes in tree=%v "+
			", length of NodeInfoMap=%v, length of nodes in cache=%v"+
			", trying to recover",
			len(nodeSnapshot.nodeInfoList), cache.nodeTree.numNodes,
			len(nodeSnapshot.nodeInfoMap), len(cache.nodes))
		klog.ErrorS(nil, errMsg)
		// We will try to recover by re-creating the lists for the next scheduling cycle, but still return an
		// error to surface the problem, the error will likely cause a failure to the current scheduling cycle.
		cache.updateNodeInfoSnapshotList(nodeSnapshot, true)
		return fmt.Errorf(errMsg)
	}

	return nil
}

// 先思考一个问题：为什么有Node添加或者删除需要更新快照中的全量列表？如果是Node删除了，
// 需要找到Node在全量列表中的位置，然后删除它，最悲观的复杂度就是遍历一遍列表，然后再挪动它后面的Node
// 因为快照的Node列表是用slice实现，所以一旦快照中Node列表有任何更新，复杂度都是Node的数量。
// 那如果是有新的Node添加呢？并不知道应该插在哪里，所以重新创建一次全量列表最为简单有效。
// 亲和性和反亲和性列表道理也是一样的。
func (cache *cacheImpl) updateNodeInfoSnapshotList(snapshot *Snapshot, updateAll bool) {
    // 快照创建亲和性和反亲和性列表
	snapshot.havePodsWithAffinityNodeInfoList = make([]*framework.NodeInfo, 0, cache.nodeTree.numNodes)
	snapshot.havePodsWithRequiredAntiAffinityNodeInfoList = make([]*framework.NodeInfo, 0, cache.nodeTree.numNodes)
	snapshot.usedPVCSet = sets.NewString()
	if updateAll {
        // 创建快照全量列表
		// Take a snapshot of the nodes order in the tree
		snapshot.nodeInfoList = make([]*framework.NodeInfo, 0, cache.nodeTree.numNodes)
		nodesList, err := cache.nodeTree.list()
		if err != nil {
			klog.ErrorS(err, "Error occurred while retrieving the list of names of the nodes from node tree")
		}
        // 遍历nodeTree的Node
		for _, nodeName := range nodesList {
			if nodeInfo := snapshot.nodeInfoMap[nodeName]; nodeInfo != nil {
				snapshot.nodeInfoList = append(snapshot.nodeInfoList, nodeInfo)
                // 追加全量、亲和性(按需)、反亲和性列表(按需)
				if len(nodeInfo.PodsWithAffinity) > 0 {
					snapshot.havePodsWithAffinityNodeInfoList = append(snapshot.havePodsWithAffinityNodeInfoList, nodeInfo)
				}
				if len(nodeInfo.PodsWithRequiredAntiAffinity) > 0 {
					snapshot.havePodsWithRequiredAntiAffinityNodeInfoList = append(snapshot.havePodsWithRequiredAntiAffinityNodeInfoList, nodeInfo)
				}
				for key := range nodeInfo.PVCRefCounts {
					snapshot.usedPVCSet.Insert(key)
				}
			} else {
				klog.ErrorS(nil, "Node exists in nodeTree but not in NodeInfoMap, this should not happen", "node", klog.KRef("", nodeName))
			}
		}
	} else {
        // 如果更新全量列表，只需要遍历快照中的全量列表就可以了
		for _, nodeInfo := range snapshot.nodeInfoList {
			if len(nodeInfo.PodsWithAffinity) > 0 {
				snapshot.havePodsWithAffinityNodeInfoList = append(snapshot.havePodsWithAffinityNodeInfoList, nodeInfo)
			}
			if len(nodeInfo.PodsWithRequiredAntiAffinity) > 0 {
				snapshot.havePodsWithRequiredAntiAffinityNodeInfoList = append(snapshot.havePodsWithRequiredAntiAffinityNodeInfoList, nodeInfo)
			}
			for key := range nodeInfo.PVCRefCounts {
				snapshot.usedPVCSet.Insert(key)
			}
		}
	}
}

```


想想快照中Node的列表用来干什么？前面应该提到了，就是map没有任何序，而列表按照nodeTree排序，对于调度更有利，读者应该能想明白了。


# 总结

1. Cache缓存了Pod和Node信息，并且Node信息聚合了运行在该Node上所有Pod的资源量和镜像信息；Node有虚实之分，已删除的Node，Cache不会立刻删除它，而是继续维护一个虚的Node，直到Node上的Pod清零后才会被删除；但是nodeTree中维护的是实际的Node，调度使用nodeTree就可以避免将Pod调度到虚Node上；
2. kube-scheduler利用client-go监控(watch)Pod和Node状态，当有事件发生时调用Cache的AddPod，RemovePod，UpdatePod，AddNode，RemoveNode，UpdateNode更新Cache中Pod和Node的状态，这样kube-scheduler开始新一轮调度的时候可以获得最新的状态；
3. kube-scheduler每一轮调度都会调用UpdateSnapshot更新本地(局部变量)的Node状态，因为Cache中的Node按照最近更新排序，只需要将Cache中Node.Generation大于kube-scheduler本地的快照generation的Node更新到snapshot中即可，这样可以避免大量不必要的拷贝；
4. kube-scheduler找到合适的Node调度Pod后，需要调用Cache.AssumePod假定Pod已调度，然后启动协程异步Bind Pod到Node上，当Pod完成Bind后，调用Cache.FinishBinding通知Cache；
5. kube-scheudler调用Cache.AssumePod后续的所有造作一旦有错误就会调用Cache.ForgetPod删除假定的Pod，释放资源；
6. 完成Bind的Pod默认超时为30秒，Cache有一个协程定时(1秒)清理超时的Bind超时的Pod，如果超时依然没有收到Pod确认消息(调用AddPod)，则将删除超时的Pod，进而释放出Cache.AssumePod占用的资源;
7. Cache的核心功能就是统计Node的调度状态(比如累加Pod的资源量、统计镜像)，然后以镜像的形式输出给kube-scheduler，kube-scheduler从调度队列(SchedulingQueue)中取出等待调度的Pod，根据镜像计算最合适的Node；

此时再来看看源码中关于Pod状态机的注释就非常容易理解了：

```go
// State Machine of a pod's events in scheduler's cache:
//
//
//   +-------------------------------------------+  +----+
//   |                            Add            |  |    |
//   |                                           |  |    | Update
//   +      Assume                Add            v  v    |
//Initial +--------> Assumed +------------+---> Added <--+
//   ^                +   +               |       +
//   |                |   |               |       |
//   |                |   |           Add |       | Remove
//   |                |   |               |       |
//   |                |   |               +       |
//   +----------------+   +-----------> Expired   +----> Deleted
//         Forget             Expire
//
```

上面总结中描述了kube-scheduler大致调度一个Pod的流程，其实kube-scheduler调度一个Pod的流程非常复杂，此处为了方便理解Cache在kube-scheduler中的位置和作用
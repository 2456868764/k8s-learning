# 事情处理

事件处理主要介绍：
1. 需要调度的Pod是怎么进入[调度队列](./scheduling_queue.md)的？
2. 调度完成的Pod是如何被更新到[调度缓存](./cache.md)的？
3. Node的上线、下线等状态更新是如何更新到[调度缓存](./cache.md)的？
4. 调度插件依赖的各种资源(比如PV)是如何影响调度的？

## 1. 注册

来看看kube-scheduler总共注册了哪些对象事件处理函数，在后续的章节中笔者会一一解析每个事件的处理函数。

在 [调度队列](./scheduling_queue.md) 简单介绍 Pod 是如何加到activeQ活动队列。 是通过 在创建 Scheduler.New() 对象时调用 addAllEventHandlers()方法

```golang
// /pkg/scheduler/scheduler.go
// New returns a Scheduler
func New(client clientset.Interface,
	informerFactory informers.SharedInformerFactory,
	dynInformerFactory dynamicinformer.DynamicSharedInformerFactory,
	recorderFactory profile.RecorderFactory,
	stopCh <-chan struct{},
	opts ...Option) (*Scheduler, error) {
	
	// ...

	addAllEventHandlers(sched, informerFactory, dynInformerFactory, unionedGVKs(clusterEventMap))

	return sched, nil
}
```

现在详细的来看 addAllEventHandlers 的源码。

```golang

// addAllEventHandlers is a helper function used in tests and in Scheduler
// to add event handlers for various informers.
// addAllEventHandlers()用来注册kube-scheduler所有的事件处理函数，其中sched是调度器对象。
// 而informerFactory是kube-scheduler所有模块共享使用的SharedIndexInformer工厂。
func addAllEventHandlers(
	sched *Scheduler,
	informerFactory informers.SharedInformerFactory,
	dynInformerFactory dynamicinformer.DynamicSharedInformerFactory,
	gvkMap map[framework.GVK]framework.ActionType,
) {
	// scheduled pod cache
	// 注册已调度Pod事件处理函数，这就要从过滤条件进行区分了。
	informerFactory.Core().V1().Pods().Informer().AddEventHandler(
		cache.FilteringResourceEventHandler{
            // 事件过滤函数
			FilterFunc: func(obj interface{}) bool {
                // 如果对象是Pod，只有已调度的Pod才会通过过滤条件，assignedPod()函数判断len(pod.Spec.NodeName) != 0。
                // 所以此处断定是为已调度的Pod注册事件处理函数
				switch t := obj.(type) {
				case *v1.Pod:
					return assignedPod(t)
                // 与上一个case相同，只是Pod处于已删除状态而已
				case cache.DeletedFinalStateUnknown:
					if _, ok := t.Obj.(*v1.Pod); ok {
						// The carried object may be stale, so we don't use it to check if
						// it's assigned or not. Attempting to cleanup anyways.
						return true
					}
					utilruntime.HandleError(fmt.Errorf("unable to convert object %T to *v1.Pod in %T", obj, sched))
					return false
                // 不是Pod类型，自然也就不可能通过过滤条件，当然这种可能性应该不大	
				default:
					utilruntime.HandleError(fmt.Errorf("unable to handle object in %T: %T", sched, obj))
					return false
				}
			},
            // 注册已调度Pod事件处理函数
            // 至此，结合调度框架相关的知识，总结如下：
            // 1. 调度框架异步调用绑定插件将绑定子对象写入apiserver；
            // 2. SharedIndexInformer感知到Pod的状态更新，因为Pod.Spec.NodeName被设置，所以会通过过滤条件；
            // 3. kube-scheduler根据Pod的事件进行处理，进而更新Cache的状态；
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc:    sched.addPodToCache,
				UpdateFunc: sched.updatePodInCache,
				DeleteFunc: sched.deletePodFromCache,
			},
		},
	)
	// unscheduled pod queue
    // 注册未调度Pod事件处理函数，同样的道理，根据Pod的状态过滤未调度的Pod
	informerFactory.Core().V1().Pods().Informer().AddEventHandler(
		cache.FilteringResourceEventHandler{
			FilterFunc: func(obj interface{}) bool {
				switch t := obj.(type) {
                // 对象类型为Pod
				case *v1.Pod:
				    // 不仅Pod.Spec.NodeName没有被设置，同时Pod.Spec.SchedulerName指定的调度器是存在的。
				    // 那么问题来了，Pod.Spec.SchedulerName指定的调度器不存在怎么办？无非是进不了调度队列，状态长期处于Pending而已。
					return !assignedPod(t) && responsibleForPod(t, sched.Profiles)
                // 同上，只是Pod处于已删除状态	
				case cache.DeletedFinalStateUnknown:
					if pod, ok := t.Obj.(*v1.Pod); ok {
						// The carried object may be stale, so we don't use it to check if
						// it's assigned or not.
						return responsibleForPod(pod, sched.Profiles)
					}
					utilruntime.HandleError(fmt.Errorf("unable to convert object %T to *v1.Pod in %T", obj, sched))
					return false
				default:
					utilruntime.HandleError(fmt.Errorf("unable to handle object in %T: %T", sched, obj))
					return false
				}
			},
			// 注册未调度Pod事件处理函数
            // 无论是用kubectl创建的Pod还是各种controller创建的Pod，都是先在apiserver中创建对象，再通过apiserver通知到kube-scheduler
			Handler: cache.ResourceEventHandlerFuncs{
				AddFunc:    sched.addPodToSchedulingQueue,
				UpdateFunc: sched.updatePodInSchedulingQueue,
				DeleteFunc: sched.deletePodFromSchedulingQueue,
			},
		},
	)

    // 注册Node事件处理函数
	informerFactory.Core().V1().Nodes().Informer().AddEventHandler(
		cache.ResourceEventHandlerFuncs{
			AddFunc:    sched.addNodeToCache,
			UpdateFunc: sched.updateNodeInCache,
			DeleteFunc: sched.deleteNodeFromCache,
		},
	)

	buildEvtResHandler := func(at framework.ActionType, gvk framework.GVK, shortGVK string) cache.ResourceEventHandlerFuncs {
		funcs := cache.ResourceEventHandlerFuncs{}
		if at&framework.Add != 0 {
			evt := framework.ClusterEvent{Resource: gvk, ActionType: framework.Add, Label: fmt.Sprintf("%vAdd", shortGVK)}
			funcs.AddFunc = func(_ interface{}) {
				sched.SchedulingQueue.MoveAllToActiveOrBackoffQueue(evt, nil)
			}
		}
		if at&framework.Update != 0 {
			evt := framework.ClusterEvent{Resource: gvk, ActionType: framework.Update, Label: fmt.Sprintf("%vUpdate", shortGVK)}
			funcs.UpdateFunc = func(_, _ interface{}) {
				sched.SchedulingQueue.MoveAllToActiveOrBackoffQueue(evt, nil)
			}
		}
		if at&framework.Delete != 0 {
			evt := framework.ClusterEvent{Resource: gvk, ActionType: framework.Delete, Label: fmt.Sprintf("%vDelete", shortGVK)}
			funcs.DeleteFunc = func(_ interface{}) {
				sched.SchedulingQueue.MoveAllToActiveOrBackoffQueue(evt, nil)
			}
		}
		return funcs
	}

	for gvk, at := range gvkMap {
		switch gvk {
		case framework.Node, framework.Pod:
			// Do nothing.
		case framework.CSINode:
            // 注册CSINode事件处理函数
			informerFactory.Storage().V1().CSINodes().Informer().AddEventHandler(
				buildEvtResHandler(at, framework.CSINode, "CSINode"),
			)
		case framework.CSIDriver:
             // 注册CSIDriver事件处理函数
			informerFactory.Storage().V1().CSIDrivers().Informer().AddEventHandler(
				buildEvtResHandler(at, framework.CSIDriver, "CSIDriver"),
			)
		case framework.CSIStorageCapacity:
             // 注册CSIStorageCapacity事件处理函数
			informerFactory.Storage().V1().CSIStorageCapacities().Informer().AddEventHandler(
				buildEvtResHandler(at, framework.CSIStorageCapacity, "CSIStorageCapacity"),
			)
			
		case framework.PersistentVolume:
			// MaxPDVolumeCountPredicate: since it relies on the counts of PV.
			//
			// PvAdd: Pods created when there are no PVs available will be stuck in
			// unschedulable queue. But unbound PVs created for static provisioning and
			// delay binding storage class are skipped in PV controller dynamic
			// provisioning and binding process, will not trigger events to schedule pod
			// again. So we need to move pods to active queue on PV add for this
			// scenario.
			//
			// PvUpdate: Scheduler.bindVolumesWorker may fail to update assumed pod volume
			// bindings due to conflicts if PVs are updated by PV controller or other
			// parties, then scheduler will add pod back to unschedulable queue. We
			// need to move pods to active queue on PV update for this scenario.
            // 注册PV事件处理函数，熟悉调度插件的读者应该不陌生，因为部分调度插件依赖PV，比如Pod需要挂载块设备
			informerFactory.Core().V1().PersistentVolumes().Informer().AddEventHandler(
				buildEvtResHandler(at, framework.PersistentVolume, "Pv"),
			)
		case framework.PersistentVolumeClaim:
		    // 注册PVC事件处理函数，没有PVC只有PV也是没用的，所以不用解释了
			// MaxPDVolumeCountPredicate: add/update PVC will affect counts of PV when it is bound.
			informerFactory.Core().V1().PersistentVolumeClaims().Informer().AddEventHandler(
				buildEvtResHandler(at, framework.PersistentVolumeClaim, "Pvc"),
			)
		case framework.PodSchedulingContext:
            // 注册PodSchedulingContext事件处理函数
			if utilfeature.DefaultFeatureGate.Enabled(features.DynamicResourceAllocation) {
				_, _ = informerFactory.Resource().V1alpha2().PodSchedulingContexts().Informer().AddEventHandler(
					buildEvtResHandler(at, framework.PodSchedulingContext, "PodSchedulingContext"),
				)
			}
		case framework.ResourceClaim:
		    // 注册ResourceClaim事件处理函数
			if utilfeature.DefaultFeatureGate.Enabled(features.DynamicResourceAllocation) {
				_, _ = informerFactory.Resource().V1alpha2().ResourceClaims().Informer().AddEventHandler(
					buildEvtResHandler(at, framework.ResourceClaim, "ResourceClaim"),
				)
			}
		case framework.StorageClass:
		    // 注册StorageClass事件处理函数
			if at&framework.Add != 0 {
				informerFactory.Storage().V1().StorageClasses().Informer().AddEventHandler(
					cache.ResourceEventHandlerFuncs{
						AddFunc: sched.onStorageClassAdd,
					},
				)
			}
			if at&framework.Update != 0 {
				informerFactory.Storage().V1().StorageClasses().Informer().AddEventHandler(
					cache.ResourceEventHandlerFuncs{
						UpdateFunc: func(_, _ interface{}) {
							sched.SchedulingQueue.MoveAllToActiveOrBackoffQueue(queue.StorageClassUpdate, nil)
						},
					},
				)
			}
		default:
			// Tests may not instantiate dynInformerFactory.
			if dynInformerFactory == nil {
				continue
			}
			// GVK is expected to be at least 3-folded, separated by dots.
			// <kind in plural>.<version>.<group>
			// Valid examples:
			// - foos.v1.example.com
			// - bars.v1beta1.a.b.c
			// Invalid examples:
			// - foos.v1 (2 sections)
			// - foo.v1.example.com (the first section should be plural)
			if strings.Count(string(gvk), ".") < 2 {
				klog.ErrorS(nil, "incorrect event registration", "gvk", gvk)
				continue
			}
			// Fall back to try dynamic informers.
			gvr, _ := schema.ParseResourceArg(string(gvk))
			dynInformer := dynInformerFactory.ForResource(*gvr).Informer()
			dynInformer.AddEventHandler(
				buildEvtResHandler(at, gvk, strings.Title(gvr.Resource)),
			)
		}
	}
}


```

## 已调度Pod 

```golang
// addPodToCache()是kube-scheduler处理已调度Pod的Added事件的函数。
func (sched *Scheduler) addPodToCache(obj interface{}) {
	pod, ok := obj.(*v1.Pod)
	if !ok {
		klog.ErrorS(nil, "Cannot convert to *v1.Pod", "obj", obj)
		return
	}
	klog.V(3).InfoS("Add event for scheduled pod", "pod", klog.KObj(pod))

    // 函数的核心应该就是调用调度缓存(Cache)的接口将Pod添加到Cache中。
	if err := sched.Cache.AddPod(pod); err != nil {
		klog.ErrorS(err, "Scheduler cache AddPod failed", "pod", klog.KObj(pod))
	}
	
    // 这里有点意思了，已调度的Pod还要添加到调度缓存？还记得SchedulingQueue.AssignedPodAdded()这个函数的作用么？
    // 其实这段代码的用意就是通知调度队列收到了已调度(Assigned指的是已分配Node)pod添加的消息，
    // 那么因为依赖该Pod被放入不可调度自子队列的Pod可以考虑进入active或者退避子队列了。
    // 所以此处并不是向调度队列添加Pod，而是触发调度队列更新可能依赖于此Pod的其他Pod的调度状态。
	sched.SchedulingQueue.AssignedPodAdded(pod)
}

// updatePodInCache()是kube-scheduler处理已调度Pod的Updated事件的函数。
func (sched *Scheduler) updatePodInCache(oldObj, newObj interface{}) {
	oldPod, ok := oldObj.(*v1.Pod)
	if !ok {
		klog.ErrorS(nil, "Cannot convert oldObj to *v1.Pod", "oldObj", oldObj)
		return
	}
	newPod, ok := newObj.(*v1.Pod)
	if !ok {
		klog.ErrorS(nil, "Cannot convert newObj to *v1.Pod", "newObj", newObj)
		return
	}
	klog.V(4).InfoS("Update event for scheduled pod", "pod", klog.KObj(oldPod))

    // 更新调度缓存(Cache)中的Pod。
	if err := sched.Cache.UpdatePod(oldPod, newPod); err != nil {
		klog.ErrorS(err, "Scheduler cache UpdatePod failed", "pod", klog.KObj(oldPod))
	}
    // 在addPodToCache()已经解释过，此处不赘述
	sched.SchedulingQueue.AssignedPodUpdated(newPod)
}
// deletePodFromCache()是kube-scheduler处理已调度Pod的Deleted事件的函数。
func (sched *Scheduler) deletePodFromCache(obj interface{}) {
	var pod *v1.Pod
	switch t := obj.(type) {
	case *v1.Pod:
		pod = t
	case cache.DeletedFinalStateUnknown:
		var ok bool
		pod, ok = t.Obj.(*v1.Pod)
		if !ok {
			klog.ErrorS(nil, "Cannot convert to *v1.Pod", "obj", t.Obj)
			return
		}
	default:
		klog.ErrorS(nil, "Cannot convert to *v1.Pod", "obj", t)
		return
	}
	klog.V(3).InfoS("Delete event for scheduled pod", "pod", klog.KObj(pod))
    // 将Pod从调度缓存(Cache)中删除
	if err := sched.Cache.RemovePod(pod); err != nil {
		klog.ErrorS(err, "Scheduler cache RemovePod failed", "pod", klog.KObj(pod))
	}

   // 与Pod的Added和Updated事件的处理方法类似，前两个都是更新依赖该Pod的Pod调度状态。
   // 对于Deleted事件则是更新所有不可调度的Pod的调度状态，笔者猜测应该暂时没有办法确定由于Pod的删除会影响哪些Pod调，所以采用如此简单粗暴的方法。
	sched.SchedulingQueue.MoveAllToActiveOrBackoffQueue(queue.AssignedPodDelete, nil)
}

```

## 未调度Pod

再来看看未调度Pod的事件处理函数

```golang
// addPodToSchedulingQueue()是kube-scheduler处理未调度Pod的Added事件的函数。
// 比如通过kubectl创建的Pod，controller创建的Pod等等
func (sched *Scheduler) addPodToSchedulingQueue(obj interface{}) {
	pod := obj.(*v1.Pod)
	klog.V(3).InfoS("Add event for unscheduled pod", "pod", klog.KObj(pod))
    // 函数实现很简单，直接添加到调度队列中
	if err := sched.SchedulingQueue.Add(pod); err != nil {
		utilruntime.HandleError(fmt.Errorf("unable to queue %T: %v", obj, err))
	}
}

// updatePodInSchedulingQueue()是kube-scheduler处理未调度Pod的Updated事件的函数。
// 还处在调度队列中Pod被更新，比如需更新资源需求、亲和性、标签等
func (sched *Scheduler) updatePodInSchedulingQueue(oldObj, newObj interface{}) {
	oldPod, newPod := oldObj.(*v1.Pod), newObj.(*v1.Pod)
	// Bypass update event that carries identical objects; otherwise, a duplicated
	// Pod may go through scheduling and cause unexpected behavior (see #96071).
    // 如果对象版本号没有更新则忽略，重复的Pod可能导致意外行为。那么问题来了:
    // 1. 为什么会有相同版本的对象的更新事件？不是只有对象更新的时候才会触发更新事件么？一旦对象更新版本肯定会更新。
    // 2. 即便版本号不变，说明对象也没有变化，会带来什么意外的行为？
    // 如果SharedIndexInformer设置了ResyncPeriod，那么就会周期性的重新同步对象，就会触发此类情况。
    // 至于会导致什么意外行为，其实没有人能够确定，只是这样做会避免不必要的处理，同时也就避免了出现问题的可能。
	if oldPod.ResourceVersion == newPod.ResourceVersion {
		return
	}

	// 判断是已经 假定POD
	isAssumed, err := sched.Cache.IsAssumedPod(newPod)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to check whether pod %s/%s is assumed: %v", newPod.Namespace, newPod.Name, err))
	}
	if isAssumed {
		return
	}
    // 更新调度队列中的Pod
	if err := sched.SchedulingQueue.Update(oldPod, newPod); err != nil {
		utilruntime.HandleError(fmt.Errorf("unable to update %T: %v", newObj, err))
	}
}

// deletePodFromSchedulingQueue()是kube-scheduler处理未调度Pod的Deleted事件的函数。
// 还处于调度队列中的Pod被删除，这种情况很常见，比如利用kubectl创建的Pod一直处于Pending状态，
// 人工分析发现某些字段有问题，然后再用kubectl删除掉该Pod，至少笔者经常这么干...
func (sched *Scheduler) deletePodFromSchedulingQueue(obj interface{}) {
	var pod *v1.Pod
	switch t := obj.(type) {
	case *v1.Pod:
		pod = obj.(*v1.Pod)
	case cache.DeletedFinalStateUnknown:
		var ok bool
		pod, ok = t.Obj.(*v1.Pod)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("unable to convert object %T to *v1.Pod in %T", obj, sched))
			return
		}
	default:
		utilruntime.HandleError(fmt.Errorf("unable to handle object in %T: %T", sched, obj))
		return
	}
	klog.V(3).InfoS("Delete event for unscheduled pod", "pod", klog.KObj(pod))
    // 将Pod从调度队列中删除
	if err := sched.SchedulingQueue.Delete(pod); err != nil {
		utilruntime.HandleError(fmt.Errorf("unable to dequeue %T: %v", obj, err))
	}
    // 根据Pod.Spec.SchedulerName获取调度框架(调度器)
	fwk, err := sched.frameworkForPod(pod)
	if err != nil {
		// This shouldn't happen, because we only accept for scheduling the pods
		// which specify a scheduler name that matches one of the profiles.
		klog.ErrorS(err, "Unable to get profile", "pod", klog.KObj(pod))
		return
	}
	// If a waiting pod is rejected, it indicates it's previously assumed and we're
	// removing it from the scheduler cache. In this case, signal a AssignedPodDelete
	// event to immediately retry some unscheduled Pods.
	// 弹出等待的Pod，了解调度插件的读者应该不陌生，PermitPlugin插件会让Pod等待，所以如果Pod处于等待状态则从等待类表中删除。
	// 因为等待Pod是单独管理的(WaitingPod)，并且Pod没有字段指定其是否为等待状态，所以这是"盲目"删除。
	if fwk.RejectWaitingPod(pod.UID) {
		sched.SchedulingQueue.MoveAllToActiveOrBackoffQueue(queue.AssignedPodDelete, nil)
	}
}

```

## Node

已经知道kube-scheduler如何处理Pod事件，接下来看看如何处理Node的事件

```golang

func (sched *Scheduler) addNodeToCache(obj interface{}) {
   // 将interface{}转为Node
	node, ok := obj.(*v1.Node)
	if !ok {
		klog.ErrorS(nil, "Cannot convert to *v1.Node", "obj", obj)
		return
	}
    // 将Node添加到调度缓存(Cache)
	nodeInfo := sched.Cache.AddNode(node)
	klog.V(3).InfoS("Add event for node", "node", klog.KObj(node))
    // 因为有新的Node添加，所有不可调度的Pod都有可能可以被调度了，这种实现还是相对简单粗暴
	sched.SchedulingQueue.MoveAllToActiveOrBackoffQueue(queue.NodeAdd, preCheckForNode(nodeInfo))
}


// updateNodeInCache()是kube-scheduler处理Node的Updated事件的函数。比如，集群管理员为Node打了标签。
func (sched *Scheduler) updateNodeInCache(oldObj, newObj interface{}) {
    // 将interface{}转为Node
	oldNode, ok := oldObj.(*v1.Node)
	if !ok {
		klog.ErrorS(nil, "Cannot convert oldObj to *v1.Node", "oldObj", oldObj)
		return
	}
	newNode, ok := newObj.(*v1.Node)
	if !ok {
		klog.ErrorS(nil, "Cannot convert newObj to *v1.Node", "newObj", newObj)
		return
	}
   // 更新调度缓存(Cache)中的Node
	nodeInfo := sched.Cache.UpdateNode(oldNode, newNode)
	// Only requeue unschedulable pods if the node became more schedulable.
    // 仅在Node变得更可调度时才重新调度不可调度的Pod，什么叫做更可调度？比如可分配资源增加、以前不可调度现在可调度等。
    // 下面有nodeSchedulingPropertiesChange()的注释。
	if event := nodeSchedulingPropertiesChange(newNode, oldNode); event != nil {
		sched.SchedulingQueue.MoveAllToActiveOrBackoffQueue(*event, preCheckForNode(nodeInfo))
	}
}
// deleteNodeFromCache()是kube-scheduler处理Node的Deleted事件的函数。比如，集群管理员将Node从集群中删除。
func (sched *Scheduler) deleteNodeFromCache(obj interface{}) {
    // 将interface{}转为Node
	var node *v1.Node
	switch t := obj.(type) {
	case *v1.Node:
		node = t
	case cache.DeletedFinalStateUnknown:
		var ok bool
		node, ok = t.Obj.(*v1.Node)
		if !ok {
			klog.ErrorS(nil, "Cannot convert to *v1.Node", "obj", t.Obj)
			return
		}
	default:
		klog.ErrorS(nil, "Cannot convert to *v1.Node", "obj", t)
		return
	}
	klog.V(3).InfoS("Delete event for node", "node", klog.KObj(node))
    // 将Node从调度缓存(Cache)中删除
	if err := sched.Cache.RemoveNode(node); err != nil {
		klog.ErrorS(err, "Scheduler cache RemoveNode failed")
	}
}


// nodeSchedulingPropertiesChange()并不是判断Node是否更可调度，而是判断可调度属性发生变化。
// 如果Node向不可调度变化其实不应该触发重新调度不可调度Pod，但是也不会引起什么问题，无非多一点不必要的计算而已。
// 但是这样的实现比较简单，不容易出错，毕竟什么是更可调度不是一两句话可以说清楚的，而判断是否变化是很容易的。
func nodeSchedulingPropertiesChange(newNode *v1.Node, oldNode *v1.Node) *framework.ClusterEvent {
    // Node.Spec.Unschedulable发生变化
	if nodeSpecUnschedulableChanged(newNode, oldNode) {
		return &queue.NodeSpecUnschedulableChange
	}
    // Node.Status.Allocatable发生变化
	if nodeAllocatableChanged(newNode, oldNode) {
		return &queue.NodeAllocatableChange
	}
    // Node.Labels发生变化
	if nodeLabelsChanged(newNode, oldNode) {
		return &queue.NodeLabelChange
	}
    // Node.Spec.Taints发生变化
	if nodeTaintsChanged(newNode, oldNode) {
		return &queue.NodeTaintChange
	}
    // Node.Status.Conditions发生变化
	if nodeConditionsChanged(newNode, oldNode) {
		return &queue.NodeConditionChange
	}

	return nil
}
```

## CSINode

是时候介绍什么是CSINode了， 请阅读[CSINode官方文档](https://kubernetes-csi.github.io/docs/csi-node-object.html)，
将k8s和存储系统解耦，抽象出了CSI(container storage interface)接口，其提供三种类型的gRPC接口，每个CSI plugin必须实现这些接口，
[CSINode Spec](https://github.com/container-storage-interface/spec/blob/master/spec.md)

```golang
buildEvtResHandler := func(at framework.ActionType, gvk framework.GVK, shortGVK string) cache.ResourceEventHandlerFuncs {
		funcs := cache.ResourceEventHandlerFuncs{}
		if at&framework.Add != 0 {
			evt := framework.ClusterEvent{Resource: gvk, ActionType: framework.Add, Label: fmt.Sprintf("%vAdd", shortGVK)}
			funcs.AddFunc = func(_ interface{}) {
				// 重新调度所有不可调度的Pod
				sched.SchedulingQueue.MoveAllToActiveOrBackoffQueue(evt, nil)
			}
		}
		if at&framework.Update != 0 {
			evt := framework.ClusterEvent{Resource: gvk, ActionType: framework.Update, Label: fmt.Sprintf("%vUpdate", shortGVK)}
			funcs.UpdateFunc = func(_, _ interface{}) {
                // 重新调度所有不可调度的Pod
				sched.SchedulingQueue.MoveAllToActiveOrBackoffQueue(evt, nil)
			}
		}
		if at&framework.Delete != 0 {
			evt := framework.ClusterEvent{Resource: gvk, ActionType: framework.Delete, Label: fmt.Sprintf("%vDelete", shortGVK)}
			funcs.DeleteFunc = func(_ interface{}) {
                // 重新调度所有不可调度的Pod
				sched.SchedulingQueue.MoveAllToActiveOrBackoffQueue(evt, nil)
			}
		}
		return funcs
	}
```
```golang
informerFactory.Storage().V1().CSINodes().Informer().AddEventHandler(
				buildEvtResHandler(at, framework.CSINode, "CSINode"),
			)
```
添加，变更和删除全部是 重新调度所有不可调度的Pod

## 其他资源

CSIDriver， CSIStorageCapacity， PersistentVolume， PersistentVolumeClaim， PodSchedulingContext， ResourceClaim， StorageClass
和 CSINode 是一样区别是 重新调度所有不可调度的Pod。

# 总结

1. kube-scheduler处理了Pod、Node、CSINode、PV、PVC、PodSchedulingContext， ResourceClaim以及StorageClass对象的事件，基于事件的类型(添加、删除、更新)执行对应的操作；
2. 未调度的Pod会被SchedulingQueue和WaitingPod管理，所以删除时需要从这两个对象中删除；
3. Pod、Node的事件处理会根据状态(已调度、未调度)和事件类型操作调度队列和调度缓存；
4. CSINode、PV、PVC、PodSchedulingContext， ResourceClaim以及StorageClass的事件只是重新调度不可调度的Pod，因为这些事件可能会导致一些不可调度的Pod可调度；





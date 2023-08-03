# 调度概览

文档位置： https://kubernetes.io/zh-cn/docs/concepts/scheduling-eviction/

调度器通过 Kubernetes 的监测（Watch）机制来发现集群中新创建且尚未被调度到节点上的 Pod。 调度器会将所发现的每一个未调度的 Pod 调度到一个合适的节点上来运行。 

## kube-scheduler

kube-scheduler 是 Kubernetes 集群的默认调度器，并且是集群 控制面 的一部分。 如果你真得希望或者有这方面的需求，kube-scheduler 在设计上允许你自己编写一个调度组件并替换原有的 kube-scheduler。

Kube-scheduler 选择一个最佳节点来运行新创建的或尚未调度（unscheduled）的 Pod。 由于 Pod 中的容器和 Pod 本身可能有不同的要求，调度程序会过滤掉任何不满足 Pod 特定调度需求的节点。 或者，API 允许你在创建 Pod 时为它指定一个节点，但这并不常见，并且仅在特殊情况下才会这样做。

在一个集群中，满足一个 Pod 调度请求的所有节点称之为 可调度节点。 如果没有任何一个节点能满足 Pod 的资源请求， 那么这个 Pod 将一直停留在未调度状态直到调度器能够找到合适的 Node。

调度器先在集群中找到一个 Pod 的所有可调度节点，然后根据一系列函数对这些可调度节点打分， 选出其中得分最高的节点来运行 Pod。之后，调度器将这个调度决定通知给 kube-apiserver，这个过程叫做 绑定。

在做调度决定时需要考虑的因素包括：单独和整体的资源请求、硬件/软件/策略限制、 亲和以及反亲和要求、数据局部性、负载间的干扰等等。

Pod 是 Kubernetes 中最小的调度单元，Pod 被创建出来的工作流程如图所示：

![img.png](images/img1.png)


在这张图中: 
- 第一步通过 apiserver REST API 创建一个 Pod。
- 然后 apiserver 接收到数据后将数据写入到 etcd 中。
- 由于 kube-scheduler 通过 apiserver watch API 一直在监听资源的变化，这个时候发现有一个新的 Pod，但是这个时候该 Pod 还没和任何 Node 节点进行绑定，所以 kube-scheduler 就进行调度，选择出一个合适的 Node 节点，将该 Pod 和该目标 Node 进行绑定。绑定之后再更新消息到 etcd 中。
- 这个时候一样的目标 Node 节点上的 kubelet 通过 apiserver watch API 检测到有一个新的 Pod 被调度过来了，他就将该 Pod 的相关数据传递给后面的容器运行时(container runtime)，比如 Docker，让他们去运行该 Pod。
- 而且 kubelet 还会通过 container runtime 获取 Pod 的状态，然后更新到 apiserver 中，当然最后也是写入到 etcd 中去的。

整个过程中最重要的就是 apiserver watch API 和kube-scheduler的调度策略。

调度主要分为以下几个部分：

- 首先是预选过程，过滤掉不满足条件的节点，这个过程称为Predicates（过滤）
- 然后是优选过程，对通过的节点按照优先级排序，称之为Priorities（打分）
- 最后从中选择优先级最高的节点，如果中间任何一步骤有错误，就直接返回错误

过滤阶段会将所有满足 Pod 调度需求的节点选出来。 例如，PodFitsResources 过滤函数会检查候选节点的可用资源能否满足 Pod 的资源请求。 在过滤之后，得出一个节点列表，里面包含了所有可调度节点；通常情况下， 这个节点列表包含不止一个节点。如果这个列表是空的，代表这个 Pod 不可调度。

在打分阶段，调度器会为 Pod 从所有可调度节点中选取一个最合适的节点。 根据当前启用的打分规则，调度器会给每一个可调度节点进行打分。

最后，kube-scheduler 会将 Pod 调度到得分最高的节点上。 如果存在多个得分最高的节点，kube-scheduler 会从中随机选取一个。


## 调度框架


调度框架是面向 Kubernetes 调度器的一种插件架构， 它为现有的调度器添加了一组新的“插件” API。插件会被编译到调度器之中。 这些 API 允许大多数调度功能以插件的形式实现，同时使调度“核心”保持简单且可维护。

调度框架定义了一组扩展点，用户可以实现扩展点定义的接口来定义自己的调度逻辑（称之为扩展），并将扩展注册到扩展点上，调度框架在执行调度工作流时，遇到对应的扩展点时，将调用用户注册的扩展。调度框架在预留扩展点时，都是有特定的目的，有些扩展点上的扩展可以改变调度程序的决策方法，有些扩展点上的扩展只是发送一个通知。

每次调度一个 Pod 的尝试都分为两个阶段，即 调度周期 和 绑定周期。

### 1. 调度周期和绑定周期

调度过程为 Pod 选择一个合适的节点，绑定过程则将调度过程的决策应用到集群中（也就是在被选定的节点上运行 Pod），将调度过程和绑定过程合在一起，称之为调度上下文（scheduling context）。

需要注意的是调度过程是同步运行的（同一时间点只为一个 Pod 进行调度），绑定过程可异步运行（同一时间点可并发为多个 Pod 执行绑定）。

调度过程和绑定过程遇到如下情况时会中途退出：
- 调度程序认为当前没有该 Pod 的可选节点
- 内部错误

这个时候，该 Pod 将被放回到 待调度队列，并等待下次重试。

### 2. 扩展点

下图展示了调度框架中的调度上下文及其中的扩展点，一个扩展可以注册多个扩展点，以便可以执行更复杂的有状态的任务。


![img.png](images/img2.png)

1. PreEnqueue

这些插件在将 Pod 被添加到内部活动队列之前被调用，在此队列中 Pod 被标记为准备好进行调度。

只有当所有 PreEnqueue 插件返回 Success 时，Pod 才允许进入活动队列。 否则，它将被放置在内部无法调度的 Pod 列表中，并且不会获得 Unschedulable 状态。

要了解有关内部调度器队列如何工作的更多详细信息，请阅读 kube-scheduler 调度队列。

2. 队列排序

这些插件用于对调度队列中的 Pod 进行排序。 队列排序插件本质上提供 Less(Pod1, Pod2) 函数。 一次只能启动一个队列插件。

3. PreFilter

这些插件用于预处理 Pod 的相关信息，或者检查集群或 Pod 必须满足的某些条件。 如果 PreFilter 插件返回错误，则调度周期将终止。

4. Filter

这些插件用于过滤出不能运行该 Pod 的节点。对于每个节点， 调度器将按照其配置顺序调用这些过滤插件。如果任何过滤插件将节点标记为不可行， 则不会为该节点调用剩下的过滤插件。节点可以被同时进行评估。

5. PostFilter

这些插件在 Filter 阶段后调用，但仅在该 Pod 没有可行的节点时调用。 插件按其配置的顺序调用。如果任何 PostFilter 插件标记节点为“Schedulable”， 则其余的插件不会调用。典型的 PostFilter 实现是抢占，试图通过抢占其他 Pod 的资源使该 Pod 可以调度。

6. PreScore

这些插件用于执行 “前置评分（pre-scoring）” 工作，即生成一个可共享状态供 Score 插件使用。 如果 PreScore 插件返回错误，则调度周期将终止。

7. Score

这些插件用于对通过过滤阶段的节点进行排序。调度器将为每个节点调用每个评分插件。 将有一个定义明确的整数范围，代表最小和最大分数。 在标准化评分阶段之后，调度器将根据配置的插件权重 合并所有插件的节点分数。

8. NormalizeScore

这些插件用于在调度器计算 Node 排名之前修改分数。 在此扩展点注册的插件被调用时会使用同一插件的 Score 结果。 每个插件在每个调度周期调用一次。


9. Reserve

Reserve 是一个通知性质的扩展点，有状态的插件可以使用该扩展点来获得节点上为 Pod 预留的资源，该事件发生在调度器将 Pod 绑定到节点之前，目的是避免调度器在等待 Pod 与节点绑定的过程中调度新的 Pod 到节点上时，发生实际使用资源超出可用资源的情况（因为绑定 Pod 到节点上是异步发生的）。这是调度过程的最后一个步骤，Pod 进入 reserved 状态以后，要么在绑定失败时触发 Unreserve 扩展，要么在绑定成功时，由 Post-bind 扩展结束绑定过程；

10. Permit

Permit 插件在每个 Pod 调度周期的最后调用，用于防止或延迟 Pod 的绑定。 一个允许插件可以做以下三件事之一：

- 批准  : 一旦所有 Permit 插件批准 Pod 后，该 Pod 将被发送以进行绑定。
- 拒绝 : 如果任何 Permit 插件拒绝 Pod，则该 Pod 将被返回到调度队列。 这将触发 Reserve 插件中的 Unreserve 阶段。
- 等待（带有超时） :如果一个 Permit 插件返回 “等待” 结果，则 Pod 将保持在一个内部的 “等待中” 的 Pod 列表，同时该 Pod 的绑定周期启动时即直接阻塞直到得到批准。 如果超时发生，等待 变成 拒绝，并且 Pod 将返回调度队列，从而触发 Reserve 插件中的 Unreserve 阶段。

11. PreBind

这些插件用于执行 Pod 绑定前所需的所有工作。 例如，一个 PreBind 插件可能需要制备网络卷并且在允许 Pod 运行在该节点之前 将其挂载到目标节点上。

如果任何 PreBind 插件返回错误，则 Pod 将被 拒绝 并且 退回到调度队列中。

12. Bind

Bind 插件用于将 Pod 绑定到节点上。直到所有的 PreBind 插件都完成，Bind 插件才会被调用。 各 Bind 插件按照配置顺序被调用。Bind 插件可以选择是否处理指定的 Pod。 如果某 Bind 插件选择处理某 Pod，剩余的 Bind 插件将被跳过。

13. PostBind

这是个信息性的扩展点。 PostBind 插件在 Pod 成功绑定后被调用。这是绑定周期的结尾，可用于清理相关的资源。


14. Unreserve

这是个信息性的扩展点。 如果 Pod 被保留，然后在后面的阶段中被拒绝，则 Unreserve 插件将被通知。 Unreserve 插件应该清楚保留 Pod 的相关状态。


## 插件 API

插件 API 分为两个步骤。首先，插件必须完成注册并配置，然后才能使用扩展点接口。 扩展点接口具有以下形式。

```golang
type Plugin interface {
    Name() string
}

type QueueSortPlugin interface {
    Plugin
    Less(*v1.pod, *v1.pod) bool
}

type PreFilterPlugin interface {
    Plugin
    PreFilter(context.Context, *framework.CycleState, *v1.pod) error
}

// ...
```

## 插件配置

你可以在调度器配置中启用或禁用插件。 如果你在使用 Kubernetes v1.18 或更高版本，大部分调度 [插件](https://kubernetes.io/zh-cn/docs/reference/scheduling/config/#scheduling-plugins) 都在使用中且默认启用。

除了默认的插件，你还可以实现自己的调度插件并且将它们与默认插件一起配置。 你可以访问 [scheduler-plugins](https://github.com/kubernetes-sigs/scheduler-plugins) 了解更多信息。

如果你正在使用 Kubernetes v1.18 或更高版本，你可以将一组插件设置为 一个调度器配置文件，然后定义不同的配置文件来满足各类工作负载。 了解更多关于[多配置文件](https://kubernetes.io/zh-cn/docs/reference/scheduling/config/#multiple-profiles)。






# featuregate 特性门控

## 概念

特性门控是描述 Kubernetes 特性的一组键值对。你可以在 Kubernetes 的各个组件中使用 --feature-gates 标志来启用或禁用这些特性。

每个 Kubernetes 组件都支持启用或禁用与该组件相关的一组特性门控。 使用 -h 参数来查看所有组件支持的完整特性门控。 要为诸如 kubelet 之类的组件设置特性门控，请使用 --feature-gates 参数， 并向其传递一个特性设置键值对列表：

```shell
--feature-gates=...,GracefulNodeShutdown=true
```

下表总结了在不同的 Kubernetes 组件上可以设置的特性门控。

- 引入特性或更改其发布阶段后，"开始（Since）" 列将包含 Kubernetes 版本。
- "结束（Until）" 列（如果不为空）包含最后一个 Kubernetes 版本，你仍可以在其中使用特性门控。
- 如果某个特性处于 Alpha 或 Beta 状态，你可以在 Alpha 和 Beta 特性门控表中找到该特性。
- 如果某个特性处于稳定状态， 你可以在已毕业和废弃特性门控表中找到该特性的所有阶段。
- [已毕业和废弃特性门控表](https://kubernetes.io/zh-cn/docs/reference/command-line-tools-reference/feature-gates/#feature-gates-for-graduated-or-deprecated-features) 还列出了废弃的和已被移除的特性。

## 使用特性

特性阶段 : Alpha, Beta, GA

### Alpha 特性代表：

- 默认禁用。
- 可能有错误，启用此特性可能会导致错误。
- 随时可能删除对此特性的支持，恕不另行通知。
- 在以后的软件版本中，API 可能会以不兼容的方式更改，恕不另行通知。
- 建议将其仅用于短期测试中，因为开启特性会增加错误的风险，并且缺乏长期支持。

## Beta 特性代表：

- 默认启用。
- 该特性已经经过良好测试。启用该特性是安全的。
- 尽管详细信息可能会更改，但不会放弃对整体特性的支持。
- 对象的架构或语义可能会在随后的 Beta 或稳定版本中以不兼容的方式更改。 当发生这种情况时，我们将提供迁移到下一版本的说明。此特性可能需要删除、编辑和重新创建 API 对象。 编辑过程可能需要慎重操作，因为这可能会导致依赖该特性的应用程序停机。
- 推荐仅用于非关键业务用途，因为在后续版本中可能会发生不兼容的更改。如果你具有多个可以独立升级的，则可以放宽此限制。


> 说明：
> 请试用 Beta 特性并提供相关反馈！ 一旦特性结束 Beta 状态，我们就不太可能再对特性进行大幅修改。


## General Availability (GA) 特性也称为 稳定 特性，GA 特性代表着：

- 此特性会一直启用；你不能禁用它。
- 不再需要相应的特性门控。
- 对于许多后续版本，特性的稳定版本将出现在发行的软件中。


## 如何开发

### 关键定义

位置： ./vendor/k8s.io/component-base/featuregate/feature_gate.go

```go

type Feature string

const (
	flagName = "feature-gates"

	// allAlphaGate is a global toggle for alpha features. Per-feature key
	// values override the default set by allAlphaGate. Examples:
	//   AllAlpha=false,NewFeature=true  will result in newFeature=true
	//   AllAlpha=true,NewFeature=false  will result in newFeature=false
	allAlphaGate Feature = "AllAlpha"

	// allBetaGate is a global toggle for beta features. Per-feature key
	// values override the default set by allBetaGate. Examples:
	//   AllBeta=false,NewFeature=true  will result in NewFeature=true
	//   AllBeta=true,NewFeature=false  will result in NewFeature=false
	allBetaGate Feature = "AllBeta"
)

var (
	// The generic features.
	defaultFeatures = map[Feature]FeatureSpec{
		allAlphaGate: {Default: false, PreRelease: Alpha},
		allBetaGate:  {Default: false, PreRelease: Beta},
	}

	// Special handling for a few gates.
	specialFeatures = map[Feature]func(known map[Feature]FeatureSpec, enabled map[Feature]bool, val bool){
		allAlphaGate: setUnsetAlphaGates,
		allBetaGate:  setUnsetBetaGates,
	}
)

type FeatureSpec struct {
	// Default is the default enablement state for the feature
	Default bool
	// LockToDefault indicates that the feature is locked to its default and cannot be changed
	LockToDefault bool
	// PreRelease indicates the maturity level of the feature
	PreRelease prerelease
}

type prerelease string

const (
	// Values for PreRelease.
	Alpha = prerelease("ALPHA")
	Beta  = prerelease("BETA")
	GA    = prerelease("")

	// Deprecated
	Deprecated = prerelease("DEPRECATED")
)

// FeatureGate indicates whether a given feature is enabled or not
type FeatureGate interface {
	// Enabled returns true if the key is enabled.
	Enabled(key Feature) bool
	// KnownFeatures returns a slice of strings describing the FeatureGate's known features.
	KnownFeatures() []string
	// DeepCopy returns a deep copy of the FeatureGate object, such that gates can be
	// set on the copy without mutating the original. This is useful for validating
	// config against potential feature gate changes before committing those changes.
	DeepCopy() MutableFeatureGate
}

// MutableFeatureGate parses and stores flag gates for known features from
// a string like feature1=true,feature2=false,...
type MutableFeatureGate interface {
	FeatureGate

	// AddFlag adds a flag for setting global feature gates to the specified FlagSet.
	AddFlag(fs *pflag.FlagSet)
	// Set parses and stores flag gates for known features
	// from a string like feature1=true,feature2=false,...
	Set(value string) error
	// SetFromMap stores flag gates for known features from a map[string]bool or returns an error
	SetFromMap(m map[string]bool) error
	// Add adds features to the featureGate.
	Add(features map[Feature]FeatureSpec) error
	// GetAll returns a copy of the map of known feature names to feature specs.
	GetAll() map[Feature]FeatureSpec
}



```

```go
// featureGate implements FeatureGate as well as pflag.Value for flag parsing.
type featureGate struct {
	featureGateName string

	special map[Feature]func(map[Feature]FeatureSpec, map[Feature]bool, bool)

	// lock guards writes to known, enabled, and reads/writes of closed
	lock sync.Mutex
	// known holds a map[Feature]FeatureSpec
	known *atomic.Value
	// enabled holds a map[Feature]bool
	enabled *atomic.Value
	// closed is set to true when AddFlag is called, and prevents subsequent calls to Add
	closed bool
}
```
1. FlagName

```shell

flagName = "feature-gates"

```

2. Feature

定义feature名称， 有两个默认 AllAlpha， AllBeta 特征

- AllAlpha=false,NewFeature=true  表示使用 AllAlpha 关闭 ，但是 newFeature 启用

- AllAlpha=true,NewFeature=false  表示使用 AllAlpha 启用 ，但是 newFeature 关闭

- AllBeta=false,NewFeature=true  表示使用 AllBeta 关闭 ，但是 newFeature 启用

- AllBeta=true,NewFeature=false  表示使用 AllBeta 启用 ，但是 newFeature 关闭

3. FeatureSpec

```go

type FeatureSpec struct {
	// 特征激活状态
	Default bool
	// 特征状态是否锁定，不能变更
	LockToDefault bool
	// 特征级别 Alpha， Beta, GA
	PreRelease prerelease
}

```

4. 接口和接口实现

接口 MutableFeatureGate 和 实现 featureGate


5. 初始化

位置： /vendor/k8s.io/apiserver/pkg/util/feature/feature_gate.go

```go

package feature

import (
	"k8s.io/component-base/featuregate"
)

var (
	// DefaultMutableFeatureGate is a mutable version of DefaultFeatureGate.
	// Only top-level commands/options setup and the k8s.io/component-base/featuregate/testing package should make use of this.
	// Tests that need to modify feature gates for the duration of their test should use:
	//   defer featuregatetesting.SetFeatureGateDuringTest(t, utilfeature.DefaultFeatureGate, features.<FeatureName>, <value>)()
	DefaultMutableFeatureGate featuregate.MutableFeatureGate = featuregate.NewFeatureGate()

	// DefaultFeatureGate is a shared global FeatureGate.
	// Top-level commands/options setup that needs to modify this feature gate should use DefaultMutableFeatureGate.
	DefaultFeatureGate featuregate.FeatureGate = DefaultMutableFeatureGate
)


```

### 开发

1. 自定义Feature和特征列表，同时把自定义特征列表加入到DefaultMutableFeatureGate


```go
package features

import (
	"k8s.io/apimachinery/pkg/util/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/component-base/featuregate"
)

// 定义特征 
const (
	// CraneAutoscaling enables the autoscaling features for workloads.
	CraneAutoscaling featuregate.Feature = "Autoscaling"

	// CraneAnalysis enables analysis features, including analytics and recommendation.
	CraneAnalysis featuregate.Feature = "Analysis"

	// CraneNodeResource enables the node resource features.
	CraneNodeResource featuregate.Feature = "NodeResource"

	// CraneNodeResourceTopology enables node resource topology features.
	CraneNodeResourceTopology featuregate.Feature = "NodeResourceTopology"

	// CranePodResource enables the pod resource features.
	CranePodResource featuregate.Feature = "PodResource"

	// CraneClusterNodePrediction enables the cluster node prediction features.
	CraneClusterNodePrediction featuregate.Feature = "ClusterNodePrediction"

	// CraneTimeSeriesPrediction enables the time series prediction features.
	CraneTimeSeriesPrediction featuregate.Feature = "TimeSeriesPrediction"

	// CraneCPUManager enables the cpu manger features.
	CraneCPUManager featuregate.Feature = "CraneCPUManager"

	// CraneDashboardControl enables the control from Dashboard.
	CraneDashboardControl featuregate.Feature = "DashboardControl"
)


// 定义特征列表
var defaultFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	CraneAutoscaling:           {Default: true, PreRelease: featuregate.Alpha},
	CraneAnalysis:              {Default: true, PreRelease: featuregate.Alpha},
	CraneNodeResource:          {Default: true, PreRelease: featuregate.Alpha},
	CraneNodeResourceTopology:  {Default: false, PreRelease: featuregate.Alpha},
	CranePodResource:           {Default: true, PreRelease: featuregate.Alpha},
	CraneClusterNodePrediction: {Default: false, PreRelease: featuregate.Alpha},
	CraneTimeSeriesPrediction:  {Default: true, PreRelease: featuregate.Alpha},
	CraneCPUManager:            {Default: false, PreRelease: featuregate.Alpha},
	CraneDashboardControl:      {Default: false, PreRelease: featuregate.Alpha},
}

func init() {
	// 加入特征列表
	runtime.Must(utilfeature.DefaultMutableFeatureGate.Add(defaultFeatureGates))
}
```


2. 判断特征启用情况

```go

	if err := webhooks.SetupWebhookWithManager(mgr,
		utilfeature.DefaultFeatureGate.Enabled(features.CraneAutoscaling),
		utilfeature.DefaultFeatureGate.Enabled(features.CraneNodeResource),
		utilfeature.DefaultFeatureGate.Enabled(features.CraneClusterNodePrediction),
		utilfeature.DefaultFeatureGate.Enabled(features.CraneAnalysis),
		utilfeature.DefaultFeatureGate.Enabled(features.CraneTimeSeriesPrediction)); err != nil {
		klog.Exit(err, "unable to create webhook", "webhook", "TimeSeriesPrediction")
	}

```




# helm

# 文档

1. [install](https://helm.sh/zh/docs/intro/install/)
2. [文档](https://helm.sh/zh/docs/intro/quickstart/)

## 自定义资源

- 随着Helm 3的到来，现在可以在chart中创建一个名为 crds 的特殊目录来保存CRD。 这些CRD没有模板化，但是运行helm install时可以为chart默认安装。

- 如果CRD已经存在，会显示警告并跳过。如果希望跳过CRD安装步骤， 可以使用--skip-crds参数。

- https://v3.helm.sh/zh/docs/chart_best_practices/custom_resource_definitions/
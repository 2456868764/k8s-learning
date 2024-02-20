#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

HIGRESS_KUBECONFIG="${HOME}/.kube/config_higress"
HIGRESS_CLUSTER_NAME="higress"

echo "Step1: Create local cluster: " ${HIGRESS_KUBECONFIG}
kind delete cluster --name="${HIGRESS_CLUSTER_NAME}" 2>&1
kind create cluster --kubeconfig "${HIGRESS_KUBECONFIG}" --name "${HIGRESS_CLUSTER_NAME}"  --config=`pwd`/ingress_kind_config.yaml  --image kindest/node:v1.21.1
export KUBECONFIG="${HIGRESS_KUBECONFIG}"
echo "Step1: Create local cluster finished."

echo "Get docker kind network "

sudo docker network inspect -f '{{.IPAM.Config[1].Subnet}}' kind

echo "Step2: Installing kind cluster LoadBalancer using Metallb."

kubectl apply -f `pwd`/metallb-native.yaml

kubectl get deploy -n metallb-system

echo "Please wait for Metallb pods ready"

kubectl wait --namespace metallb-system \
                --for=condition=ready pod \
                --selector=app=metallb \
                --timeout=300s
echo "Step3: Installing LoadBalancer using Metallb finished."

echo "Step4: Installing address pool used by loadbalancers."

kubectl apply -f `pwd`/addresspool.yaml

echo "Step4: Installing address pool finished."

echo "Step5: Installing Higress "
helm repo add higress.io https://higress.io/helm-charts
helm install higress -n higress-system higress.io/higress --create-namespace --render-subchart-notes --set global.kind=true --set higress-console.o11y.enabled=true  --set higress-controller.domain=console.higress.io --set higress-console.admin.password.value=admin

echo "Step5: Installing Higress finished."

kubectl get deploy -n higress-system

echo "Please wait for higress gateway ready"

kubectl wait --namespace higress-system \
                --for=condition=ready pod \
                --selector=app=higress-gateway \
                --timeout=300s


echo "After all pods ready, Get the Higress Dashboard URL to visit by running these commands in the same shell:"
echo "    export KUBECONFIG=${HOME}/.kube/config_higress"
echo "    GATEWAY_IP=\$(kubectl get svc/higress-gateway -n higress-system -o=jsonpath='{.status.loadBalancer.ingress[0].ip}')"
echo "    echo \"\${GATEWAY_IP} console.higress.io\" | sudo tee -a /etc/hosts"

echo "    higress console url: http://console.higress.io"
echo "    higress grafana url: http://console.higress.io/grafana"
echo "    higress grafana url: http://console.higress.io/prometheus"




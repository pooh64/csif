#!/bin/bash

# kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-provisioner/v2.1.0/deploy/kubernetes/rbac.yaml
# kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-attacher/v2.1.0/deploy/kubernetes/rbac.yaml

kubectl apply -f rbac-controller.yaml
kubectl apply -f driverinfo.yaml
kubectl apply -f filter.yaml
kubectl apply -f controller.yaml
kubectl apply -f node.yaml
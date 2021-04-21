#!/bin/bash

kubectl delete -f node.yaml
kubectl delete -f controller.yaml
kubectl delete -f driverinfo.yaml
kubectl delete -f rbac-controller.yaml

# kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-attacher/v2.1.0/deploy/kubernetes/rbac.yaml
# kubectl delete -f https://raw.githubusercontent.com/kubernetes-csi/external-provisioner/v2.1.0/deploy/kubernetes/rbac.yaml

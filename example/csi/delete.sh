#!/bin/sh

kubectl delete -f pod-csi.yaml
kubectl delete -f pvc-csi.yaml
kubectl delete -f pv-csi.yaml
kubectl delete -f pvc-gce.yaml

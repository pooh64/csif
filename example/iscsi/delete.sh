#!/bin/sh

kubectl delete -f pod-iscsi.yaml
kubectl delete -f pvc-iscsi.yaml
kubectl delete -f pv-iscsi.yaml

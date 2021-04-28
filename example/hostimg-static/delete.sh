#!/bin/sh

kubectl delete -f pod-static.yaml
kubectl delete -f pvc-static.yaml
kubectl delete -f pv-static.yaml

#!/bin/sh

kubectl delete -f pod-dynamic.yaml
kubectl delete -f pvc-dynamic.yaml
kubectl delete -f storageclass.yaml

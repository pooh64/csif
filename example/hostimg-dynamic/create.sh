#!/bin/sh

kubectl apply -f storageclass.yaml
kubectl apply -f pvc-dynamic.yaml
kubectl apply -f pod-dynamic.yaml

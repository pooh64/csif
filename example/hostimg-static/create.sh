#!/bin/sh

kubectl apply -f pv-static.yaml
kubectl apply -f pvc-static.yaml
kubectl apply -f pod-static.yaml

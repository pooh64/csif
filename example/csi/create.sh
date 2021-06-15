#!/bin/sh

kubectl apply -f pv-csi.yaml
kubectl apply -f pvc-csi.yaml
kubectl apply -f pod-csi.yaml
#!/bin/sh

kubectl apply -f pvc-gce.yaml
kubectl apply -f pv-csi.yaml
kubectl apply -f pvc-csi.yaml
kubectl apply -f pod-csi.yaml
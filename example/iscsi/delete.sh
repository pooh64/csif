#!/bin/sh

kubectl apply -f pod-iscsi.yaml
kubectl apply -f pvc-iscsi.yaml
kubectl apply -f pv-iscsi.yaml
#!/bin/bash
kubectl apply -f _pod_p.yaml
sleep 40
kubectl get pods
sleep 3
curl -H "Content-Type: application/json" -d @pod-template-simplify.json localhost:10253
kubectl apply -f _pod.yaml

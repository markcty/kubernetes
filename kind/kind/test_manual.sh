#!/bin/bash
docker cp fast.sh kind-control-plane:/
docker cp pod-template-simplify.json kind-control-plane:/
docker cp _pod.yaml kind-control-plane:/
docker cp _pod_p.yaml kind-control-plane:/
docker exec -t kind-control-plane bash fast.sh

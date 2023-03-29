#!/bin/bash
N=1

# create cluster
create-cluster() {
  if [ "$1" == "build" ]; then
    kind build node-image --image kindest/node:quic ~/go/src/k8s.io/kubernetes
  fi
  if [ "$?" != "0" ]; then
    echo "build failed"
    exit 127
  fi
  kind create cluster --image kindest/node:quic --config config.yaml

  sleep 10
}

extract-duration() {
  API_SERVER_LOG=$(find logs -type f -name "kube-apiserver*")
  SCHEDULER_LOG=$(find logs -type f -name "kube-scheduler*")
  KUBELET_LOG=$(find logs -type f -name "kubelet.log")

  CREATE_T=$(egrep "Prepare for pod $POD_NAME's creation" $API_SERVER_LOG | head -1 | cut -d " " -f1)
  SCHEDULE_T=$(egrep "Schedule pod $POD_NAME" $SCHEDULER_LOG | head -1 | cut -d " " -f1)
  SYNC_T=$(egrep "Sync pod $POD_NAME" $KUBELET_LOG | head -1 | cut -d " " -f7)
  START_T=$(egrep "Starting to bring up container $POD_NAME" $KUBELET_LOG | head -1 | cut -d " " -f7)
  UP_T=$(egrep "Finish bring up pod $POD_NAME" $KUBELET_LOG | head -1 | cut -d " " -f7)

  for i in CREATE_T SCHEDULE_T SYNC_T START_T UP_T; do
    echo "$i = ${!i}"
  done
}

cleanup() {
  kind delete cluster
}

# main
trap cleanup INT
create-cluster $1

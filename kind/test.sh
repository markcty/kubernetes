#!/bin/bash
N=10

set -e

extract_duration() {
  local i

  API_SERVER_LOG=$(find logs -type f -name "kube-apiserver*")
  SCHEDULER_LOG=$(find logs -type f -name "kube-scheduler*")
  KUBELET_LOG=$(find logs -type f -name "kubelet.log")

  API_SERVER_T=$(egrep "Prepare for pod $POD_NAME's creation" $API_SERVER_LOG | head -1 | cut -d " " -f1 | cut -d "T" -f2 | head -c -5)
  SCHEDULE_T=$(egrep "Schedule pod $POD_NAME" $SCHEDULER_LOG | head -1 | cut -d " " -f1 | cut -d "T" -f2 | head -c -5)
  KUBELET_T=$(egrep "SyncLoop get pod $POD_NAME" $KUBELET_LOG | head -1 | cut -d " " -f7)
  RUNTIME_T=$(egrep "Starting to bring up container $POD_NAME" $KUBELET_LOG | head -1 | cut -d " " -f7)
  UP_T=$(egrep "Finish bring up pod $POD_NAME" $KUBELET_LOG | head -1 | cut -d " " -f7)

  echo

  for i in KUBECTL_T API_SERVER_T SCHEDULE_T KUBELET_T RUNTIME_T UP_T; do
    echo "$i = ${!i}"
  done

  echo

  KUBECTL_TO_API_SERVER=$(($(date -d "$API_SERVER_T" "+%s%3N") - $(date -d "$KUBECTL_T" "+%s%3N")))
  API_SERVER_TO_SCHEDULE=$(($(date -d "$SCHEDULE_T" "+%s%3N") - $(date -d "$API_SERVER_T" "+%s%3N")))
  SCHEDULTE_TO_KUBELET=$(($(date -d "$KUBELET_T" "+%s%3N") - $(date -d "$SCHEDULE_T" "+%s%3N")))
  KUBELET_TO_RUNTIME=$(($(date -d "$RUNTIME_T" "+%s%3N") - $(date -d "$KUBELET_T" "+%s%3N")))
  RUNTIME_TO_UP=$(($(date -d "$UP_T" "+%s%3N") - $(date -d "$RUNTIME_T" "+%s%3N")))

  for i in KUBECTL_TO_API_SERVER API_SERVER_TO_SCHEDULE SCHEDULTE_TO_KUBELET KUBELET_TO_RUNTIME RUNTIME_TO_UP; do
    echo "$i = ${!i}ms"
    echo -n "${!i}," >>dur.csv
  done
  echo >>dur.csv
}

cleanup() {
  kubectl delete --all --now pods
}

# main
trap cleanup INT

truncate -s 0 dur.csv

START_I=$(($(cat count.txt) + 1))
END_I=$(($START_I + $N))
for ((i = $START_I; i < $END_I; i++)); do
  echo "--- round $i"
  export POD_NAME="nginx$i"

  echo $i >count.txt

  envsubst <pod.yaml >_pod.yaml
  KUBECTL_T=$(date -u +"%T.%6N")
  kubectl create -f _pod.yaml

  sleep 15

  rm -r logs
  kind export logs logs
  extract_duration

  sleep 2
  echo
done

# cleanup
cleanup

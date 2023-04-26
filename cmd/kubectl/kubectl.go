/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"k8s.io/component-base/cli"
	"k8s.io/kubectl/pkg/cmd"
	"k8s.io/kubectl/pkg/cmd/util"

	// Import to initialize client auth plugins.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

// func fast_apply(filePath string, stopCh chan int) {
// 	fileContent, err := ioutil.ReadFile(filePath)
// 	if err != nil {
// 		klog.Fatalf("Error reading the YAML file: %v", err)
// 	}

// 	var pod v1.Pod
// 	err = yaml.Unmarshal(fileContent, &pod)
// 	if err != nil {
// 		log.Fatalf("Error unmarshalling the YAML content: %v", err)
// 	}

// 	// var podConfig map[string]interface{}
// 	// err = yaml.Unmarshal(fileContent, &podConfig)
// 	// if err != nil {
// 	// 	log.Fatalf("Error unmarshalling the YAML content: %v", err)
// 	// }
// 	// fmt.Println("pod name", podConfig["metadata"].(map[string]string)["name"])

// 	// hack the pod
// 	pod.ObjectMeta.Name = pod.Spec.Containers[0].Name
// 	pod.ObjectMeta.Namespace = "default"
// 	pod.ObjectMeta.UID = "3ab5b2e5-0e75-4171-8128-51af97297917"

// 	json, err := json.Marshal(pod)
// 	if err != nil {
// 		klog.Fatalf("Error marshalling the config to JSON: %v", err)
// 	}

// 	fmt.Printf("JSON Data:\n%s\n", json)

// 	stopCh <- 1
// }

func main() {
	// stopCh := make(chan int)
	// if len(os.Args) == 4 && os.Args[1] == "apply" && os.Args[2] == "-f" && strings.Contains(os.Args[3], "fast") {
	// 	fast_apply(os.Args[3], stopCh)
	// 	return
	// }
	command := cmd.NewDefaultKubectlCommand()
	if err := cli.RunNoErrOutput(command); err != nil {
		// Pretty-print the error and exit with an error.
		util.CheckErr(err)
	}
	// <-stopCh
}

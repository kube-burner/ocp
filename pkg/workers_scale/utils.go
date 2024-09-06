// Copyright 2024 The Kube-burner Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package workers_scale


import (
	"fmt"
	"time"
	"context"

	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/kubernetes"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/wait"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	machinev1beta1 "github.com/openshift/client-go/machine/clientset/versioned/typed/machine/v1beta1"
)

// Helper function to create a pointer to an int32
func int32Ptr(i int32) *int32 {
    return &i
}

// getMachineClient creates a reusable machine client
func getMachineClient(restConfig *rest.Config) (*machinev1beta1.MachineV1beta1Client) {
	machineClient, err := machinev1beta1.NewForConfig(restConfig)
	if err != nil {
		log.Fatalf("error creating machine API client: %s", err)
	}

	return machineClient
}

// getNodeCount returns current node count
func getNodeCount(clientset kubernetes.Interface) (int, error) {
	nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
    if err != nil {
        return 0, fmt.Errorf("error listing nodes: %s", err)
    }

    return len(nodes.Items), nil
}

// isNodeReady checks if a node is ready
func isNodeReady(node *v1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == v1.NodeReady && condition.Status == v1.ConditionTrue {
			return true
		}
	}
	return false
}

// waitForNodes waits for all the nodes to be ready
func waitForNodes(clientset kubernetes.Interface, maxWaitTimeout time.Duration) error {
	return wait.PollUntilContextTimeout(context.TODO(), time.Second, maxWaitTimeout, true, func(ctx context.Context) (done bool, err error) {
		nodes, err := clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return false, err
		}

		for _, node := range nodes.Items {
			if !isNodeReady(&node) {
				log.Debugf("Node %s is not ready", node.Name)
				return false, nil
			}
		}
		log.Infof("All nodes are ready")
		return true, nil
	})
}

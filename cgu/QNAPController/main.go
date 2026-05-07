// main.go
package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

func main() {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	var config *rest.Config
	var err error

	// Check if kubeconfig file exists
	if _, err = os.Stat(*kubeconfig); err == nil {
		// kubeconfig file exists, use it to build config
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
		if err != nil {
			fmt.Printf("Error building kubeconfig: %s\n", err.Error())
			os.Exit(1)
		}
	} else {
		// kubeconfig file does not exist, try in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			fmt.Printf("Error building in-cluster config: %s\n", err.Error())
			os.Exit(1)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("Error creating Kubernetes client: %s\n", err.Error())
		os.Exit(1)
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		fmt.Printf("Error creating dynamic client: %s\n", err.Error())
		os.Exit(1)
	}
	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Starting QNAP Controller...")

	// Start the controller
	go runController(clientset, dynamicClient)

	<-stopCh
	fmt.Println("Shutting down QNAP Controller...")
}

func runController(clientset *kubernetes.Clientset, dynamicClient dynamic.Interface) {
	profileGVR := schema.GroupVersionResource{
		Group:    "kubeflow.org",
		Version:  "v1",
		Resource: "profiles",
	}

	for {
		// List all Profile CRDs
		profiles, err := dynamicClient.Resource(profileGVR).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			fmt.Printf("Error listing profiles: %s\n", err.Error())
			time.Sleep(10 * time.Second)
			continue
		}

		// Iterate over each profile
		for _, profile := range profiles.Items {
			namespace := profile.GetName()

			// Check if the Secret exists
			_, err := clientset.CoreV1().Secrets(namespace).Get(context.TODO(), "qnap", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				// Secret does not exist, create it
				createSecret(clientset, namespace)
			} else if err != nil {
				fmt.Printf("Error getting secret: %s\n", err.Error())
			}

			// Check if the LimitRange exists
			_, err = clientset.CoreV1().LimitRanges(namespace).Get(context.TODO(), "default-cpu-mem-limits", metav1.GetOptions{})
			if errors.IsNotFound(err) {
				// LimitRange does not exist, create it
				createLimitRange(clientset, namespace)
			} else if err != nil {
				fmt.Printf("Error getting limitrange: %s\n", err.Error())
			}
		}

		time.Sleep(30 * time.Second)
	}
}

func createSecret(clientset *kubernetes.Clientset, namespace string) {
	// Retrieve the ConfigMap
	configMap, err := clientset.CoreV1().ConfigMaps("kubeflow").Get(context.TODO(), "qnap-config", metav1.GetOptions{})
	if err != nil {
		fmt.Printf("Error getting configmap: %s\n", err.Error())
		return
	}

	// Decode base64 encoded values from ConfigMap
	ip, err := base64.StdEncoding.DecodeString(configMap.Data["ip"])
	if err != nil {
		fmt.Printf("Error decoding base64 ip: %s\n", err.Error())
		return
	}

	username, err := base64.StdEncoding.DecodeString(configMap.Data["username"])
	if err != nil {
		fmt.Printf("Error decoding base64 username: %s\n", err.Error())
		return
	}

	password, err := base64.StdEncoding.DecodeString(configMap.Data["password"])
	if err != nil {
		fmt.Printf("Error decoding base64 password: %s\n", err.Error())
		return
	}

	// Create the Secret with decoded values
	secretData := map[string][]byte{
		"ip":       ip,
		"username": username,
		"password": password,
	}

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "qnap",
		},
		Data: secretData,
	}

	_, err = clientset.CoreV1().Secrets(namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("Error creating secret: %s\n", err.Error())
	} else {
		fmt.Printf("Secret 'qnap' created in namespace '%s'\n", namespace)
	}
}

func createLimitRange(clientset *kubernetes.Clientset, namespace string) {
	// Create the LimitRange
	limitRange := &v1.LimitRange{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-cpu-mem-limits",
			Namespace: namespace,
		},
		Spec: v1.LimitRangeSpec{
			Limits: []v1.LimitRangeItem{
				{
					Type: v1.LimitTypeContainer,
					Default: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("500m"),
						v1.ResourceMemory: resource.MustParse("1Gi"),
					},
					DefaultRequest: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("250m"),
						v1.ResourceMemory: resource.MustParse("512Mi"),
					},
				},
			},
		},
	}

	_, err := clientset.CoreV1().LimitRanges(namespace).Create(context.TODO(), limitRange, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("Error creating limitrange: %s\n", err.Error())
	} else {
		fmt.Printf("LimitRange 'default-cpu-mem-limits' created in namespace '%s'\n", namespace)
	}
}

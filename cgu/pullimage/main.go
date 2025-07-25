package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"sigs.k8s.io/yaml"
)

type SpawnerUIConfig struct {
	SpawnerFormDefaults struct {
		Image struct {
			Options []string `json:"options" yaml:"options"`
		} `json:"image" yaml:"image"`
	} `json:"spawnerFormDefaults" yaml:"spawnerFormDefaults"`
}

var (
	deploymentName     = "jupyter-web-app-deployment"
	deploymentNS       = "kubeflow"
	configmapNameStart = "jupyter-web-app-config"
	pullImageNS        = "pullimage"
)

func main() {
	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("取得叢集設定失敗: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("建立 clientset 失敗: %v", err)
	}

	factory := informers.NewSharedInformerFactory(clientset, 5*time.Minute)
	configMapInformer := factory.Core().V1().ConfigMaps().Informer()

	configMapInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			cm, ok := obj.(*corev1.ConfigMap)
			if !ok {
				return
			}
			if strings.HasPrefix(cm.Name, configmapNameStart) && cm.Namespace == deploymentNS {
				log.Printf("偵測到 ConfigMap %s/%s 新增，產生 Job", cm.Namespace, cm.Name)
				reconcileJobs(clientset)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			cm, ok := newObj.(*corev1.ConfigMap)
			if !ok {
				return
			}
			if strings.HasPrefix(cm.Name, configmapNameStart) && cm.Namespace == deploymentNS {
				log.Printf("偵測到 ConfigMap %s/%s 更新，產生 Job", cm.Namespace, cm.Name)
				reconcileJobs(clientset)
			}
		},
		DeleteFunc: func(obj interface{}) {
			cm, ok := obj.(*corev1.ConfigMap)
			if !ok {
				return
			}
			if strings.HasPrefix(cm.Name, configmapNameStart) && cm.Namespace == deploymentNS {
				log.Printf("偵測到 ConfigMap %s/%s 刪除，產生 Job", cm.Namespace, cm.Name)
				reconcileJobs(clientset)
			}
		},
	})

	stopCh := make(chan struct{})
	defer close(stopCh)
	factory.Start(stopCh)

	if !cache.WaitForCacheSync(stopCh, configMapInformer.HasSynced) {
		log.Fatalf("cache 同步失敗")
	}

	select {} // block forever
}

func reconcileJobs(clientset *kubernetes.Clientset) {
	ctx := context.Background()

	// 取得指定的 Deployment
	dep, err := clientset.AppsV1().Deployments(deploymentNS).Get(ctx, deploymentName, metav1.GetOptions{})
	if err != nil {
		log.Printf("取得 Deployment %s/%s 失敗: %v", deploymentNS, deploymentName, err)
		return
	}

	var foundVolume bool
	var volumeName string
	for _, vol := range dep.Spec.Template.Spec.Volumes {
		if vol.ConfigMap != nil && strings.HasPrefix(vol.ConfigMap.Name, configmapNameStart) {
			foundVolume = true
			volumeName = vol.ConfigMap.Name
			break
		}
	}
	if !foundVolume {
		log.Printf("Deployment %s/%s 並未參照 configmap", deploymentNS, deploymentName)
		return
	}

	// 取得 ConfigMap
	cm, err := clientset.CoreV1().ConfigMaps(deploymentNS).Get(ctx, volumeName, metav1.GetOptions{})
	if err != nil {
		log.Printf("取得 ConfigMap %s/%s 失敗: %v", deploymentNS, volumeName, err)
		return
	}
	configContent, ok := cm.Data["spawner_ui_config.yaml"]
	if !ok {
		log.Printf("ConfigMap %s/%s 中找不到 spawner_ui_config.yaml", deploymentNS, volumeName)
		return
	}

	var spawnerCfg SpawnerUIConfig
	if err := yaml.Unmarshal([]byte(configContent), &spawnerCfg); err != nil {
		log.Printf("解析 spawner_ui_config.yaml 失敗: %v", err)
		return
	}
	images := spawnerCfg.SpawnerFormDefaults.Image.Options
	log.Printf("將根據 ConfigMap 產生 Job，images: %v", images)

	// 取得所有 node
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		log.Printf("取得 node 列表失敗: %v", err)
		return
	}

	// 刪除現有的 Job
	jobList, err := clientset.BatchV1().Jobs(pullImageNS).List(ctx, metav1.ListOptions{})
	var backgroundDeletion = metav1.DeletePropagationBackground
	if err == nil {
		for _, job := range jobList.Items {
			_ = clientset.BatchV1().Jobs(pullImageNS).Delete(ctx, job.Name, metav1.DeleteOptions{PropagationPolicy: &backgroundDeletion})
		}
	}

	// 為每個 image, 每個 node 建立一個 Job
	var activeDeadlineSeconds int64 = 900   // 900 seconds, 15 minutes
	var backoffLimit int32 = 0              // no retry
	var ttlSecondsAfterFinished int32 = 600 // 600 seconds, 10 minutes
	for _, image := range images {
		for _, node := range nodes.Items {
			jobName := generateJobName(image, node.Name)
			job := &batchv1.Job{
				ObjectMeta: metav1.ObjectMeta{
					Name:      jobName,
					Namespace: pullImageNS,
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							NodeName:      node.Name,
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{
								{
									Name:    "puller",
									Image:   image,
									Command: []string{"echo", "image pulled!"},
								},
							},
						},
					},
					ActiveDeadlineSeconds:   &activeDeadlineSeconds,   // e.g. 15 min – long enough to finish
					BackoffLimit:            &backoffLimit,            // no retry
					TTLSecondsAfterFinished: &ttlSecondsAfterFinished, // 10 minutes after job finished
				},
			}
			_, err := clientset.BatchV1().Jobs(pullImageNS).Create(ctx, job, metav1.CreateOptions{})
			if err != nil {
				log.Printf("建立 Job %s (image: %s, node: %s) 失敗: %v", jobName, image, node.Name, err)
			} else {
				log.Printf("建立 Job %s (image: %s, node: %s) 成功", jobName, image, node.Name)
			}
		}
	}
	log.Println("所有 Job 產生完成")
}

func generateJobName(image, nodeName string) string {
	// 產生合法名稱
	name := strings.ToLower(fmt.Sprintf("%s-%s", image, nodeName))
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') ||
			(r >= '0' && r <= '9') ||
			r == '-' {
			return r
		}
		return '-'
	}, name)
	if len(name) > 50 {
		name = name[:50]
	}
	return "job-" + name
}

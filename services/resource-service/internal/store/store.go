package store

import (
	"context"
	"fmt"
	"sync"

	resourcev1 "github.com/gopaas/platform/api/gen/resource/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func int32Ptr(v int32) *int32 { return &v }

// Store abstracts Deployment management for memory and Kubernetes backends.
type Store interface {
	Create(ctx context.Context, req *resourcev1.CreateDeploymentRequest) (*resourcev1.DeploymentInfo, error)
	Get(ctx context.Context, namespace, name string) (*resourcev1.DeploymentInfo, error)
	List(ctx context.Context, namespace string) ([]*resourcev1.DeploymentInfo, error)
	Scale(ctx context.Context, namespace, name string, replicas int32) (*resourcev1.DeploymentInfo, error)
	Delete(ctx context.Context, namespace, name string) error
}

type memoryStore struct {
	mu   sync.RWMutex
	data map[string]*resourcev1.DeploymentInfo
}

func key(ns, name string) string { return ns + "/" + name }

// NewMemoryStore is used for local demos without a cluster.
func NewMemoryStore() Store {
	return &memoryStore{data: map[string]*resourcev1.DeploymentInfo{}}
}

func (m *memoryStore) Create(_ context.Context, req *resourcev1.CreateDeploymentRequest) (*resourcev1.DeploymentInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(req.Namespace, req.Name)
	if _, ok := m.data[k]; ok {
		return nil, fmt.Errorf("deployment %s already exists", k)
	}
	info := &resourcev1.DeploymentInfo{
		Namespace:     req.Namespace,
		Name:          req.Name,
		Image:         req.Image,
		Replicas:      req.Replicas,
		ReadyReplicas: req.Replicas,
		Status:        "Available",
		Labels:        req.Labels,
	}
	m.data[k] = info
	return clone(info), nil
}

func (m *memoryStore) Get(_ context.Context, namespace, name string) (*resourcev1.DeploymentInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	info, ok := m.data[key(namespace, name)]
	if !ok {
		return nil, fmt.Errorf("deployment %s/%s not found", namespace, name)
	}
	return clone(info), nil
}

func (m *memoryStore) List(_ context.Context, namespace string) ([]*resourcev1.DeploymentInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*resourcev1.DeploymentInfo, 0)
	for _, v := range m.data {
		if namespace == "" || v.Namespace == namespace {
			out = append(out, clone(v))
		}
	}
	return out, nil
}

func (m *memoryStore) Scale(_ context.Context, namespace, name string, replicas int32) (*resourcev1.DeploymentInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	info, ok := m.data[key(namespace, name)]
	if !ok {
		return nil, fmt.Errorf("deployment %s/%s not found", namespace, name)
	}
	info.Replicas = replicas
	info.ReadyReplicas = replicas
	info.Status = "Available"
	return clone(info), nil
}

func (m *memoryStore) Delete(_ context.Context, namespace, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(namespace, name)
	if _, ok := m.data[k]; !ok {
		return fmt.Errorf("deployment %s/%s not found", namespace, name)
	}
	delete(m.data, k)
	return nil
}

func clone(in *resourcev1.DeploymentInfo) *resourcev1.DeploymentInfo {
	if in == nil {
		return nil
	}
	cp := *in
	if in.Labels != nil {
		cp.Labels = map[string]string{}
		for k, v := range in.Labels {
			cp.Labels[k] = v
		}
	}
	return &cp
}

type k8sStore struct {
	client *kubernetes.Clientset
}

// NewK8sStore manages real Kubernetes Deployments via client-go.
func NewK8sStore(client *kubernetes.Clientset) Store {
	return &k8sStore{client: client}
}

func (k *k8sStore) Create(ctx context.Context, req *resourcev1.CreateDeploymentRequest) (*resourcev1.DeploymentInfo, error) {
	labels := req.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	if _, ok := labels["app"]; !ok {
		labels["app"] = req.Name
	}
	replicas := req.Replicas
	if replicas <= 0 {
		replicas = 1
	}
	port := req.ContainerPort
	if port <= 0 {
		port = 80
	}
	cpu := req.CpuRequest
	if cpu == "" {
		cpu = "50m"
	}
	mem := req.MemoryRequest
	if mem == "" {
		mem = "64Mi"
	}

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(replicas),
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": labels["app"]}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": labels["app"]}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  req.Name,
						Image: req.Image,
						Ports: []corev1.ContainerPort{{ContainerPort: port}},
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse(cpu),
								corev1.ResourceMemory: resource.MustParse(mem),
							},
						},
					}},
				},
			},
		},
	}

	created, err := k.client.AppsV1().Deployments(req.Namespace).Create(ctx, dep, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	return fromDeployment(created), nil
}

func (k *k8sStore) Get(ctx context.Context, namespace, name string) (*resourcev1.DeploymentInfo, error) {
	dep, err := k.client.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return fromDeployment(dep), nil
}

func (k *k8sStore) List(ctx context.Context, namespace string) ([]*resourcev1.DeploymentInfo, error) {
	list, err := k.client.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]*resourcev1.DeploymentInfo, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, fromDeployment(&list.Items[i]))
	}
	return out, nil
}

func (k *k8sStore) Scale(ctx context.Context, namespace, name string, replicas int32) (*resourcev1.DeploymentInfo, error) {
	scale, err := k.client.AppsV1().Deployments(namespace).GetScale(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	scale.Spec.Replicas = replicas
	if _, err := k.client.AppsV1().Deployments(namespace).UpdateScale(ctx, name, scale, metav1.UpdateOptions{}); err != nil {
		return nil, err
	}
	return k.Get(ctx, namespace, name)
}

func (k *k8sStore) Delete(ctx context.Context, namespace, name string) error {
	err := k.client.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if errors.IsNotFound(err) {
		return fmt.Errorf("deployment %s/%s not found", namespace, name)
	}
	return err
}

func fromDeployment(dep *appsv1.Deployment) *resourcev1.DeploymentInfo {
	var replicas int32
	if dep.Spec.Replicas != nil {
		replicas = *dep.Spec.Replicas
	}
	image := ""
	if len(dep.Spec.Template.Spec.Containers) > 0 {
		image = dep.Spec.Template.Spec.Containers[0].Image
	}
	status := "Progressing"
	if dep.Status.ReadyReplicas >= replicas && replicas > 0 {
		status = "Available"
	}
	return &resourcev1.DeploymentInfo{
		Namespace:     dep.Namespace,
		Name:          dep.Name,
		Image:         image,
		Replicas:      replicas,
		ReadyReplicas: dep.Status.ReadyReplicas,
		Status:        status,
		Labels:        dep.Labels,
	}
}

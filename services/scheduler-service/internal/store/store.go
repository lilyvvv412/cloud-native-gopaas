package store

import (
	"context"
	"fmt"
	"sync"

	schedulerv1 "github.com/gopaas/platform/api/gen/scheduler/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// Store manages scheduled task records / CRDs.
type Store interface {
	Submit(ctx context.Context, req *schedulerv1.SubmitTaskRequest) (*schedulerv1.TaskInfo, error)
	Get(ctx context.Context, namespace, name string) (*schedulerv1.TaskInfo, error)
	List(ctx context.Context, namespace string) ([]*schedulerv1.TaskInfo, error)
	Cancel(ctx context.Context, namespace, name string) error
}

type memoryStore struct {
	mu   sync.RWMutex
	data map[string]*schedulerv1.TaskInfo
}

func key(ns, name string) string { return ns + "/" + name }

func NewMemoryStore() Store {
	return &memoryStore{data: map[string]*schedulerv1.TaskInfo{}}
}

func (m *memoryStore) Submit(_ context.Context, req *schedulerv1.SubmitTaskRequest) (*schedulerv1.TaskInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(req.Namespace, req.Name)
	if _, ok := m.data[k]; ok {
		return nil, fmt.Errorf("task %s already exists", k)
	}
	info := &schedulerv1.TaskInfo{
		Name:      req.Name,
		Namespace: req.Namespace,
		Image:     req.Image,
		Schedule:  req.Schedule,
		Command:   req.Command,
		Status:    "Accepted",
		Message:   "stored in memory; apply ScheduledTask CRD in k8s mode",
		Labels:    req.Labels,
	}
	m.data[k] = info
	return clone(info), nil
}

func (m *memoryStore) Get(_ context.Context, namespace, name string) (*schedulerv1.TaskInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	info, ok := m.data[key(namespace, name)]
	if !ok {
		return nil, fmt.Errorf("task %s/%s not found", namespace, name)
	}
	return clone(info), nil
}

func (m *memoryStore) List(_ context.Context, namespace string) ([]*schedulerv1.TaskInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*schedulerv1.TaskInfo, 0)
	for _, v := range m.data {
		if namespace == "" || v.Namespace == namespace {
			out = append(out, clone(v))
		}
	}
	return out, nil
}

func (m *memoryStore) Cancel(_ context.Context, namespace, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := key(namespace, name)
	if _, ok := m.data[k]; !ok {
		return fmt.Errorf("task %s/%s not found", namespace, name)
	}
	delete(m.data, k)
	return nil
}

func clone(in *schedulerv1.TaskInfo) *schedulerv1.TaskInfo {
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

var scheduledTaskGVR = schema.GroupVersionResource{
	Group:    "gopaas.io",
	Version:  "v1",
	Resource: "scheduledtasks",
}

type k8sStore struct {
	client  *kubernetes.Clientset
	dynamic dynamic.Interface
}

// NewK8sStore creates ScheduledTask CRDs (preferred) or falls back to Jobs.
func NewK8sStore(client *kubernetes.Clientset, dyn dynamic.Interface) Store {
	return &k8sStore{client: client, dynamic: dyn}
}

func (k *k8sStore) Submit(ctx context.Context, req *schedulerv1.SubmitTaskRequest) (*schedulerv1.TaskInfo, error) {
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "gopaas.io/v1",
			"kind":       "ScheduledTask",
			"metadata": map[string]interface{}{
				"name":      req.Name,
				"namespace": req.Namespace,
				"labels":    toIfaceMap(req.Labels),
			},
			"spec": map[string]interface{}{
				"image":    req.Image,
				"schedule": req.Schedule,
				"command":  req.Command,
			},
		},
	}
	created, err := k.dynamic.Resource(scheduledTaskGVR).Namespace(req.Namespace).Create(ctx, obj, metav1.CreateOptions{})
	if err == nil {
		return fromUnstructured(created), nil
	}
	// Fallback for clusters without the CRD installed yet: create a one-shot Job.
	if !apierrors.IsNotFound(err) && !metaNoMatch(err) {
		// still try job fallback for CRD missing
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
			Labels:    req.Labels,
			Annotations: map[string]string{
				"gopaas.io/schedule": req.Schedule,
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Containers: []corev1.Container{{
						Name:    req.Name,
						Image:   req.Image,
						Command: splitCommand(req.Command),
					}},
				},
			},
		},
	}
	createdJob, jerr := k.client.BatchV1().Jobs(req.Namespace).Create(ctx, job, metav1.CreateOptions{})
	if jerr != nil {
		return nil, fmt.Errorf("create ScheduledTask: %v; fallback Job: %w", err, jerr)
	}
	return &schedulerv1.TaskInfo{
		Name:      createdJob.Name,
		Namespace: createdJob.Namespace,
		Image:     req.Image,
		Schedule:  req.Schedule,
		Command:   req.Command,
		Status:    "JobFallback",
		Message:   "ScheduledTask CRD unavailable; created batch/v1 Job",
		Labels:    req.Labels,
	}, nil
}

func (k *k8sStore) Get(ctx context.Context, namespace, name string) (*schedulerv1.TaskInfo, error) {
	obj, err := k.dynamic.Resource(scheduledTaskGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return fromUnstructured(obj), nil
	}
	job, jerr := k.client.BatchV1().Jobs(namespace).Get(ctx, name, metav1.GetOptions{})
	if jerr != nil {
		return nil, err
	}
	return &schedulerv1.TaskInfo{
		Name:      job.Name,
		Namespace: job.Namespace,
		Schedule:  job.Annotations["gopaas.io/schedule"],
		Status:    "Job",
		Message:   "loaded from Job fallback",
		Labels:    job.Labels,
	}, nil
}

func (k *k8sStore) List(ctx context.Context, namespace string) ([]*schedulerv1.TaskInfo, error) {
	list, err := k.dynamic.Resource(scheduledTaskGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err == nil {
		out := make([]*schedulerv1.TaskInfo, 0, len(list.Items))
		for i := range list.Items {
			out = append(out, fromUnstructured(&list.Items[i]))
		}
		return out, nil
	}
	jobs, jerr := k.client.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{})
	if jerr != nil {
		return nil, err
	}
	out := make([]*schedulerv1.TaskInfo, 0, len(jobs.Items))
	for i := range jobs.Items {
		j := jobs.Items[i]
		out = append(out, &schedulerv1.TaskInfo{
			Name:      j.Name,
			Namespace: j.Namespace,
			Schedule:  j.Annotations["gopaas.io/schedule"],
			Status:    "Job",
			Labels:    j.Labels,
		})
	}
	return out, nil
}

func (k *k8sStore) Cancel(ctx context.Context, namespace, name string) error {
	err := k.dynamic.Resource(scheduledTaskGVR).Namespace(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err == nil {
		return nil
	}
	return k.client.BatchV1().Jobs(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

func fromUnstructured(obj *unstructured.Unstructured) *schedulerv1.TaskInfo {
	spec, _, _ := unstructured.NestedMap(obj.Object, "spec")
	status, _, _ := unstructured.NestedMap(obj.Object, "status")
	info := &schedulerv1.TaskInfo{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		Labels:    obj.GetLabels(),
	}
	if v, ok := spec["image"].(string); ok {
		info.Image = v
	}
	if v, ok := spec["schedule"].(string); ok {
		info.Schedule = v
	}
	if v, ok := spec["command"].(string); ok {
		info.Command = v
	}
	if v, ok := status["phase"].(string); ok {
		info.Status = v
	} else {
		info.Status = "Submitted"
	}
	if v, ok := status["message"].(string); ok {
		info.Message = v
	}
	return info
}

func toIfaceMap(in map[string]string) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func splitCommand(cmd string) []string {
	if cmd == "" {
		return []string{"sh", "-c", "echo gopaas-task && sleep 1"}
	}
	return []string{"sh", "-c", cmd}
}

func metaNoMatch(err error) bool {
	return apierrors.IsNotFound(err) || apierrors.IsMethodNotSupported(err)
}

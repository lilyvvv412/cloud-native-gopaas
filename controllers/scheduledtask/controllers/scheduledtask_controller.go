package controllers

import (
	"context"
	"fmt"
	"time"

	gopaasv1 "github.com/gopaas/platform/controllers/scheduledtask/api/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ScheduledTaskReconciler reconciles a ScheduledTask object into Jobs.
type ScheduledTaskReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *ScheduledTaskReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var task gopaasv1.ScheduledTask
	if err := r.Get(ctx, req.NamespacedName, &task); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	jobName := task.Name + "-job"
	var existing batchv1.Job
	err := r.Get(ctx, client.ObjectKey{Namespace: task.Namespace, Name: jobName}, &existing)
	if err == nil {
		task.Status.Phase = "Running"
		task.Status.ActiveJob = jobName
		task.Status.Message = "Job already exists"
		_ = r.Status().Update(ctx, &task)
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	}
	if !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	cmd := task.Spec.Command
	if cmd == "" {
		cmd = "echo scheduled-task && date"
	}
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: task.Namespace,
			Labels: map[string]string{
				"gopaas.io/scheduledtask": task.Name,
			},
			Annotations: map[string]string{
				"gopaas.io/schedule": task.Spec.Schedule,
			},
		},
		Spec: batchv1.JobSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers: []corev1.Container{{
						Name:    "task",
						Image:   task.Spec.Image,
						Command: []string{"sh", "-c", cmd},
					}},
				},
			},
		},
	}
	if err := ctrl.SetControllerReference(&task, job, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.Create(ctx, job); err != nil {
		task.Status.Phase = "Error"
		task.Status.Message = err.Error()
		_ = r.Status().Update(ctx, &task)
		return ctrl.Result{}, err
	}

	now := metav1.Now()
	task.Status.Phase = "Scheduled"
	task.Status.ActiveJob = jobName
	task.Status.LastScheduleTime = &now
	task.Status.Message = fmt.Sprintf("created Job %s for schedule %s", jobName, task.Spec.Schedule)
	if err := r.Status().Update(ctx, &task); err != nil {
		logger.Error(err, "update status")
	}
	logger.Info("created Job for ScheduledTask", "job", jobName)
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

func (r *ScheduledTaskReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&gopaasv1.ScheduledTask{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

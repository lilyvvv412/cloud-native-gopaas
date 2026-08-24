package handler

import (
	"context"

	schedulerv1 "github.com/gopaas/platform/api/gen/scheduler/v1"
	"github.com/gopaas/platform/services/scheduler-service/internal/store"
)

// SchedulerHandler implements SchedulerService via go-micro.
type SchedulerHandler struct {
	Store store.Store
}

func (h *SchedulerHandler) SubmitTask(ctx context.Context, req *schedulerv1.SubmitTaskRequest, rsp *schedulerv1.TaskResponse) error {
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.Image == "" {
		req.Image = "busybox:1.36"
	}
	if req.Schedule == "" {
		req.Schedule = "*/1 * * * *"
	}
	info, err := h.Store.Submit(ctx, req)
	if err != nil {
		return err
	}
	rsp.Task = info
	rsp.Message = "submitted"
	return nil
}

func (h *SchedulerHandler) GetTask(ctx context.Context, req *schedulerv1.GetTaskRequest, rsp *schedulerv1.TaskResponse) error {
	info, err := h.Store.Get(ctx, req.Namespace, req.Name)
	if err != nil {
		return err
	}
	rsp.Task = info
	rsp.Message = "ok"
	return nil
}

func (h *SchedulerHandler) ListTasks(ctx context.Context, req *schedulerv1.ListTasksRequest, rsp *schedulerv1.ListTasksResponse) error {
	items, err := h.Store.List(ctx, req.Namespace)
	if err != nil {
		return err
	}
	rsp.Tasks = items
	return nil
}

func (h *SchedulerHandler) CancelTask(ctx context.Context, req *schedulerv1.CancelTaskRequest, rsp *schedulerv1.CancelTaskResponse) error {
	if err := h.Store.Cancel(ctx, req.Namespace, req.Name); err != nil {
		return err
	}
	rsp.Message = "cancelled"
	return nil
}

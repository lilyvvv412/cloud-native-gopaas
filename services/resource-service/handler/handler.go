package handler

import (
	"context"

	resourcev1 "github.com/gopaas/platform/api/gen/resource/v1"
	"github.com/gopaas/platform/services/resource-service/internal/store"
)

// ResourceHandler implements ResourceService via go-micro.
type ResourceHandler struct {
	Store store.Store
}

func (h *ResourceHandler) CreateDeployment(ctx context.Context, req *resourcev1.CreateDeploymentRequest, rsp *resourcev1.DeploymentResponse) error {
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.Replicas <= 0 {
		req.Replicas = 1
	}
	info, err := h.Store.Create(ctx, req)
	if err != nil {
		return err
	}
	rsp.Deployment = info
	rsp.Message = "created"
	return nil
}

func (h *ResourceHandler) GetDeployment(ctx context.Context, req *resourcev1.GetDeploymentRequest, rsp *resourcev1.DeploymentResponse) error {
	info, err := h.Store.Get(ctx, req.Namespace, req.Name)
	if err != nil {
		return err
	}
	rsp.Deployment = info
	rsp.Message = "ok"
	return nil
}

func (h *ResourceHandler) ListDeployments(ctx context.Context, req *resourcev1.ListDeploymentsRequest, rsp *resourcev1.ListDeploymentsResponse) error {
	items, err := h.Store.List(ctx, req.Namespace)
	if err != nil {
		return err
	}
	rsp.Deployments = items
	return nil
}

func (h *ResourceHandler) ScaleDeployment(ctx context.Context, req *resourcev1.ScaleDeploymentRequest, rsp *resourcev1.DeploymentResponse) error {
	info, err := h.Store.Scale(ctx, req.Namespace, req.Name, req.Replicas)
	if err != nil {
		return err
	}
	rsp.Deployment = info
	rsp.Message = "scaled"
	return nil
}

func (h *ResourceHandler) DeleteDeployment(ctx context.Context, req *resourcev1.DeleteDeploymentRequest, rsp *resourcev1.DeleteDeploymentResponse) error {
	if err := h.Store.Delete(ctx, req.Namespace, req.Name); err != nil {
		return err
	}
	rsp.Message = "deleted"
	return nil
}

package store

import (
	"context"
	"testing"

	resourcev1 "github.com/gopaas/platform/api/gen/resource/v1"
)

func TestMemoryStoreCRUDAndScale(t *testing.T) {
	s := NewMemoryStore()
	ctx := context.Background()

	created, err := s.Create(ctx, &resourcev1.CreateDeploymentRequest{
		Namespace: "default",
		Name:      "web",
		Image:     "nginx:1.25",
		Replicas:  2,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Replicas != 2 {
		t.Fatalf("replicas=%d", created.Replicas)
	}

	got, err := s.Get(ctx, "default", "web")
	if err != nil || got.Image != "nginx:1.25" {
		t.Fatalf("get: %#v err=%v", got, err)
	}

	scaled, err := s.Scale(ctx, "default", "web", 5)
	if err != nil || scaled.Replicas != 5 {
		t.Fatalf("scale: %#v err=%v", scaled, err)
	}

	list, err := s.List(ctx, "default")
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %#v err=%v", list, err)
	}

	if err := s.Delete(ctx, "default", "web"); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

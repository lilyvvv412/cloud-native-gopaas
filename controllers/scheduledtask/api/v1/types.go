package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	Group   = "gopaas.io"
	Version = "v1"
	Kind    = "ScheduledTask"
)

var (
	GroupVersion  = schema.GroupVersion{Group: Group, Version: Version}
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

// ScheduledTask is the Schema for the scheduledtasks API.
type ScheduledTask struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ScheduledTaskSpec   `json:"spec,omitempty"`
	Status            ScheduledTaskStatus `json:"status,omitempty"`
}

type ScheduledTaskSpec struct {
	Image    string `json:"image"`
	Schedule string `json:"schedule"`
	Command  string `json:"command,omitempty"`
}

type ScheduledTaskStatus struct {
	Phase          string       `json:"phase,omitempty"`
	Message        string       `json:"message,omitempty"`
	LastScheduleTime *metav1.Time `json:"lastScheduleTime,omitempty"`
	ActiveJob      string       `json:"activeJob,omitempty"`
}

// ScheduledTaskList contains a list of ScheduledTask.
type ScheduledTaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ScheduledTask `json:"items"`
}

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion,
		&ScheduledTask{},
		&ScheduledTaskList{},
	)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}

func (in *ScheduledTask) DeepCopyObject() runtime.Object {
	out := *in
	out.ObjectMeta = *in.ObjectMeta.DeepCopy()
	if in.Status.LastScheduleTime != nil {
		t := *in.Status.LastScheduleTime
		out.Status.LastScheduleTime = &t
	}
	return &out
}

func (in *ScheduledTaskList) DeepCopyObject() runtime.Object {
	out := *in
	out.Items = make([]ScheduledTask, len(in.Items))
	for i := range in.Items {
		out.Items[i] = *in.Items[i].DeepCopyObject().(*ScheduledTask)
	}
	return &out
}

/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"k8s-demo/my-replicaset/api/v1alpha1"
	crdv1alpha1 "k8s-demo/my-replicaset/api/v1alpha1"
)

// MyReplicasetReconciler reconciles a MyReplicaset object
type MyReplicasetReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=crd.k8s.demo,resources=myreplicasets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=crd.k8s.demo,resources=myreplicasets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=crd.k8s.demo,resources=myreplicasets/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the MyReplicaset object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile
func (r *MyReplicasetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// 获取对象
	var replicaSet crdv1alpha1.MyReplicaset
	if err := r.Get(ctx, req.NamespacedName, &replicaSet); err != nil {
		logger.Error(err, "Unable to fetch MyReplicaSet")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if replicaSet.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	// 获取相关的pod
	var podList corev1.PodList
	if err := r.List(ctx, &podList, &client.ListOptions{
		Namespace: replicaSet.Namespace,
	}); err != nil {
		logger.Error(err, "unable to fetch PodList")
		return ctrl.Result{}, err
	}

	filterPods, err := r.GetPodsForReplicaset(podList.Items, &replicaSet)
	if err != nil {
		logger.Error(err, "failed to get pods list")
		return ctrl.Result{}, err
	}

	logger.Info("get pods", "count", len(filterPods))

	diff := len(filterPods) - *replicaSet.Spec.Replicas
	if diff < 0 {
		// 创建pod
		diff = diff * -1
		logger.Info("create pods", "count", diff)
		controllerRef := metav1.NewControllerRef(&replicaSet, replicaSet.GroupVersionKind())
		pod, err := GetPodFromTemplate(&replicaSet.Spec.Template, &replicaSet, controllerRef)
		if err != nil {
			logger.Error(err, "failed to get pod from template")
			return ctrl.Result{}, err
		}

		generateName := replicaSet.Name + "-"
		pod.ObjectMeta.GenerateName = generateName

		if pod.Labels == nil {
			pod.Labels = make(map[string]string)
		}

		for key, value := range r.GenLabels(&replicaSet) {
			pod.Labels[key] = value
		}

		for i := 0; i < diff; i++ {
			podCopy := pod.DeepCopy()
			if err := r.Create(ctx, podCopy, &client.CreateOptions{}); err != nil {
				logger.Error(err, "failed to create pod")
				return ctrl.Result{}, err
			}

			r.Recorder.Event(&replicaSet, corev1.EventTypeNormal, "SuccessfulCreatePod", fmt.Sprintf("create pod: %s", podCopy.Name))
		}
	} else if diff > 0 {
		// 删除pod
		logger.Info("delete pods", "count", diff)
		deletePods := getDeletePod(filterPods, diff)
		for _, pod := range deletePods {
			if err := r.Delete(ctx, &pod, &client.DeleteOptions{}); err != nil {
				logger.Error(err, "failed to delete pod")
				return ctrl.Result{}, err
			}

			r.Recorder.Event(&replicaSet, corev1.EventTypeNormal, "SuccessfulDeletePod", fmt.Sprintf("delete pod: %s", pod.Name))
		}
	}

	// 更新状态
	replicas := len(filterPods)
	newRs := replicaSet.DeepCopy()
	newRs.Status.Replicas = &replicas
	if replicaSet.Status.Replicas != nil && *(newRs.Status.Replicas) == *(replicaSet.Status.Replicas) {
		return ctrl.Result{}, nil
	}

	if err := r.Status().Update(ctx, newRs, &client.UpdateOptions{}); err != nil {
		logger.Error(err, "failed to update status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MyReplicasetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&crdv1alpha1.MyReplicaset{}).
		Owns(&corev1.Pod{}, builder.WithPredicates(&PredicateForPod{})).
		Complete(r)
}

func (r *MyReplicasetReconciler) GetPodsForReplicaset(pods []corev1.Pod, rs *crdv1alpha1.MyReplicaset) ([]corev1.Pod, error) {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: r.GenLabels(rs),
	})
	if err != nil {
		return nil, err
	}

	var result []corev1.Pod
	for _, p := range pods {
		// pod处于终止阶段
		if corev1.PodSucceeded == p.Status.Phase ||
			corev1.PodFailed == p.Status.Phase ||
			p.DeletionTimestamp != nil {
			continue
		}

		if !selector.Matches(labels.Set(p.Labels)) {
			continue
		}

		result = append(result, p)
	}

	return result, nil
}

func (r *MyReplicasetReconciler) GenLabels(rs *crdv1alpha1.MyReplicaset) map[string]string {
	return map[string]string{
		"my-relicaset-name": rs.GetName(),
	}
}

func GetPodFromTemplate(template *corev1.PodTemplateSpec, rs *v1alpha1.MyReplicaset, controllerRef *metav1.OwnerReference) (*corev1.Pod, error) {
	desiredLabels := getPodsLabelSet(template, rs)
	desiredFinalizers := getPodsFinalizers(template)
	desiredAnnotations := getPodsAnnotationSet(template)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      desiredLabels,
			Annotations: desiredAnnotations,
			Namespace:   rs.Namespace,
			Finalizers:  desiredFinalizers,
		},
	}
	if controllerRef != nil {
		pod.OwnerReferences = append(pod.OwnerReferences, *controllerRef)
	}
	pod.Spec = *template.Spec.DeepCopy()
	return pod, nil
}

func getPodsLabelSet(template *corev1.PodTemplateSpec, rs *v1alpha1.MyReplicaset) labels.Set {
	desiredLabels := make(labels.Set)
	for k, v := range template.Labels {
		desiredLabels[k] = v
	}

	return desiredLabels
}

func getPodsFinalizers(template *corev1.PodTemplateSpec) []string {
	desiredFinalizers := make([]string, len(template.Finalizers))
	copy(desiredFinalizers, template.Finalizers)
	return desiredFinalizers
}

func getPodsAnnotationSet(template *corev1.PodTemplateSpec) labels.Set {
	desiredAnnotations := make(labels.Set)
	for k, v := range template.Annotations {
		desiredAnnotations[k] = v
	}
	return desiredAnnotations
}

func getDeletePod(pods []corev1.Pod, diff int) []corev1.Pod {
	if diff >= len(pods) {
		return pods
	}

	sort.Sort(PodList(pods))
	return pods[:diff]
}

type PodList []corev1.Pod

var (
	podPhaseToOrdinal = map[corev1.PodPhase]int{corev1.PodPending: 0, corev1.PodUnknown: 1, corev1.PodRunning: 2}
)

func (l PodList) Len() int {
	return len(l)
}

func (l PodList) Swap(i, j int) {
	l[i], l[j] = l[j], l[i]
}

func (l PodList) Less(i, j int) bool {
	// 1. Unassigned < assigned
	if l[i].Spec.NodeName != l[j].Spec.NodeName && (len(l[i].Spec.NodeName) == 0 || len(l[j].Spec.NodeName) == 0) {
		return len(l[i].Spec.NodeName) == 0
	}

	// 2. PodPending < PodUnknown < PodRunning
	if podPhaseToOrdinal[l[i].Status.Phase] != podPhaseToOrdinal[l[j].Status.Phase] {
		return podPhaseToOrdinal[l[i].Status.Phase] < podPhaseToOrdinal[l[j].Status.Phase]
	}

	// 3. newer pods < older pods
	return l[i].CreationTimestamp.After(l[j].CreationTimestamp.Time)
}

// ////////////////////////////////////////////////////////////////////////////////////////
type PredicateForPod struct {
}

func (p *PredicateForPod) Create(evt event.CreateEvent) bool {
	process := false
	log.Log.Info("======================= CreateEvent =======================",
		"event name", evt.Object.GetName(),
		"event namespace", evt.Object.GetNamespace(),
	)
	return process
}

func (p *PredicateForPod) Update(evt event.UpdateEvent) bool {
	process := false
	log.Log.Info("======================= UpdateEvent =======================",
		"event old name", evt.ObjectOld.GetName(), "event new name", evt.ObjectNew.GetName(),
		"event namespace", evt.ObjectNew.GetNamespace(),
	)
	return process
}

func (p *PredicateForPod) Delete(evt event.DeleteEvent) bool {
	process := true
	log.Log.Info("======================= DeleteEvent =======================",
		"event name", evt.Object.GetName(),
		"event namespace", evt.Object.GetNamespace(),
	)
	return process
}

func (p *PredicateForPod) Generic(evt event.GenericEvent) bool {
	process := false
	log.Log.Info("======================= GenericEvent =======================",
		"event name", evt.Object.GetName(),
		"event namespace", evt.Object.GetNamespace(),
	)
	return process
}

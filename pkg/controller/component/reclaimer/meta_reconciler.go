/*
Copyright 2021 Vesoft Inc.

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

package reclaimer

import (
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/vesoft-inc/nebula-operator/apis/apps/v1alpha1"
	"github.com/vesoft-inc/nebula-operator/pkg/controller/component"
	"github.com/vesoft-inc/nebula-operator/pkg/kube"
	"github.com/vesoft-inc/nebula-operator/pkg/label"
)

type meta struct {
	clientSet kube.ClientSet
}

func NewMetaReconciler(clientSet kube.ClientSet) component.ReconcileManager {
	return &meta{clientSet}
}

func (m *meta) Reconcile(nc *v1alpha1.NebulaCluster) error {
	namespace := nc.GetNamespace()
	clusterName := nc.GetClusterName()
	l, err := label.New().Cluster(clusterName).Selector()
	if err != nil {
		return err
	}
	pods, err := m.clientSet.Pod().ListPods(namespace, l)
	if err != nil {
		return fmt.Errorf("list pods for cluster %s/%s failed: %v", namespace, clusterName, err)
	}

	for i := range pods {
		pod := pods[i]
		if !label.Label(pod.Labels).IsNebulaComponent() {
			continue
		}
		pvcs, err := m.resolvePVCFromPod(&pod)
		if err != nil {
			return err
		}
		for i := range pvcs {
			pvc := pvcs[i]
			if err := m.clientSet.PVC().UpdateMetaInfo(pvc, &pod); err != nil {
				return err
			}
			if pvc.Spec.VolumeName == "" {
				continue
			}
			pv, err := m.clientSet.PV().GetPersistentVolume(pvc.Spec.VolumeName)
			if err != nil {
				klog.Errorf("get pv %s failed: %v", pvc.Spec.VolumeName, err)
				return err
			}
			if err := m.clientSet.PV().UpdateMetaInfo(nc, pv); err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *meta) resolvePVCFromPod(pod *corev1.Pod) ([]*corev1.PersistentVolumeClaim, error) {
	var pvcs []*corev1.PersistentVolumeClaim
	var pvcName string
	for i := range pod.Spec.Volumes {
		vol := pod.Spec.Volumes[i]
		if vol.PersistentVolumeClaim == nil {
			continue
		}
		pvcName = vol.PersistentVolumeClaim.ClaimName
		if pvcName == "" {
			continue
		}
		pvc, err := m.clientSet.PVC().GetPVC(pod.Namespace, pvcName)
		if err != nil {
			klog.Errorf("get pvc %s/%s failed: %v", pod.Namespace, pvcName, err)
			continue
		}
		pvcs = append(pvcs, pvc)
	}
	if len(pvcs) == 0 {
		return nil, errors.New("pvcs not found")
	}
	return pvcs, nil
}

type FakeMetaReconciler struct {
	err error
}

func NewFakeMetaReconciler() *FakeMetaReconciler {
	return &FakeMetaReconciler{}
}

func (f *FakeMetaReconciler) SetReconcileError(err error) {
	f.err = err
}

func (f *FakeMetaReconciler) Reconcile(_ *v1alpha1.NebulaCluster) error {
	return f.err
}

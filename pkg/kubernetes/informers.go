/*
Copyright 2019 The Kubernetes Authors.

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

package kubernetes

import (
	"context"
	"sync"
	"time"

	"k8s.io/client-go/informers"
	v1 "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/sample-controller/pkg/signals"
	"sigs.k8s.io/vsphere-csi-driver/v3/pkg/csi/service/logger"
)

const (
	// resyncPeriodConfigMapInformer is the time interval between each resync
	// operation for the configmap informer.
	//
	// Whenever there is a update on the configmap, we do get a callback to the
	// handler immediately. However, if for some reason update was missed, we
	// need to set a resync interval for resync check operations that happens
	// as part of NewFilteredConfigMapInformer(). Since we do not anticipate
	// frequent changes to the configmaps, the resync interval is set to 30 min.
	resyncPeriodConfigMapInformer = 30 * time.Minute
)

var (
	inClusterInformerManagerInstance  *InformerManager = nil
	inClusterInformerInstanceLock                      = &sync.Mutex{}
	supervisorInformerManagerInstance *InformerManager = nil
	supervisorInformerInstanceLock                     = &sync.Mutex{}
)

func noResyncPeriodFunc() time.Duration {
	return 0
}

// NewInformer creates a new K8S client based on a service account.
// NOTE: This function expects caller function to pass appropriate client
// as per config to be created Informer for.
// This function creates shared informer factory against the client provided.
func NewInformer(ctx context.Context, client clientset.Interface, inClusterClnt bool) *InformerManager {
	var informerInstance *InformerManager
	log := logger.GetLogger(ctx)

	if inClusterClnt {
		inClusterInformerInstanceLock.Lock()
		defer inClusterInformerInstanceLock.Unlock()

		informerInstance = inClusterInformerManagerInstance
	} else {
		supervisorInformerInstanceLock.Lock()
		defer supervisorInformerInstanceLock.Unlock()

		informerInstance = supervisorInformerManagerInstance
	}

	if informerInstance == nil {
		informerInstance = &InformerManager{
			client:          client,
			stopCh:          signals.SetupSignalHandler(),
			informerFactory: informers.NewSharedInformerFactory(client, noResyncPeriodFunc()),
		}

		if inClusterClnt {
			inClusterInformerManagerInstance = informerInstance
			log.Info("Created new informer factory for in-cluster client")
		} else {
			supervisorInformerManagerInstance = informerInstance
			log.Info("Created new informer factory for supervisor client")
		}
	}

	return informerInstance
}

// AddNodeListener hooks up add, update, delete callbacks.
func (im *InformerManager) AddNodeListener(
	add func(obj interface{}), update func(oldObj, newObj interface{}), remove func(obj interface{})) {
	if im.nodeInformer == nil {
		im.nodeInformer = im.informerFactory.Core().V1().Nodes().Informer()
	}

	_, err := im.nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    add,
		UpdateFunc: update,
		DeleteFunc: remove,
	})
	if err != nil {
		return
	}
}

// AddCSINodeNodeListener hooks up add, update, delete callbacks.
func (im *InformerManager) AddCSINodeListener(
	add func(obj interface{}), update func(oldObj, newObj interface{}), remove func(obj interface{})) {
	if im.nodeInformer == nil {
		im.nodeInformer = im.informerFactory.Storage().V1().CSINodes().Informer()
	}

	_, err := im.nodeInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    add,
		UpdateFunc: update,
		DeleteFunc: remove,
	})
	if err != nil {
		return
	}
}

// AddPVCListener hooks up add, update, delete callbacks.
func (im *InformerManager) AddPVCListener(
	add func(obj interface{}), update func(oldObj, newObj interface{}), remove func(obj interface{})) {
	if im.pvcInformer == nil {
		im.pvcInformer = im.informerFactory.Core().V1().PersistentVolumeClaims().Informer()
	}
	im.pvcSynced = im.pvcInformer.HasSynced

	_, err := im.pvcInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    add,
		UpdateFunc: update,
		DeleteFunc: remove,
	})
	if err != nil {
		return
	}
}

// AddPVListener hooks up add, update, delete callbacks.
func (im *InformerManager) AddPVListener(
	add func(obj interface{}), update func(oldObj, newObj interface{}), remove func(obj interface{})) {
	if im.pvInformer == nil {
		im.pvInformer = im.informerFactory.Core().V1().PersistentVolumes().Informer()
	}
	im.pvSynced = im.pvInformer.HasSynced

	_, err := im.pvInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    add,
		UpdateFunc: update,
		DeleteFunc: remove,
	})
	if err != nil {
		return
	}
}

// AddNamespaceListener hooks up add, update, delete callbacks.
func (im *InformerManager) AddNamespaceListener(
	add func(obj interface{}), update func(oldObj, newObj interface{}), remove func(obj interface{})) {
	if im.namespaceInformer == nil {
		im.namespaceInformer = im.informerFactory.Core().V1().Namespaces().Informer()
	}
	im.namespaceSynced = im.namespaceInformer.HasSynced

	_, err := im.namespaceInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    add,
		UpdateFunc: update,
		DeleteFunc: remove,
	})
	if err != nil {
		return
	}
}

// AddConfigMapListener hooks up add, update, delete callbacks.
func (im *InformerManager) AddConfigMapListener(
	ctx context.Context, client clientset.Interface, namespace string,
	add func(obj interface{}), update func(oldObj, newObj interface{}), remove func(obj interface{})) {
	if im.configMapInformer == nil {
		im.configMapInformer = v1.NewFilteredConfigMapInformer(client, namespace,
			resyncPeriodConfigMapInformer, cache.Indexers{}, nil)
	}
	im.configMapSynced = im.configMapInformer.HasSynced

	_, err := im.configMapInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    add,
		UpdateFunc: update,
		DeleteFunc: remove,
	})
	if err != nil {
		return
	}
	stopCh := make(chan struct{})
	// Since NewFilteredConfigMapInformer is not part of the informer factory,
	// we need to invoke the Run() explicitly to start the shared informer.
	go im.configMapInformer.Run(stopCh)
}

// AddPodListener hooks up add, update, delete callbacks.
func (im *InformerManager) AddPodListener(
	add func(obj interface{}), update func(oldObj, newObj interface{}), remove func(obj interface{})) {
	if im.podInformer == nil {
		im.podInformer = im.informerFactory.Core().V1().Pods().Informer()
	}
	im.podSynced = im.podInformer.HasSynced

	_, err := im.podInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    add,
		UpdateFunc: update,
		DeleteFunc: remove,
	})
	if err != nil {
		return
	}
}

// AddVolumeAttachmentListener hooks up add, update, delete callbacks.
func (im *InformerManager) AddVolumeAttachmentListener(
	add func(obj interface{}), update func(oldObj, newObj interface{}), remove func(obj interface{})) {
	if im.volumeAttachmentInformer == nil {
		im.volumeAttachmentInformer = im.informerFactory.Storage().V1().VolumeAttachments().Informer()
	}

	_, err := im.volumeAttachmentInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    add,
		UpdateFunc: update,
		DeleteFunc: remove,
	})
	if err != nil {
		return
	}
}

// GetPVLister returns PV Lister for the calling informer manager.
func (im *InformerManager) GetPVLister() corelisters.PersistentVolumeLister {
	return im.informerFactory.Core().V1().PersistentVolumes().Lister()
}

// GetPVCLister returns PVC Lister for the calling informer manager.
func (im *InformerManager) GetPVCLister() corelisters.PersistentVolumeClaimLister {
	return im.informerFactory.Core().V1().PersistentVolumeClaims().Lister()
}

// GetConfigMapLister returns ConfigMap Lister for the calling informer manager.
func (im *InformerManager) GetConfigMapLister() corelisters.ConfigMapLister {
	return im.informerFactory.Core().V1().ConfigMaps().Lister()
}

// GetPodLister returns Pod Lister for the calling informer manager.
func (im *InformerManager) GetPodLister() corelisters.PodLister {
	return im.informerFactory.Core().V1().Pods().Lister()
}

// Listen starts the Informers.
func (im *InformerManager) Listen() (stopCh <-chan struct{}) {
	go im.informerFactory.Start(im.stopCh)
	if im.pvSynced != nil && im.pvcSynced != nil && im.podSynced != nil && im.configMapSynced != nil {
		if !cache.WaitForCacheSync(im.stopCh, im.pvSynced, im.pvcSynced, im.podSynced, im.configMapSynced) {
			return
		}

	}
	return im.stopCh
}

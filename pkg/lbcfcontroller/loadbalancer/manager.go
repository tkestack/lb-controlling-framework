package loadbalancer

import (
	"time"

	"git.tencent.com/tke/lb-controlling-framework/pkg/client-go/apis/lbcf.tke.cloud.tencent.com/v1beta1"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/kubernetes/pkg/controller"
)

type Manager struct {
	store map[string]*v1beta1.LoadBalancer

	queue workqueue.RateLimitingInterface

}

func NewManager() *Manager{
	return &Manager{}
}

func (m *Manager) Start(){
	for i := 0; i < 3; i++{
		go wait.Until(m.worker, time.Second, wait.NeverStop)
	}
}

func (m *Manager) EnqueueLoadBalancer(obj interface{}){
	key, err := controller.KeyFunc(obj)
	if err != nil{
		return
	}
	m.queue.Add(key)
}

func (m *Manager) worker(){
	for m.processNextItem(){
	}
}

func (m *Manager) processNextItem() bool{
	key, quit := m.queue.Get()
	if quit{
		return false
	}
	defer m.queue.Done(key)
	if err := m.sync(key.(string)); err != nil{
		m.queue.AddRateLimited(key)
	} else{
		m.queue.Forget(key)
	}
	return true
}

func (m *Manager) sync(key string) error{
	// TODO: sync LoadBalancer

	return nil
}



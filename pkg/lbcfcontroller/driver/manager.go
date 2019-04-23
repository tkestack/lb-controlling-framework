package driver

//import (
//	"k8s.io/apimachinery/pkg/util/wait"
//	"k8s.io/client-go/util/workqueue"
//	"k8s.io/kubernetes/pkg/controller"
//	"time"
//)
//
//type Manager struct {
//	store map[string]*LoadBalancerDriverInternal
//
//	queue workqueue.RateLimitingInterface
//}
//
//func NewManager() *Manager {
//	return &Manager{
//		store: make(map[string]*LoadBalancerDriverInternal),
//		queue: workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "drivermanager"),
//	}
//}
//
//func (m *Manager) Start(){
//	for i := 0; i < 3; i++{
//		go wait.Until(m.worker, time.Second, wait.NeverStop)
//	}
//}
//
//func (m *Manager) GetDriver(name string, driverType string) (*LoadBalancerDriverInternal, bool){
//	d, ok := m.store[name]
//	if !ok{
//		return nil, false
//	}
//	if d.DriverType == driverType{
//		return d, true
//	}
//	return nil, false
//}
//
//func (m *Manager) EnqueueDriver(obj interface{}){
//	key, err := controller.KeyFunc(obj)
//	if err != nil{
//		return
//	}
//	m.queue.Add(key)
//}
//
//func (m *Manager) worker(){
//	for m.processNextItem(){
//	}
//}
//
//func (m *Manager) processNextItem() bool{
//	key, quit := m.queue.Get()
//	if quit{
//		return false
//	}
//	defer m.queue.Done(key)
//	if err := m.sync(key.(string)); err != nil{
//		m.queue.AddRateLimited(key)
//	} else{
//		m.queue.Forget(key)
//	}
//	return true
//}
//
//func (m *Manager) sync(key string) error{
//	// TODO: sync Driver
//
//	return nil
//}


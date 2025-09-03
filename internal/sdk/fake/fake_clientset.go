package fake

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	kfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	rfake "k8s.io/client-go/rest/fake"

	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

type FakeExtendedCoreV1 struct {
	typedcorev1.CoreV1Interface
}

func (c *FakeExtendedCoreV1) RESTClient() rest.Interface {
	return &rfake.RESTClient{}
}

type FakeExtendedClientset struct {
	*kfake.Clientset
}

func (f *FakeExtendedClientset) CoreV1() typedcorev1.CoreV1Interface {
	return &FakeExtendedCoreV1{f.Clientset.CoreV1()}
}

func NewFakeClientset(objs ...runtime.Object) kubernetes.Interface {
	return &FakeExtendedClientset{kfake.NewClientset(objs...)}
}

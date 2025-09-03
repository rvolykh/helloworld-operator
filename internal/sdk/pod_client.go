package sdk

import (
	"context"
	"fmt"
	"strings"

	helloworldv1 "github.com/rvolykh/helloworld-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type podClient struct {
	clientSet kubernetes.Interface
	podExec   PodExec
}

type PodClientOptions struct {
	ClientSet kubernetes.Interface
	PodExec   PodExec
}

func NewPodClient(options PodClientOptions) (*podClient, error) {
	var (
		restConfig *rest.Config
		err        error
	)

	if options.ClientSet == nil || options.PodExec == nil {
		kubeCfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			clientcmd.NewDefaultClientConfigLoadingRules(),
			&clientcmd.ConfigOverrides{},
		)
		restConfig, err = kubeCfg.ClientConfig()
		if err != nil {
			return nil, err
		}
	}

	if options.ClientSet == nil {
		options.ClientSet, err = kubernetes.NewForConfig(restConfig)
		if err != nil {
			return nil, err
		}
	}
	if options.PodExec == nil {
		options.PodExec = &SPDYExecutor{restConfig: restConfig}
	}

	return &podClient{
		clientSet: options.ClientSet,
		podExec:   options.PodExec,
	}, nil
}

func (c *podClient) CopyTo(ctx context.Context, obj helloworldv1.CopyToPod) error {
	request := c.clientSet.CoreV1().RESTClient().
		Post().
		Namespace(obj.Namespace).
		Resource("pods").
		Name(obj.Spec.PodName).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: obj.Spec.ContainerName,
			Command:   []string{"/bin/sh", "-c", fmt.Sprintf("cat > %s", obj.Spec.FileName)},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	err := c.podExec.Execute(ctx, request.URL(), strings.NewReader(obj.Spec.Content))
	if err != nil {
		return fmt.Errorf("failed to copy %v/%v %s: %w",
			obj.Namespace, obj.Spec.PodName, obj.Spec.FileName, err)
	}

	return nil
}

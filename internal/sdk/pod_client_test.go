package sdk_test

import (
	"context"
	"fmt"
	"testing"

	helloworldv1 "github.com/rvolykh/helloworld-operator/api/v1"
	helloworldsdk "github.com/rvolykh/helloworld-operator/internal/sdk"
	helloworldsdkfake "github.com/rvolykh/helloworld-operator/internal/sdk/fake"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPodClient_CopyTo(t *testing.T) {
	ctrl := gomock.NewController(t)

	fakePodExec := helloworldsdkfake.NewMockPodExec(ctrl)

	options := helloworldsdk.PodClientOptions{
		ClientSet: helloworldsdkfake.NewFakeClientset(),
		PodExec:   fakePodExec,
	}
	sut, err := helloworldsdk.NewPodClient(options)
	require.NoError(t, err)

	t.Run("positive", func(t *testing.T) {
		fakePodExec.EXPECT().
			Execute(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil)

		input := helloworldv1.CopyToPod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test_copy_to_prod",
				Namespace: "default",
			},
			Spec: helloworldv1.CopyToPodSpec{
				PodName:       "test_pod_name",
				ContainerName: "test_container_name",
				FileName:      "/tmp/TestPodClient_CopyTo.txt",
				Content:       "Hello World",
			},
		}

		result := sut.CopyTo(context.TODO(), input)
		require.NoError(t, result)
	})

	t.Run("negative", func(t *testing.T) {
		fakePodExec.EXPECT().
			Execute(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(fmt.Errorf("a"))

		input := helloworldv1.CopyToPod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test_copy_to_prod",
				Namespace: "default",
			},
			Spec: helloworldv1.CopyToPodSpec{
				PodName:       "test_pod_name",
				ContainerName: "test_container_name",
				FileName:      "/tmp/TestPodClient_CopyTo.txt",
				Content:       "Hello World",
			},
		}

		result := sut.CopyTo(context.TODO(), input)
		require.Error(t, result)
	})
}

package fake

import (
	"context"

	helloworldv1 "github.com/rvolykh/helloworld-operator/api/v1"
)

type FakePodCopyCmd struct {
	Err error
}

func (f *FakePodCopyCmd) CopyTo(ctx context.Context, obj helloworldv1.CopyToPod) error {
	return f.Err
}

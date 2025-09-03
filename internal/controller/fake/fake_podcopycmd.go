package fake

import (
	context "context"

	v1 "github.com/rvolykh/helloworld-operator/api/v1"
)

type FakePodCopyCmd struct {
	Err error
}

func (f *FakePodCopyCmd) CopyTo(ctx context.Context, obj v1.CopyToPod) error {
	return f.Err
}

package controller

import (
	"context"

	helloworldv1 "github.com/rvolykh/helloworld-operator/api/v1"
)

type PodCopyCmd interface {
	CopyTo(ctx context.Context, obj helloworldv1.CopyToPod) error
}

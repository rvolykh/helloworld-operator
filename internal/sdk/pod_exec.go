package sdk

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

type PodExec interface {
	Execute(ctx context.Context, cmdURL *url.URL, stdin io.Reader) error
}

type SPDYExecutor struct {
	restConfig *rest.Config
}

func (e *SPDYExecutor) Execute(ctx context.Context, cmdURL *url.URL, stdin io.Reader) error {
	exec, err := remotecommand.NewSPDYExecutor(e.restConfig, "POST", cmdURL)
	if err != nil {
		return fmt.Errorf("failed to prepare executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	options := remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if err = exec.StreamWithContext(ctx, options); err != nil {
		return fmt.Errorf("command failed: %s: %w", stderr.String(), err)
	}

	return nil
}

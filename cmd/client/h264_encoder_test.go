//go:build desktop
// +build desktop

package main

import (
	"fmt"
	"testing"
)

func TestIsRetryableH264EncodeError(t *testing.T) {
	retryable := fmt.Errorf("%w: warmup", errH264EncoderNeedsMoreInput)
	if !isRetryableH264EncodeError(retryable) {
		t.Fatalf("expected retryable error to be detected")
	}

	if isRetryableH264EncodeError(fmt.Errorf("fatal encode failure")) {
		t.Fatalf("did not expect fatal error to be treated as retryable")
	}
}

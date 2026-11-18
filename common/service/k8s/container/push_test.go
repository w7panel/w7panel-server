package container

import (
	"errors"
	"testing"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

func TestConsumeRemotePushProgressDrainsAfterCallbackError(t *testing.T) {
	updates := make(chan v1.Update)
	done := make(chan struct{})
	go func() {
		consumeRemotePushProgress(updates, func(Progress) error { return errors.New("client disconnected") })
		close(done)
	}()

	for i := 0; i < 256; i++ {
		updates <- v1.Update{Complete: int64(i), Total: 256}
	}
	close(updates)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("progress consumer stopped draining updates")
	}
}

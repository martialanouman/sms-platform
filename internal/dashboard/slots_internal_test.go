package dashboard

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestEveryHashingSlotTakenRefusesAtOnce(t *testing.T) {
	h := NewHasher()
	phc, err := h.Hash(context.Background(), "Correct-horse-9")
	if err != nil {
		t.Fatal(err)
	}
	release, held := make(chan struct{}), sync.WaitGroup{}
	held.Add(slots)
	var done sync.WaitGroup
	for range slots {
		done.Go(func() {
			_ = h.hold(func() error {
				held.Done()
				<-release
				return nil
			})
		})
	}
	held.Wait()
	ctx := context.Background()
	if _, err := h.Hash(ctx, "Correct-horse-9"); !errors.Is(err, ErrBusy) {
		t.Errorf("hash = %v, want ErrBusy", err)
	}
	if _, err := h.Verify(ctx, phc, "Correct-horse-9"); !errors.Is(err, ErrBusy) {
		t.Errorf("verify = %v, want ErrBusy", err)
	}
	if err := h.VerifyDummy(ctx, "x"); !errors.Is(err, ErrBusy) {
		t.Errorf("dummy = %v, want ErrBusy", err)
	}
	close(release)
	done.Wait()
	if err := h.VerifyDummy(ctx, "x"); err != nil {
		t.Errorf("dummy after release = %v", err)
	}
}

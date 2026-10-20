package dashboard

import (
	"context"
	"errors"
	"testing"
)

func TestEveryHashingSlotTakenRefusesAtOnce(t *testing.T) {
	h := NewHasher()
	ctx := context.Background()
	phc, err := h.Hash(ctx, "Correct-horse-9")
	if err != nil {
		t.Fatal(err)
	}
	release := HoldEverySlot(h)
	if _, err := h.Hash(ctx, "Correct-horse-9"); !errors.Is(err, ErrBusy) {
		t.Errorf("hash = %v, want ErrBusy", err)
	}
	if _, err := h.Verify(ctx, phc, "Correct-horse-9"); !errors.Is(err, ErrBusy) {
		t.Errorf("verify = %v, want ErrBusy", err)
	}
	if err := h.VerifyDummy(ctx, "x"); !errors.Is(err, ErrBusy) {
		t.Errorf("dummy = %v, want ErrBusy", err)
	}
	release()
	if err := h.VerifyDummy(ctx, "x"); err != nil {
		t.Errorf("dummy after release = %v", err)
	}
}

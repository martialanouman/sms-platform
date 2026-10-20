package dashboard

import "sync"

// HoldEverySlot takes every slot of h until release is called.
func HoldEverySlot(h *Hasher) (release func()) {
	done, held := make(chan struct{}), sync.WaitGroup{}
	held.Add(slots)
	var returned sync.WaitGroup
	for range slots {
		returned.Go(func() {
			_ = h.hold(func() error {
				held.Done()
				<-done
				return nil
			})
		})
	}
	held.Wait()
	return func() {
		close(done)
		returned.Wait()
	}
}

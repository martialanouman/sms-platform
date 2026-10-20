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

var (
	TotpCode   = totpCode
	TotpMatch  = totpMatch
	OtpauthURI = otpauthURI
)

func SealTotp(k TotpKey, operatorID string, secret []byte) []byte { return k.seal(operatorID, secret) }

func OpenTotp(k TotpKey, operatorID string, sealed []byte) ([]byte, error) {
	return k.open(operatorID, sealed)
}

package dashboard

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
)

const totpIssuer = "Tableau de bord SMS"

var totpEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

func totpCode(secret []byte, step int64) string {
	m := hmac.New(sha1.New, secret)
	_ = binary.Write(m, binary.BigEndian, step)
	sum := m.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1_000_000)
}

func totpMatch(secret []byte, code string, now, last int64) (int64, bool) {
	for step := now - 1; step <= now+1; step++ {
		if step > last && hmac.Equal([]byte(totpCode(secret, step)), []byte(code)) {
			return step, true
		}
	}
	return 0, false
}

func otpauthURI(secret []byte, email string) string {
	u := url.URL{
		Scheme:   "otpauth",
		Host:     "totp",
		Path:     "/" + totpIssuer + ":" + email,
		RawQuery: url.Values{"secret": {totpEncoding.EncodeToString(secret)}, "issuer": {totpIssuer}}.Encode(),
	}
	return u.String()
}

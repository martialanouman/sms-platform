package dashboard_test

import (
	"encoding/base32"
	"net/url"
	"testing"

	"github.com/martialanouman/sms-platform/internal/dashboard"
)

var rfc6238 = []byte("12345678901234567890")

func TestTotpCodesMatchTheVectorsOfRFC6238(t *testing.T) {
	for at, want := range map[int64]string{
		59:          "287082",
		1111111109:  "081804",
		1111111111:  "050471",
		1234567890:  "005924",
		2000000000:  "279037",
		20000000000: "353130",
	} {
		if got := dashboard.TotpCode(rfc6238, at/30); got != want {
			t.Errorf("code at %d = %s, want %s", at, got, want)
		}
	}
}

func TestATotpCodeIsAcceptedOneStepAroundAndOnlyOnce(t *testing.T) {
	const now = 1_000_000
	for step, accepted := range map[int64]bool{now - 2: false, now - 1: true, now: true, now + 1: true, now + 2: false} {
		got, ok := dashboard.TotpMatch(rfc6238, dashboard.TotpCode(rfc6238, step), now, 0)
		if ok != accepted || (ok && got != step) {
			t.Errorf("step %+d: %d, %v; want accepted %v", step-now, got, ok, accepted)
		}
	}
	if _, ok := dashboard.TotpMatch(rfc6238, dashboard.TotpCode(rfc6238, now), now, now); ok {
		t.Error("a code of the step last consumed was accepted again")
	}
	if got, ok := dashboard.TotpMatch(rfc6238, dashboard.TotpCode(rfc6238, now+1), now, now); !ok || got != now+1 {
		t.Errorf("the next step after the one consumed: %d, %v", got, ok)
	}
	if _, ok := dashboard.TotpMatch(rfc6238, "000000", now, 0); ok && dashboard.TotpCode(rfc6238, now) != "000000" {
		t.Error("a wrong code was accepted")
	}
	if _, ok := dashboard.TotpMatch(rfc6238, "", now, 0); ok {
		t.Error("an empty code was accepted")
	}
}

func TestTheOtpauthURICarriesTheSecretTheIssuerAndTheAccount(t *testing.T) {
	u, err := url.Parse(dashboard.OtpauthURI(rfc6238, "operateur@example.test"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(rfc6238)
	if u.Scheme != "otpauth" || u.Host != "totp" || u.Path != "/Tableau de bord SMS:operateur@example.test" ||
		q.Get("secret") != secret || q.Get("issuer") != "Tableau de bord SMS" {
		t.Errorf("uri = %s", u)
	}
}

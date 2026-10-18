package errors

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/martialanouman/sms-platform/internal/openapi"
)

var smppByName = map[string]uint32{
	"ESME_RINVMSGLEN":  0x01,
	"ESME_RINVCMDID":   0x03,
	"ESME_RSYSERR":     0x08,
	"ESME_RINVMSGID":   0x0C,
	"ESME_RBINDFAIL":   0x0D,
	"ESME_RINVPASWD":   0x0E,
	"ESME_RCANCELFAIL": 0x11,
	"ESME_RMSGQFUL":    0x14,
	"ESME_RSUBMITFAIL": 0x45,
	"ESME_RTHROTTLED":  0x58,
}

func TestCatalogIsTheSpecTable(t *testing.T) {
	md, err := os.ReadFile("../../../docs/spec/09-contrats.md")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := openapi.SpecErrors(md)
	if err != nil {
		t.Fatal(err)
	}
	spec := map[Code]mapping{}
	for _, r := range rows {
		var m mapping
		if r.HTTP != "" {
			if m.http, err = strconv.Atoi(r.HTTP); err != nil {
				t.Fatalf("%s: HTTP %q", r.Code, r.HTTP)
			}
		}
		if r.SMPP != "" {
			var ok bool
			if m.smpp, ok = smppByName[r.SMPP]; !ok {
				t.Fatalf("%s: unknown SMPP status %q", r.Code, r.SMPP)
			}
		}
		spec[Code(r.Code)] = m
	}
	if len(spec) != len(rows) {
		t.Errorf("§9.3 lists a code twice")
	}
	for c, m := range spec {
		if got, ok := catalog[c]; !ok || got != m {
			t.Errorf("%s: catalog %+v (present %v), §9.3 %+v", c, got, ok, m)
		}
	}
	for c := range catalog {
		if _, ok := spec[c]; !ok {
			t.Errorf("%s is in the catalog, not in §9.3", c)
		}
	}
}

func TestHTTP(t *testing.T) {
	for _, c := range []struct {
		err    error
		code   Code
		status int
	}{
		{fmt.Errorf("transfer: %w", ErrInsufficientCredit), ErrInsufficientCredit, 402},
		{ErrNoRoute, ErrInternal, 500},
		{fmt.Errorf("plain"), ErrInternal, 500},
	} {
		if code, status := HTTP(c.err); code != c.code || status != c.status {
			t.Errorf("HTTP(%v) = %s, %d, want %s, %d", c.err, code, status, c.code, c.status)
		}
	}
}

func TestSMPP(t *testing.T) {
	for _, c := range []struct {
		err    error
		status uint32
	}{
		{fmt.Errorf("bind: %w", ErrMaxSessionsExceeded), 0x0D},
		{ErrForbiddenScope, 0x08},
		{fmt.Errorf("plain"), 0x08},
	} {
		if status := SMPP(c.err); status != c.status {
			t.Errorf("SMPP(%v) = %#x, want %#x", c.err, status, c.status)
		}
	}
}

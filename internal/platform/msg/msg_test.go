package msg_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/martialanouman/sms-platform/internal/platform/msg"
)

const plain = "hello 0123 secret"

func TestI1BodyNeverLeaks(t *testing.T) {
	b := msg.NewBody([]byte(plain))
	wrapped := struct{ Body msg.Body }{b}
	outputs := map[string]string{}
	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x", "%X", "%d", "%o", "%b", "%c", "%U"} {
		outputs[verb] = fmt.Sprintf(verb, b)
		outputs["struct "+verb] = fmt.Sprintf(verb, wrapped)
		outputs["pointer "+verb] = fmt.Sprintf(verb, &b)
	}
	for name, h := range map[string]func(*bytes.Buffer) slog.Handler{
		"slog text": func(w *bytes.Buffer) slog.Handler { return slog.NewTextHandler(w, nil) },
		"slog json": func(w *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(w, nil) },
	} {
		var w bytes.Buffer
		slog.New(h(&w)).Info("m", "body", b, "wrapped", wrapped)
		outputs[name] = w.String()
	}
	j, err := json.Marshal(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	outputs["json"] = string(j)
	for name, out := range outputs {
		for _, leak := range []string{plain, fmt.Sprintf("%x", plain), "secret", "104 101 108", "150 145 154", "1101000", "h e l l o", "u+0068"} {
			if strings.Contains(strings.ToLower(out), leak) {
				t.Errorf("%s leaks the body: %s", name, out)
			}
		}
	}
}

func TestRevealReturnsThePlaintext(t *testing.T) {
	if got := string(msg.NewBody([]byte(plain)).Reveal()); got != plain {
		t.Errorf("Reveal = %q, want %q", got, plain)
	}
}

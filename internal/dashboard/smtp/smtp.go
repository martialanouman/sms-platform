// Package smtp sends the dashboard's messages through an SMTP relay.
package smtp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"net/textproto"
	"strings"
	"time"
)

const timeout = 10 * time.Second

// Mailer sends through the relay at Addr under STARTTLS, verified by Roots (the system's when nil); a relay that does
// not offer it is refused, since a link in clear gives the account away. Username, when set, logs in with PLAIN.
type Mailer struct {
	Addr     string
	From     string
	Username string
	Password string
	Roots    *x509.CertPool
}

// Send delivers one plain-text message. Its error carries at most the SMTP code, never an address nor the body.
func (m Mailer) Send(ctx context.Context, to, subject, body string) error {
	if strings.ContainsAny(to+subject, "\r\n") {
		return errors.New("smtp: a header holds a line break")
	}
	host, _, err := net.SplitHostPort(m.Addr)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", m.Addr)
	if err != nil {
		return err
	}
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		_ = conn.Close()
		return err
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return reduce(err)
	}
	defer func() { _ = c.Close() }()
	return reduce(m.deliver(c, host, to, subject, body))
}

func (m Mailer) deliver(c *smtp.Client, host, to, subject, body string) error {
	if err := c.StartTLS(&tls.Config{ServerName: host, RootCAs: m.Roots, MinVersion: tls.VersionTLS12}); err != nil {
		return err
	}
	if m.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", m.Username, m.Password, host)); err != nil {
			return err
		}
	}
	if err := c.Mail(m.From); err != nil {
		return err
	}
	if err := c.Rcpt(to); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMIME-Version: 1.0\r\n"+
		"Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s",
		m.From, to, mime.QEncoding.Encode("utf-8", subject), time.Now().Format(time.RFC1123Z),
		strings.ReplaceAll(body, "\n", "\r\n")); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func reduce(err error) error {
	var refused *textproto.Error
	if errors.As(err, &refused) {
		return fmt.Errorf("smtp: %d", refused.Code)
	}
	return err
}

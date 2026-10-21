package huma

import (
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	errs "github.com/martialanouman/sms-platform/internal/platform/errors"
)

type fieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

type model struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Errors  []fieldError `json:"errors,omitempty"`
	status  int
}

func (m *model) Error() string { return m.Message }

func (m *model) GetStatus() int { return m.status }

func init() {
	huma.NewError = newError
}

func newError(status int, message string, list ...error) huma.StatusError {
	m := &model{status: status, Message: message}
	for _, e := range list {
		var code errs.Code
		if m.Code == "" && errors.As(e, &code) {
			c, s := errs.HTTP(code)
			m.Code, m.status = string(c), s
		}
		var detail huma.ErrorDetailer
		if errors.As(e, &detail) {
			d := detail.ErrorDetail()
			m.Errors = append(m.Errors, fieldError{Field: d.Location, Message: d.Message})
		}
	}
	if m.Code == "" {
		switch status {
		case http.StatusUnprocessableEntity:
			m.Code = string(errs.ErrValidation)
		default:
			m.Code, m.status = string(errs.ErrInternal), http.StatusInternalServerError
		}
	}
	return m
}

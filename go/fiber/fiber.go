// Package mitufiber adapts mitu.Mitu.Charge to Fiber.
package mitufiber

import (
	"net/http"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/adaptor"
	mitu "github.com/mitulabs/payments/go"
)

// Charge returns Fiber middleware that runs the rest of the chain only once
// the request is paid at p, as mitu.Mitu.Charge does for net/http. Handlers
// read the payment with mitu.PaymentFromContext(c.Context()). An error from
// the chain is returned to Fiber as usual.
func Charge(m *mitu.Mitu, p mitu.Price) fiber.Handler {
	charge := m.Charge(p)
	return func(c fiber.Ctx) error {
		r, err := adaptor.ConvertRequest(c, true)
		if err != nil {
			return err
		}
		w := &responseWriter{c: c, header: http.Header{}}
		charge(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			w.copyHeader()
			c.SetContext(r.Context())
			err = c.Next()
		})).ServeHTTP(w, r)
		return err
	}
}

// responseWriter writes a net/http response into the Fiber response.
type responseWriter struct {
	c           fiber.Ctx
	header      http.Header
	wroteHeader bool
}

func (w *responseWriter) Header() http.Header { return w.header }

func (w *responseWriter) copyHeader() {
	for k, vs := range w.header {
		for _, v := range vs {
			w.c.Response().Header.Add(k, v)
		}
	}
}

func (w *responseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.copyHeader()
	w.c.Status(status)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	w.c.Response().AppendBody(b)
	return len(b), nil
}

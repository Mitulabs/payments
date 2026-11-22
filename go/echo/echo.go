// Package mituecho adapts mitu.Mitu.Charge to Echo.
package mituecho

import (
	"net/http"

	"github.com/labstack/echo/v5"
	mitu "github.com/mitulabs/payments/go"
)

// Charge returns Echo middleware that runs the next handler only once the
// request is paid at p, as mitu.Mitu.Charge does for net/http. Handlers read
// the payment with mitu.PaymentFromContext(c.Request().Context()). An error
// from the next handler is returned to Echo as usual.
func Charge(m *mitu.Mitu, p mitu.Price) echo.MiddlewareFunc {
	charge := m.Charge(p)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			var err error
			charge(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				c.SetRequest(r)
				err = next(c)
			})).ServeHTTP(c.Response(), c.Request())
			return err
		}
	}
}

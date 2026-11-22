// Package mitugin adapts mitu.Mitu.Charge to Gin.
package mitugin

import (
	"net/http"

	"github.com/gin-gonic/gin"
	mitu "github.com/mitulabs/payments/go"
)

// Charge returns Gin middleware that runs the rest of the chain only once
// the request is paid at p, as mitu.Mitu.Charge does for net/http. Handlers
// read the payment with mitu.PaymentFromContext(c.Request.Context()).
func Charge(m *mitu.Mitu, p mitu.Price) gin.HandlerFunc {
	charge := m.Charge(p)
	return func(c *gin.Context) {
		paid := false
		charge(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			paid = true
			c.Request = r
			c.Next()
		})).ServeHTTP(c.Writer, c.Request)
		if !paid {
			c.Abort()
		}
	}
}

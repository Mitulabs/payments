package mitugin_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	mitu "github.com/mitulabs/payments/go"
	mitugin "github.com/mitulabs/payments/go/gin"
	"github.com/mitulabs/payments/go/internal/mitutest"
)

func newRouter(t *testing.T) (*mitutest.Fake, *gin.Engine, *int) {
	gin.SetMode(gin.TestMode)
	f := mitutest.New(t)
	m := f.Mitu(t, mitu.Options{})
	ran := new(int)
	r := gin.New()
	r.GET("/report", mitugin.Charge(m, mitu.Price{Amount: "0.50"}), func(c *gin.Context) {
		*ran++
		p, ok := mitu.PaymentFromContext(c.Request.Context())
		c.JSON(http.StatusCreated, gin.H{"paid": ok, "id": p.ID, "method": p.PaymentMethod})
	})
	r.GET("/free", mitugin.Charge(m, mitu.Price{AmountFunc: func(*http.Request) (string, error) { return "", nil }}), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	return f, r, ran
}

func serve(h http.Handler, header http.Header, target string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range header {
		r.Header[k] = v
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestCharge(t *testing.T) {
	for name, pay := range map[string]func(testing.TB, http.Header) http.Header{
		"mpp": func(t testing.TB, h http.Header) http.Header {
			return http.Header{"Authorization": {mitutest.MPP(t, h)}}
		},
		"x402": func(t testing.TB, h http.Header) http.Header {
			return http.Header{"Payment-Signature": {mitutest.X402(t, h)}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			f, r, ran := newRouter(t)

			unpaid := serve(r, nil, "/report")
			if unpaid.Code != http.StatusPaymentRequired || unpaid.Header().Get("Payment-Required") == "" || unpaid.Header().Get("Www-Authenticate") == "" {
				t.Fatalf("unpaid: %d %v", unpaid.Code, unpaid.Header())
			}
			if *ran != 0 {
				t.Fatal("handler ran unpaid")
			}

			credential := pay(t, unpaid.Header())
			paid := serve(r, credential, "/report")
			if paid.Code != http.StatusCreated || paid.Header().Get("Payment-Receipt")+paid.Header().Get("Payment-Response") == "" {
				t.Fatalf("paid: %d %v", paid.Code, paid.Header())
			}
			method := map[string]string{"mpp": "mpp_crypto", "x402": "x402"}[name]
			if got, want := paid.Body.String(), `{"id":"`+f.Payments()[0].ID+`","method":"`+method+`","paid":true}`; got != want {
				t.Errorf("body = %s, want %s", got, want)
			}

			replay := serve(r, credential, "/report")
			if replay.Code != http.StatusConflict || *ran != 1 {
				t.Fatalf("replay: %d, handler ran %d times", replay.Code, *ran)
			}
		})
	}
}

func TestChargeMituUnavailable(t *testing.T) {
	f, r, ran := newRouter(t)
	f.API.Close()
	if w := serve(r, nil, "/report"); w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") != "5" || *ran != 0 {
		t.Fatalf("status = %d, Retry-After %q", w.Code, w.Header().Get("Retry-After"))
	}
}

func TestChargeFreeRoute(t *testing.T) {
	f, r, _ := newRouter(t)
	if w := serve(r, nil, "/free"); w.Code != http.StatusNoContent || len(f.Calls()) != 0 {
		t.Fatalf("status = %d, calls %d", w.Code, len(f.Calls()))
	}
}

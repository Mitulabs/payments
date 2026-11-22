package mituecho_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v5"
	mitu "github.com/mitulabs/payments/go"
	mituecho "github.com/mitulabs/payments/go/echo"
	"github.com/mitulabs/payments/go/internal/mitutest"
)

func newServer(t *testing.T) (*mitutest.Fake, *echo.Echo, *int) {
	f := mitutest.New(t)
	m := f.Mitu(t, mitu.Options{})
	ran := new(int)
	e := echo.New()
	e.GET("/report", func(c *echo.Context) error {
		*ran++
		p, ok := mitu.PaymentFromContext(c.Request().Context())
		return c.JSON(http.StatusCreated, map[string]any{"paid": ok, "id": p.ID, "method": p.PaymentMethod})
	}, mituecho.Charge(m, mitu.Price{Amount: "0.50"}))
	e.GET("/fail", func(*echo.Context) error {
		*ran++
		return echo.NewHTTPError(http.StatusTeapot, "teapot")
	}, mituecho.Charge(m, mitu.Price{Amount: "0.50"}))
	e.GET("/free", func(c *echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	}, mituecho.Charge(m, mitu.Price{AmountFunc: func(*http.Request) (string, error) { return "", nil }}))
	return f, e, ran
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
			f, e, ran := newServer(t)

			unpaid := serve(e, nil, "/report")
			if unpaid.Code != http.StatusPaymentRequired || unpaid.Header().Get("Payment-Required") == "" || unpaid.Header().Get("Www-Authenticate") == "" {
				t.Fatalf("unpaid: %d %v", unpaid.Code, unpaid.Header())
			}
			if *ran != 0 {
				t.Fatal("handler ran unpaid")
			}

			credential := pay(t, unpaid.Header())
			paid := serve(e, credential, "/report")
			if paid.Code != http.StatusCreated || paid.Header().Get("Payment-Receipt")+paid.Header().Get("Payment-Response") == "" {
				t.Fatalf("paid: %d %v", paid.Code, paid.Header())
			}
			method := map[string]string{"mpp": "mpp_crypto", "x402": "x402"}[name]
			if got, want := paid.Body.String(), `{"id":"`+f.Payments()[0].ID+`","method":"`+method+`","paid":true}`+"\n"; got != want {
				t.Errorf("body = %s, want %s", got, want)
			}

			replay := serve(e, credential, "/report")
			if replay.Code != http.StatusConflict || *ran != 1 {
				t.Fatalf("replay: %d, handler ran %d times", replay.Code, *ran)
			}
		})
	}
}

func TestChargeHandlerError(t *testing.T) {
	_, e, ran := newServer(t)
	unpaid := serve(e, nil, "/fail")

	w := serve(e, http.Header{"Authorization": {mitutest.MPP(t, unpaid.Header())}}, "/fail")

	if w.Code != http.StatusTeapot || *ran != 1 {
		t.Fatalf("status = %d, handler ran %d times", w.Code, *ran)
	}
}

func TestChargeMituUnavailable(t *testing.T) {
	f, e, ran := newServer(t)
	f.API.Close()
	if w := serve(e, nil, "/report"); w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") != "5" || *ran != 0 {
		t.Fatalf("status = %d, Retry-After %q", w.Code, w.Header().Get("Retry-After"))
	}
}

func TestChargeFreeRoute(t *testing.T) {
	f, e, _ := newServer(t)
	if w := serve(e, nil, "/free"); w.Code != http.StatusNoContent || len(f.Calls()) != 0 {
		t.Fatalf("status = %d, calls %d", w.Code, len(f.Calls()))
	}
}

package mitufiber_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	mitu "github.com/mitulabs/payments/go"
	mitufiber "github.com/mitulabs/payments/go/fiber"
	"github.com/mitulabs/payments/go/internal/mitutest"
)

func newApp(t *testing.T) (*mitutest.Fake, *fiber.App, *int) {
	f := mitutest.New(t)
	m := f.Mitu(t, mitu.Options{})
	ran := new(int)
	app := fiber.New()
	app.Get("/report", mitufiber.Charge(m, mitu.Price{Amount: "0.50"}), func(c fiber.Ctx) error {
		*ran++
		p, ok := mitu.PaymentFromContext(c.Context())
		return c.Status(http.StatusCreated).JSON(fiber.Map{"paid": ok, "id": p.ID, "method": p.PaymentMethod})
	})
	app.Get("/fail", mitufiber.Charge(m, mitu.Price{Amount: "0.50"}), func(fiber.Ctx) error {
		*ran++
		return fiber.ErrTeapot
	})
	app.Get("/free", mitufiber.Charge(m, mitu.Price{AmountFunc: func(*http.Request) (string, error) { return "", nil }}), func(c fiber.Ctx) error {
		return c.SendStatus(http.StatusNoContent)
	})
	return f, app, ran
}

func serve(t *testing.T, app *fiber.App, header http.Header, target string) (*http.Response, string) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range header {
		r.Header[k] = v
	}
	resp, err := app.Test(r)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(body)
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
			f, app, ran := newApp(t)

			unpaid, body := serve(t, app, nil, "/report")
			if unpaid.StatusCode != http.StatusPaymentRequired || unpaid.Header.Get("Payment-Required") == "" || unpaid.Header.Get("Www-Authenticate") == "" {
				t.Fatalf("unpaid: %d %v", unpaid.StatusCode, unpaid.Header)
			}
			if want := `"resource_id":"` + f.Payments()[0].ID + `"`; !strings.Contains(body, want) || *ran != 0 {
				t.Fatalf("unpaid body %s, handler ran %d times", body, *ran)
			}

			credential := pay(t, unpaid.Header)
			paid, body := serve(t, app, credential, "/report")
			if paid.StatusCode != http.StatusCreated || paid.Header.Get("Payment-Receipt")+paid.Header.Get("Payment-Response") == "" {
				t.Fatalf("paid: %d %v", paid.StatusCode, paid.Header)
			}
			if paid.Header.Get("Cache-Control") != "no-store" {
				t.Errorf("Cache-Control = %q", paid.Header.Get("Cache-Control"))
			}
			method := map[string]string{"mpp": "mpp_crypto", "x402": "x402"}[name]
			if want := `{"id":"` + f.Payments()[0].ID + `","method":"` + method + `","paid":true}`; body != want {
				t.Errorf("body = %s, want %s", body, want)
			}

			replay, _ := serve(t, app, credential, "/report")
			if replay.StatusCode != http.StatusConflict || *ran != 1 {
				t.Fatalf("replay: %d, handler ran %d times", replay.StatusCode, *ran)
			}
		})
	}
}

func TestChargeHandlerError(t *testing.T) {
	_, app, ran := newApp(t)
	unpaid, _ := serve(t, app, nil, "/fail")

	resp, _ := serve(t, app, http.Header{"Authorization": {mitutest.MPP(t, unpaid.Header)}}, "/fail")

	if resp.StatusCode != http.StatusTeapot || *ran != 1 {
		t.Fatalf("status = %d, handler ran %d times", resp.StatusCode, *ran)
	}
}

func TestChargeMituUnavailable(t *testing.T) {
	f, app, ran := newApp(t)
	f.API.Close()
	if resp, _ := serve(t, app, nil, "/report"); resp.StatusCode != http.StatusServiceUnavailable || resp.Header.Get("Retry-After") != "5" || *ran != 0 {
		t.Fatalf("status = %d, Retry-After %q", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
}

func TestChargeFreeRoute(t *testing.T) {
	f, app, _ := newApp(t)
	if resp, _ := serve(t, app, nil, "/free"); resp.StatusCode != http.StatusNoContent || len(f.Calls()) != 0 {
		t.Fatalf("status = %d, calls %d", resp.StatusCode, len(f.Calls()))
	}
}

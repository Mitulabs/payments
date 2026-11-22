module github.com/mitulabs/payments/go/echo

go 1.26.0

require (
	github.com/labstack/echo/v5 v5.4.7
	github.com/mitulabs/payments/go v0.1.0
)

require (
	github.com/stretchr/testify v1.12.1 // indirect
	golang.org/x/net v0.62.0 // indirect
)

replace github.com/mitulabs/payments/go => ../

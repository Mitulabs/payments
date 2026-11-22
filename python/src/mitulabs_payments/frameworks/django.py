"""Django: the payment is on ``request.mitu_payment``."""

from collections.abc import Mapping
from typing import Any

from django.http import HttpRequest, HttpResponse

from ..models import Incoming, Payment, Reply
from . import payment_headers


class DjangoAdapter:
    def request_of(self, args: tuple[Any, ...], kwargs: Mapping[str, Any]) -> HttpRequest | None:
        return next((value for value in args if isinstance(value, HttpRequest)), None)

    def incoming(self, request: HttpRequest) -> Incoming:
        return Incoming(request.method or "GET", request.path, payment_headers(request.headers))

    def respond(self, reply: Reply) -> HttpResponse:
        return self.finish(HttpResponse(reply.body, status=reply.status), reply.headers)

    def finish(self, result: Any, headers: Mapping[str, str]) -> Any:
        for name, value in headers.items():
            result[name] = value
        return result

    def attach(self, request: HttpRequest, payment: Payment | None) -> None:
        request.mitu_payment = payment


ADAPTER = DjangoAdapter()

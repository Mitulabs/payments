"""FastAPI and Starlette: the route takes ``request: Request``; the payment is on
``request.state.mitu_payment``."""

from collections.abc import Mapping
from typing import Any

from fastapi.encoders import jsonable_encoder
from starlette.requests import Request
from starlette.responses import JSONResponse, Response

from ..models import Incoming, Payment, Reply
from . import payment_headers


class StarletteAdapter:
    def request_of(self, args: tuple[Any, ...], kwargs: Mapping[str, Any]) -> Request | None:
        return next(
            (value for value in (*args, *kwargs.values()) if isinstance(value, Request)), None
        )

    def incoming(self, request: Request) -> Incoming:
        return Incoming(request.method, request.url.path, payment_headers(request.headers))

    def respond(self, reply: Reply) -> Response:
        return Response(reply.body, status_code=reply.status, headers=reply.headers)

    def finish(self, result: Any, headers: Mapping[str, str]) -> Response:
        response = (
            result if isinstance(result, Response) else JSONResponse(jsonable_encoder(result))
        )
        response.headers.update(headers)
        return response

    def attach(self, request: Request, payment: Payment | None) -> None:
        request.state.mitu_payment = payment


ADAPTER = StarletteAdapter()

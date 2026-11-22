"""Flask: the payment is on ``flask.g.mitu_payment``."""

from collections.abc import Mapping
from typing import Any

import flask

from ..models import Incoming, Payment, Reply
from . import payment_headers


class FlaskAdapter:
    def request_of(self, args: tuple[Any, ...], kwargs: Mapping[str, Any]) -> flask.Request | None:
        return flask.request if flask.has_request_context() else None

    def incoming(self, request: flask.Request) -> Incoming:
        return Incoming(request.method, request.path, payment_headers(request.headers))

    def respond(self, reply: Reply) -> flask.Response:
        return flask.Response(reply.body, status=reply.status, headers=reply.headers)

    def finish(self, result: Any, headers: Mapping[str, str]) -> flask.Response:
        response = flask.make_response(result)
        response.headers.update(headers)
        return response

    def attach(self, request: flask.Request, payment: Payment | None) -> None:
        flask.g.mitu_payment = payment


ADAPTER = FlaskAdapter()

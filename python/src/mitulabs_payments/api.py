import asyncio
import os
import threading
import weakref
from collections.abc import Callable, Mapping
from typing import Final

import httpx

from ._version import __version__
from .errors import MituConfigError

TIMEOUT_SECONDS: Final = 10.0
PAYMENTS_PATH: Final = "/v1/payments"

_USER_AGENT: Final = f"mitulabs-payments-python/{__version__}"

Transport = httpx.BaseTransport | httpx.AsyncBaseTransport


class MituApi:
    """Builds and sends the requests of the payment flow, from sync and async code alike.

    Only requests to the Mitu API carry ``x-api-key``; requests to an ``agent_url`` carry the
    agent's payment headers and nothing else. Redirects are never followed.
    """

    def __init__(self, api_key: str, api_url: str, *, transport: Transport | None = None) -> None:
        self._api_key = api_key
        self._payments_url = f"{api_url}{PAYMENTS_PATH}"
        self._transport = transport
        self._timeout = httpx.Timeout(TIMEOUT_SECONDS).as_dict()
        self._lock = threading.Lock()
        self._client: httpx.Client | None = None
        # One per event loop: pooled connections cannot move between loops, and Django runs
        # each async view of a WSGI app on a loop of its own.
        self._async_clients: weakref.WeakKeyDictionary[
            asyncio.AbstractEventLoop, httpx.AsyncClient
        ] = weakref.WeakKeyDictionary()
        _after_fork_in_child(self._forget_clients)

    def create_payment(self, body: Mapping[str, object]) -> httpx.Request:
        return self._request("POST", self._payments_url, self._api_headers(), json=body)

    def get_payment(self, payment_id: str) -> httpx.Request:
        return self._request("GET", f"{self._payments_url}/{payment_id}", self._api_headers())

    def agent(self, url: str, payment_headers: Mapping[str, str]) -> httpx.Request:
        return self._request("GET", url, {**self._base_headers(), **payment_headers})

    def send(self, request: httpx.Request) -> httpx.Response:
        with self._lock:
            if not self._client:
                self._client = httpx.Client(
                    follow_redirects=False, transport=self._typed(httpx.BaseTransport)
                )
            client = self._client
        return client.send(request)

    async def asend(self, request: httpx.Request) -> httpx.Response:
        loop = asyncio.get_running_loop()
        with self._lock:
            if not (client := self._async_clients.get(loop)):
                client = self._async_clients[loop] = httpx.AsyncClient(
                    follow_redirects=False, transport=self._typed(httpx.AsyncBaseTransport)
                )
        return await client.send(request)

    def close(self) -> None:
        with self._lock:
            client, self._client = self._client, None
        if client:
            client.close()

    async def aclose(self) -> None:
        with self._lock:
            client = self._async_clients.pop(asyncio.get_running_loop(), None)
            self._async_clients.clear()
        if client:
            await client.aclose()
        self.close()

    def _request(
        self,
        method: str,
        url: str,
        headers: Mapping[str, str],
        *,
        json: Mapping[str, object] | None = None,
    ) -> httpx.Request:
        return httpx.Request(
            method, url, headers=headers, json=json, extensions={"timeout": self._timeout}
        )

    def _base_headers(self) -> dict[str, str]:
        return {"accept": "application/json", "user-agent": _USER_AGENT}

    def _api_headers(self) -> dict[str, str]:
        return {**self._base_headers(), "x-api-key": self._api_key}

    def _typed[T: Transport](self, kind: type[T]) -> T | None:
        match self._transport:
            case None:
                return None
            case transport if isinstance(transport, kind):
                return transport
        raise MituConfigError(f"transport must be an {kind.__name__} for this view")

    def _forget_clients(self) -> None:
        # Not closed: the parent process still owns those pooled sockets.
        self._client = None
        self._async_clients = weakref.WeakKeyDictionary()


def _after_fork_in_child(method: Callable[[], None]) -> None:
    # Pooled sockets must not be shared with the parent (Gunicorn --preload). The weak
    # reference keeps the hook from holding the client alive.
    if not hasattr(os, "register_at_fork"):
        return
    ref = weakref.WeakMethod(method)

    def run() -> None:
        if bound := ref():
            bound()

    os.register_at_fork(after_in_child=run)

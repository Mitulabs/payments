"""Mitu's public payments API and its /pay endpoint, as the SDK sees them over HTTP."""

import json
from dataclasses import dataclass, field
from typing import Any

import httpx
from wire import MPP_CHALLENGE, PAYMENT_ID

from mitulabs_payments import __version__

API_KEY = "sk_test_" + "a1" * 24
API_PREFIX = "/economy/"
PAYMENTS_PATH = "/economy/v1/public/payments"
AGENT_ORIGIN = "https://api.mitu.ai"
TX_HASH = "0x" + "9f" * 32
RECEIPT = "eyJzdGF0dXMiOiJzdWNjZXNzIn0"
X402_RECEIPT = "eyJzdWNjZXNzIjp0cnVlfQ=="
PAYMENT_REQUIRED = "eyJ4NDAyVmVyc2lvbiI6Mn0="
SOLANA_CHALLENGE = 'Payment id="sol", realm="api.mitu.ai", method="solana", intent="charge"'
PROBLEM_JSON = {"content-type": "application/problem+json; charset=utf-8"}
CHALLENGE_BODY = {
    "type": "https://paymentauth.org/problems/payment-required",
    "status": 402,
    "intent": "charge",
}


@dataclass
class FakeMitu:
    payments: dict[str, dict[str, Any]] = field(default_factory=dict)
    requests: list[httpx.Request] = field(default_factory=list)
    agent_answer: tuple[int, dict[str, str], dict[str, Any]] | None = None
    api_status: int | None = None
    down: type[httpx.TransportError] | None = None

    @property
    def transport(self) -> httpx.MockTransport:
        return httpx.MockTransport(lambda request: self.handle(request))

    def add_payment(
        self,
        payment_id: str = PAYMENT_ID,
        *,
        amount: str = "0.01",
        currency: str = "USD",
        method: str = "GET",
        path: str = "/report",
    ) -> None:
        self.payments[payment_id] = {
            "id": payment_id,
            "object": "payment",
            "status": "pending",
            "amount": amount,
            "currency": currency,
            "reference": f"sdk_ref_{payment_id}",
            "metadata": {"sdk": f"mitulabs-payments@{__version__}", "method": method, "path": path},
            "url": f"https://mitu.ai/pay/{payment_id}",
            "agent_url": f"{AGENT_ORIGIN}/pay/{payment_id}",
        }

    def sent(self, method: str, path_prefix: str) -> list[httpx.Request]:
        return [
            request
            for request in self.requests
            if request.method == method and request.url.path.startswith(path_prefix)
        ]

    @property
    def created(self) -> list[dict[str, Any]]:
        return [json.loads(request.content) for request in self.sent("POST", PAYMENTS_PATH)]

    @property
    def agent_requests(self) -> list[httpx.Request]:
        return self.sent("GET", "/pay/")

    def handle(self, request: httpx.Request) -> httpx.Response:
        self.requests.append(request)
        if self.down:
            raise self.down("Mitu is down", request=request)
        if request.url.path.startswith(API_PREFIX):
            return self._api(request)
        return self._agent(request)

    def _api(self, request: httpx.Request) -> httpx.Response:
        if request.headers.get("x-api-key") != API_KEY:
            return httpx.Response(401, json={"error": "Unauthorized"})
        if self.api_status:
            return httpx.Response(self.api_status, json={"error": "Unavailable"})
        match request.method, request.url.path.removeprefix(PAYMENTS_PATH):
            case "POST", "":
                body = json.loads(request.content)
                payment_id = f"pmt_new{len(self.payments):016d}"
                self.add_payment(payment_id, amount=f"{body['amount']:.2f}")
                self.payments[payment_id] |= {
                    "currency": body["currency"].upper(),
                    "reference": body["reference"],
                    "metadata": body["metadata"],
                }
                return httpx.Response(
                    201, json={"message": "Payment created", "data": self.payments[payment_id]}
                )
            case "GET", path if payment := self.payments.get(path.removeprefix("/")):
                return httpx.Response(200, json={"message": "Payment retrieved", "data": payment})
        return httpx.Response(404, json={"error": "Not Found"})

    def _agent(self, request: httpx.Request) -> httpx.Response:
        paying = any(
            name in request.headers for name in ("authorization", "payment-signature", "x-payment")
        )
        if not paying:
            return httpx.Response(
                402,
                headers=[
                    *PROBLEM_JSON.items(),
                    ("www-authenticate", MPP_CHALLENGE),
                    ("www-authenticate", SOLANA_CHALLENGE),
                    ("payment-required", PAYMENT_REQUIRED),
                    ("cache-control", "no-store"),
                    ("set-cookie", "mitu=1"),
                ],
                json=CHALLENGE_BODY,
            )
        if self.agent_answer:
            status, headers, body = self.agent_answer
            return httpx.Response(status, headers={**PROBLEM_JSON, **headers}, json=body)
        receipt_header = (
            {"payment-response": X402_RECEIPT}
            if "authorization" not in request.headers
            else {"payment-receipt": RECEIPT}
        )
        return httpx.Response(
            200,
            headers=receipt_header,
            json={
                "status": "paid",
                "resource_type": "payment",
                "resource_id": request.url.path.rsplit("/", 1)[-1],
                "receipt": {
                    "transaction_id": "txn_1",
                    "tx_hash": TX_HASH,
                    "amount_paid": "0.01",
                    "currency": "USDC",
                    "network": "tempo",
                    "payment_method": "mpp",
                    "confirmed_at": "2030-01-01T00:00:00.000Z",
                },
            },
        )

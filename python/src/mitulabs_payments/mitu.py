import functools
import inspect
from collections.abc import Awaitable, Callable
from dataclasses import dataclass
from typing import Any, cast

from .amounts import parse_currency, parse_description, parse_price
from .api import MituApi, Transport
from .charge import arun, charge, run
from .environment import resolve_environment
from .errors import MituConfigError
from .frameworks import Adapter, installed_adapters
from .models import Paid, Price, Reply

PriceFn = Callable[[Any], str | None | Awaitable[str | None]]


@dataclass(frozen=True, slots=True)
class _Route:
    amount: str | PriceFn
    currency: str
    description: str | None
    fixed: Price | None

    def price(self, amount: str | None) -> Price | None:
        if amount is None:
            return None
        return self.fixed or Price(parse_price(amount), self.currency, self.description)


class Mitu:
    """Puts Mitu's payment flow in front of your routes.

    Agents pay with MPP or x402. Mitu issues the challenges, settles the payment on chain and
    sends your ``payment.completed`` webhook; this object only asks Mitu for a payment, checks
    that the one an agent presents is for the route it calls, and serves the route once paid.
    """

    def __init__(
        self,
        api_key: str,
        *,
        default_currency: str = "USD",
        base_url: str | None = None,
        transport: Transport | None = None,
    ) -> None:
        resolved = resolve_environment(api_key, base_url)
        self.environment = resolved.environment
        self.default_currency = parse_currency(default_currency)
        self._api = MituApi(api_key, resolved.base_url, transport=transport)

    def __repr__(self) -> str:
        return f"Mitu(environment={self.environment!r})"

    def charge[**P, R](
        self,
        amount: str | PriceFn,
        *,
        currency: str | None = None,
        description: str | None = None,
    ) -> Callable[[Callable[P, R]], Callable[P, R]]:
        """Route decorator charging ``amount`` in ``currency`` (default ``default_currency``).

        ``amount`` is a decimal string with at most two decimal places, or a function of the
        request returning one, or None to serve that request free. The view reads the
        :class:`Payment` from ``request.state.mitu_payment`` (FastAPI), ``g.mitu_payment``
        (Flask) or ``request.mitu_payment`` (Django).
        """
        route_currency = self.default_currency if currency is None else parse_currency(currency)
        route_description = parse_description(description)
        fixed: Price | None
        match amount:
            case str():
                fixed = Price(parse_price(amount), route_currency, route_description)
            case _ if callable(amount):
                fixed = None
            case _:
                raise MituConfigError("amount must be a decimal string or a function")
        route = _Route(amount, route_currency, route_description, fixed)

        def decorate(view: Callable[P, R]) -> Callable[P, R]:
            if inspect.iscoroutinefunction(view):
                return cast(Callable[P, R], self._wrap_async(route, view))
            return cast(Callable[P, R], self._wrap_sync(route, view))

        return decorate

    async def close(self) -> None:
        """Closes the HTTP connections to Mitu."""
        await self._api.aclose()

    def close_sync(self) -> None:
        """:meth:`close` for sync apps, such as Flask and Django."""
        self._api.close()

    def _wrap_async(
        self, route: _Route, view: Callable[..., Awaitable[Any]]
    ) -> Callable[..., Awaitable[Any]]:
        adapters = installed_adapters()

        @functools.wraps(view)
        async def charged(*args: Any, **kwargs: Any) -> Any:
            adapter, request = _find(adapters, args, kwargs, view)
            amount = route.amount(request) if callable(route.amount) else route.amount
            if inspect.isawaitable(amount):
                amount = await amount
            if not (price := route.price(amount)):
                adapter.attach(request, None)
                return await view(*args, **kwargs)
            match await arun(self._api, charge(self._api, price, adapter.incoming(request))):
                case Reply() as reply:
                    return adapter.respond(reply)
                case Paid(payment, headers):
                    adapter.attach(request, payment)
                    return adapter.finish(await view(*args, **kwargs), headers)

        return charged

    def _wrap_sync(self, route: _Route, view: Callable[..., Any]) -> Callable[..., Any]:
        adapters = installed_adapters()

        @functools.wraps(view)
        def charged(*args: Any, **kwargs: Any) -> Any:
            adapter, request = _find(adapters, args, kwargs, view)
            amount = route.amount(request) if callable(route.amount) else route.amount
            if inspect.isawaitable(amount):
                if inspect.iscoroutine(amount):
                    amount.close()
                raise MituConfigError(f"{view.__qualname__}: an async price needs an async view")
            if not (price := route.price(amount)):
                adapter.attach(request, None)
                return view(*args, **kwargs)
            match run(self._api, charge(self._api, price, adapter.incoming(request))):
                case Reply() as reply:
                    return adapter.respond(reply)
                case Paid(payment, headers):
                    adapter.attach(request, payment)
                    return adapter.finish(view(*args, **kwargs), headers)

        return charged


def _find(
    adapters: list[Adapter], args: Any, kwargs: Any, view: Callable[..., Any]
) -> tuple[Adapter, Any]:
    for adapter in adapters:
        if (request := adapter.request_of(args, kwargs)) is not None:
            return adapter, request
    raise MituConfigError(
        f"@mitu.charge found no request for {view.__qualname__}: FastAPI routes must take "
        "`request: Request`; Flask and Django views are found on their own"
    )

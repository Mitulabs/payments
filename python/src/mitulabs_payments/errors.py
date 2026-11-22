class MituError(Exception):
    """Base class for every error the package raises. Messages never contain keys or credentials."""

    code = "MITU_ERROR"


class MituConfigError(MituError):
    code = "MITU_CONFIG"


class MituApiError(MituError):
    """The Mitu API refused a request in a way a retry will not fix, such as an invalid key."""

    code = "MITU_API"

    def __init__(self, status: int, message: str) -> None:
        super().__init__(message)
        self.status = status

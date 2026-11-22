"""Credentials made by the real client libraries, so the parser is tested against their output.

Generated once with pympp 0.26.0 and x402 2.29.0 (no longer dependencies): a Challenge carrying
mppx's scope metadata, echoed by pympp's ``format_authorization``, and an x402 v2 payment payload
answering a PAYMENT-REQUIRED whose resource is Mitu's agent_url, encoded by
``encode_payment_signature_header``. Both are for ``PAYMENT_ID`` and carry dummy signatures.
"""

PAYMENT_ID = "pmt_0123456789abcdefghij"

MPP_CHALLENGE = (
    'Payment id="zDQau_l9LCKwmaDo9feL-qK9RiaHM38aKXgkeXIrO28", realm="api.mitu.ai", m'
    'ethod="tempo", intent="charge", request="eyJhbW91bnQiOiIxNTAwMDAwIiwiY3VycmVuY3k'
    "iOiIweDIwQzAwMDAwMDAwMDAwMDAwMDAwMDAwMGI5NTM3ZDExYzYwRThiNTAiLCJyZWNpcGllbnQiOiI"
    'weGFiYWJhYmFiYWJhYmFiYWJhYmFiYWJhYmFiYWJhYmFiYWJhYmFiYWIifQ", expires="2030-01-0'
    '1T00:00:00.000Z", description="Report", opaque="eyJfbXBweF9zY29wZSI6InBtdF8wMTIz'
    'NDU2Nzg5YWJjZGVmZ2hpaiJ9"'
)
MPP_AUTHORIZATION = (
    "Payment eyJjaGFsbGVuZ2UiOnsiZGVzY3JpcHRpb24iOiJSZXBvcnQiLCJleHBpcmVzIjoiMjAzMC0wMS0w"
    "MVQwMDowMDowMC4wMDBaIiwiaWQiOiJ6RFFhdV9sOUxDS3dtYURvOWZlTC1xSzlSaWFITTM4YUtYZ2tlWEly"
    "TzI4IiwiaW50ZW50IjoiY2hhcmdlIiwibWV0aG9kIjoidGVtcG8iLCJvcGFxdWUiOiJleUpmYlhCd2VGOXpZ"
    "Mjl3WlNJNkluQnRkRjh3TVRJek5EVTJOemc1WVdKalpHVm1aMmhwYWlKOSIsInJlYWxtIjoiYXBpLm1pdHUu"
    "YWkiLCJyZXF1ZXN0IjoiZXlKaGJXOTFiblFpT2lJeE5UQXdNREF3SWl3aVkzVnljbVZ1WTNraU9pSXdlREl3"
    "UXpBd01EQXdNREF3TURBd01EQXdNREF3TURBd01HSTVOVE0zWkRFeFl6WXdSVGhpTlRBaUxDSnlaV05wY0ds"
    "bGJuUWlPaUl3ZUdGaVlXSmhZbUZpWVdKaFltRmlZV0poWW1GaVlXSmhZbUZpWVdKaFltRmlZV0poWW1GaVlX"
    "SWlmUSJ9LCJwYXlsb2FkIjp7InNpZ25hdHVyZSI6IjB4Y2RjZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2RjZGNk"
    "Y2RjZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2Rj"
    "ZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2RjZCIsInR5cGUiOiJ0cmFuc2FjdGlvbiJ9LCJz"
    "b3VyY2UiOiJkaWQ6cGtoOmVpcDE1NTo0MjE3OjB4ZWZlZmVmZWZlZmVmZWZlZmVmZWZlZmVmZWZlZmVmZWZl"
    "ZmVmZWZlZiJ9"
)
X402_PAYMENT_SIGNATURE = (
    "eyJ4NDAyVmVyc2lvbiI6MiwicGF5bG9hZCI6eyJzaWduYXR1cmUiOiIweGNkY2RjZGNkY2RjZGNkY2RjZGNk"
    "Y2RjZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2Rj"
    "ZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2RjZGNkY2QiLCJhdXRob3JpemF0aW9u"
    "Ijp7ImZyb20iOiIweGVmZWZlZmVmZWZlZmVmZWZlZmVmZWZlZmVmZWZlZmVmZWZlZmVmZWYiLCJ0byI6IjB4"
    "YWJhYmFiYWJhYmFiYWJhYmFiYWJhYmFiYWJhYmFiYWJhYmFiYWJhYiIsInZhbHVlIjoiMTUwMDAwMCIsInZh"
    "bGlkQWZ0ZXIiOiIwIiwidmFsaWRCZWZvcmUiOiIxODkzNDU2MDAwIiwibm9uY2UiOiIweDAwMDAwMDAwMDAw"
    "MDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAifX0sImFjY2Vw"
    "dGVkIjp7InNjaGVtZSI6ImV4YWN0IiwibmV0d29yayI6ImVpcDE1NTo4NDUzIiwiYXNzZXQiOiIweDgzMzU4"
    "OWZDRDZlRGI2RTA4ZjRjN0MzMkQ0ZjcxYjU0YmRBMDI5MTMiLCJhbW91bnQiOiIxNTAwMDAwIiwicGF5VG8i"
    "OiIweGFiYWJhYmFiYWJhYmFiYWJhYmFiYWJhYmFiYWJhYmFiYWJhYmFiYWIiLCJtYXhUaW1lb3V0U2Vjb25k"
    "cyI6MzAwLCJleHRyYSI6eyJuYW1lIjoiVVNEIENvaW4iLCJ2ZXJzaW9uIjoiMiJ9fSwicmVzb3VyY2UiOnsi"
    "dXJsIjoiaHR0cHM6Ly9hcGkubWl0dS5haS9wYXkvcG10XzAxMjM0NTY3ODlhYmNkZWZnaGlqIiwiZGVzY3Jp"
    "cHRpb24iOiJSZXBvcnQiLCJtaW1lVHlwZSI6ImFwcGxpY2F0aW9uL2pzb24ifX0="
)

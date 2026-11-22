/** The handler's response with the payment's receipt headers added. */
export function withHeaders(response: Response, headers: Headers): Response {
  const added = [...headers]
  if (!added.length) return response
  const merged = new Headers(response.headers)
  for (const [name, value] of added) merged.set(name, value)
  return new Response(response.body, { status: response.status, statusText: response.statusText, headers: merged })
}

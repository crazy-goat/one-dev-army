const API_BASE = '/api/v2'

export class APIError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message)
    this.name = 'APIError'
  }
}

export async function fetchAPI<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    headers: {
      'Content-Type': 'application/json',
      ...options?.headers,
    },
    ...options,
  })

  if (!response.ok) {
    const text = await response.text().catch(() => 'Unknown error')
    throw new APIError(response.status, `API error ${response.status}: ${text}`)
  }

  return response.json()
}

export async function postAPI<T>(path: string, body?: unknown): Promise<T> {
  return fetchAPI<T>(path, {
    method: 'POST',
    body: body ? JSON.stringify(body) : undefined,
  })
}

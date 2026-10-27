export interface ApiErrorBody {
  error: { code: string; message: string; details?: unknown }
}

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message)
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`/api${path}`, init)
  const isJSON = res.headers.get('content-type')?.includes('application/json') ?? false
  const body: unknown = isJSON ? await res.json() : null
  if (!res.ok) {
    const err = (body as ApiErrorBody | null)?.error
    throw new ApiError(res.status, err?.code ?? 'HTTP_ERROR', err?.message ?? res.statusText)
  }
  return body as T
}

export interface Health {
  status: string
}

export const api = {
  health: () => request<Health>('/health'),
}

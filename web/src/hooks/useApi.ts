import { useState, useEffect, useCallback } from 'react'

interface UseApiResult<T> {
  data: T | null
  loading: boolean
  error: string | null
  refetch: () => void
}

export function useApi<T>(fetcher: () => Promise<T>, deps: unknown[] = []): UseApiResult<T> {
  const [data, setData] = useState<T | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refetch = useCallback(() => {
    setLoading(true)
    setError(null)
    fetcher()
      .then(setData)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps)

  useEffect(() => { refetch() }, [refetch])

  return { data, loading, error, refetch }
}

export function usePolling<T>(fetcher: () => Promise<T>, intervalMs: number | null, deps: unknown[] = []): UseApiResult<T> {
  const result = useApi(fetcher, deps)

  useEffect(() => {
    if (!intervalMs) return
    const id = setInterval(result.refetch, intervalMs)
    return () => clearInterval(id)
  }, [result.refetch, intervalMs])

  return result
}

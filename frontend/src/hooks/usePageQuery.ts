import {useCallback, useEffect, useRef, useState} from 'react';
import type {ApiRequest} from '../api-client';
import {errorMessage, type Notify, type Page} from '../types';

export function usePageQuery<T>(api: ApiRequest, path: string, notify: Notify) {
  const [items, setItems] = useState<T[]>([]);
  const [loading, setLoading] = useState(true);
  const [nextCursor, setNextCursor] = useState('');
  const [cursor, setCursor] = useState('');
  const [history, setHistory] = useState<string[]>([]);
  const [previousPath, setPreviousPath] = useState(path);
  if (previousPath !== path) {
    setPreviousPath(path);
    setCursor('');
    setHistory([]);
  }
  const controller = useRef<AbortController | null>(null);
  const sequence = useRef(0);
  const load = useCallback(async () => {
    controller.current?.abort();
    const active = new AbortController();
    controller.current = active;
    const version = ++sequence.current;
    setLoading(true);
    try {
      const page = await api<Page<T>>(
        `${path}${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ''}`,
        {signal: active.signal},
      );
      if (version !== sequence.current || active.signal.aborted) return;
      setItems(page.items || []);
      setNextCursor(page.next_cursor || '');
    } catch (error: unknown) {
      if (!active.signal.aborted && version === sequence.current)
        notify('error', errorMessage(error));
    } finally {
      if (version === sequence.current && !active.signal.aborted) setLoading(false);
    }
  }, [api, path, cursor, notify]);
  useEffect(() => {
    load();
    return () => controller.current?.abort();
  }, [load]);
  useEffect(() => {
    const refresh = () => {
      if (document.visibilityState === 'visible' && navigator.onLine) load();
    };
    window.addEventListener('online', refresh);
    document.addEventListener('visibilitychange', refresh);
    return () => {
      window.removeEventListener('online', refresh);
      document.removeEventListener('visibilitychange', refresh);
    };
  }, [load]);
  return {
    items,
    loading,
    load,
    pageNumber: history.length + 1,
    hasPrevious: history.length > 0,
    hasNext: Boolean(nextCursor),
    next: () => {
      if (nextCursor) {
        setHistory((old) => [...old, cursor]);
        setCursor(nextCursor);
      }
    },
    previous: () => {
      setCursor(history.at(-1) || '');
      setHistory((old) => old.slice(0, -1));
    },
  };
}

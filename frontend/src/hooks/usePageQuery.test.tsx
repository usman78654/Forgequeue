import {act, renderHook, waitFor} from '@testing-library/react';
import {expect, it, vi} from 'vitest';
import type {ApiRequest} from '../api-client';
import {usePageQuery} from './usePageQuery';

it('ignores obsolete responses and resets cursors when filters change', async () => {
  const pending: Array<(value: unknown) => void> = [];
  const request = vi.fn((path: string, options?: RequestInit) => {
    expect(path).toContain('/v1/jobs?');
    expect(options?.signal).toBeInstanceOf(AbortSignal);
    return new Promise((resolve) => pending.push(resolve));
  });
  const notify = vi.fn();
  const {result, rerender} = renderHook(
    ({path}) => usePageQuery<{id: string}>(request as ApiRequest, path, notify),
    {initialProps: {path: '/v1/jobs?limit=50'}},
  );
  rerender({path: '/v1/jobs?limit=50&status=dead'});
  await act(async () => {
    pending[1]?.({items: [{id: 'new'}], next_cursor: 'next'});
  });
  await act(async () => {
    pending[0]?.({items: [{id: 'old'}]});
  });
  expect(result.current.items).toEqual([{id: 'new'}]);
  act(() => result.current.next());
  await waitFor(() => expect(request.mock.calls.length).toBe(3));
  expect(request.mock.calls[2]?.[0]).toContain('cursor=next');
  rerender({path: '/v1/jobs?limit=50&status=running'});
  expect(result.current.pageNumber).toBe(1);
  expect(request.mock.calls.at(-1)?.[0]).not.toContain('cursor=');
});

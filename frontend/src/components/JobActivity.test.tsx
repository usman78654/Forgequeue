import {act, render, screen} from '@testing-library/react';
import {afterEach, expect, it, vi} from 'vitest';
import {JobActivity} from './JobActivity';

afterEach(() => vi.unstubAllGlobals());
it('resumes from the last event, ignores duplicates and closes on unmount', () => {
  const close = vi.fn();
  const listeners = new Map<string, (event: MessageEvent) => void>();
  const constructor = vi.fn(function () {
    return {
      close,
      addEventListener: (type: string, listener: (event: MessageEvent) => void) =>
        listeners.set(type, listener),
    };
  });
  vi.stubGlobal('EventSource', constructor);
  const event = {
    id: 5,
    job_id: 'job-1',
    type: 'log' as const,
    level: 'info' as const,
    message: 'initial',
    created_at: '2030-01-01T00:00:00Z',
  };
  const {unmount} = render(<JobActivity jobId="job-1" initialEvents={[event]} />);
  expect(constructor).toHaveBeenCalledWith('/v1/jobs/job-1/events/stream?after=5');
  act(() => listeners.get('log')?.(new MessageEvent('log', {data: JSON.stringify(event)})));
  expect(screen.getAllByText(/initial/)).toHaveLength(1);
  act(() =>
    listeners.get('log')?.(
      new MessageEvent('log', {data: JSON.stringify({...event, id: 6, message: 'new log'})}),
    ),
  );
  expect(screen.getByText(/new log/)).toBeVisible();
  unmount();
  expect(close).toHaveBeenCalledTimes(1);
});

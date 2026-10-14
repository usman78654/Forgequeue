import {fireEvent, render, screen} from '@testing-library/react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import {Builder} from './Builder';
import {allowNavigation} from '../routing';
import type {ApiRequest} from '../api-client';

vi.mock('@xyflow/react', () => ({
  Background: () => null,
  Controls: () => null,
  ReactFlow: () => <div>Graph</div>,
}));

afterEach(() => vi.restoreAllMocks());

describe('builder safeguards', () => {
  it('blocks viewers from submitting workflows', () => {
    const api = vi.fn() as unknown as ApiRequest;
    render(
      <Builder
        api={api}
        notify={vi.fn()}
        principal={{id: 'viewer', name: 'Viewer', role: 'viewer'}}
      />,
    );
    expect(screen.getByRole('button', {name: 'Run workflow'})).toBeDisabled();
    expect(api).not.toHaveBeenCalled();
  });

  it('rejects cyclic imports and protects unsaved changes during navigation', () => {
    const notify = vi.fn();
    render(<Builder api={vi.fn() as unknown as ApiRequest} notify={notify} />);
    fireEvent.change(screen.getByLabelText('Import workflow JSON'), {
      target: {
        value: JSON.stringify({
          name: 'cycle',
          steps: [
            {key: 'a', type: 'echo', condition: 'success()', payload: {}, depends_on: ['b']},
            {key: 'b', type: 'echo', condition: 'success()', payload: {}, depends_on: ['a']},
          ],
        }),
      },
    });
    fireEvent.click(screen.getByRole('button', {name: 'Import workflow'}));
    expect(notify).toHaveBeenCalledWith('error', expect.stringContaining('acyclic'));
    fireEvent.change(screen.getByLabelText('Name'), {target: {value: 'changed'}});
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false);
    expect(allowNavigation()).toBe(false);
    confirm.mockReturnValue(true);
    expect(allowNavigation()).toBe(true);
  });
});

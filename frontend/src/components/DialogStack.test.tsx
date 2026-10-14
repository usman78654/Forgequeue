import {fireEvent, render, screen} from '@testing-library/react';
import {describe, expect, it, vi} from 'vitest';
import {Dialog} from './Dialog';

describe('nested confirmations', () => {
  it('dismisses only the topmost dialog with Escape', () => {
    const parentClose = vi.fn();
    const childClose = vi.fn();
    render(
      <Dialog title="Job details" onClose={parentClose}>
        <Dialog title="Delete artifact?" onClose={childClose}>
          <button>Confirm deletion</button>
        </Dialog>
      </Dialog>,
    );
    fireEvent.keyDown(screen.getByText('Confirm deletion'), {key: 'Escape'});
    expect(childClose).toHaveBeenCalledOnce();
    expect(parentClose).not.toHaveBeenCalled();
  });
});

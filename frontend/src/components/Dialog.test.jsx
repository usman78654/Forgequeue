import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {axe} from 'jest-axe';
import {describe, expect, it, vi} from 'vitest';
import {Dialog} from './Dialog';

describe('Dialog', () => {
  it('has a usable accessible name and closes with Escape', async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    const {container} = render(
      <Dialog title="Cancel job" onClose={onClose} actions={<button>Confirm</button>}>
        <p>This stops future attempts.</p>
      </Dialog>,
    );
    expect(screen.getByRole('dialog', {name: 'Cancel job'})).toBeInTheDocument();
    await user.keyboard('{Escape}');
    expect(onClose).toHaveBeenCalledOnce();
    expect(await axe(container)).toHaveNoViolations();
  });
});

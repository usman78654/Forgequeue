import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {axe} from 'jest-axe';
import {describe, expect, it, vi} from 'vitest';
import {ConnectionPanel} from './ConnectionPanel';

describe('ConnectionPanel', () => {
  it('validates an empty key and exposes the error accessibly', async () => {
    const user = userEvent.setup();
    const {container} = render(<ConnectionPanel onConnect={vi.fn()} />);
    await user.click(screen.getByRole('button', {name: 'Connect'}));
    expect(screen.getByRole('alert')).toHaveTextContent('Enter an API key.');
    expect(await axe(container)).toHaveNoViolations();
  });

  it('submits the trimmed key once', async () => {
    const user = userEvent.setup();
    const onConnect = vi.fn().mockResolvedValue(undefined);
    render(<ConnectionPanel onConnect={onConnect} />);
    await user.type(screen.getByLabelText('API key'), '  fq_example  ');
    await user.click(screen.getByRole('button', {name: 'Connect'}));
    expect(onConnect).toHaveBeenCalledOnce();
    expect(onConnect).toHaveBeenCalledWith('fq_example');
  });
});

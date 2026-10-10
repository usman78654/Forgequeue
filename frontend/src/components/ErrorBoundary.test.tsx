import {render, screen} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {afterEach, describe, expect, it, vi} from 'vitest';
import {ErrorBoundary} from './ErrorBoundary';

let shouldThrow = true;

function UnstablePage() {
  if (shouldThrow) throw new Error('sensitive internal detail');
  return <p>Recovered page</p>;
}

describe('ErrorBoundary', () => {
  afterEach(() => {
    shouldThrow = true;
    vi.restoreAllMocks();
  });

  it('isolates render failures and offers a recovery action without exposing error details', async () => {
    const user = userEvent.setup();
    vi.spyOn(console, 'error').mockImplementation(() => undefined);
    render(
      <ErrorBoundary resetKey="jobs">
        <UnstablePage />
      </ErrorBoundary>,
    );

    expect(screen.getByRole('alert')).toHaveTextContent('This page could not be displayed');
    expect(screen.queryByText('sensitive internal detail')).not.toBeInTheDocument();
    shouldThrow = false;
    await user.click(screen.getByRole('button', {name: 'Try again'}));
    expect(screen.getByText('Recovered page')).toBeVisible();
  });
});

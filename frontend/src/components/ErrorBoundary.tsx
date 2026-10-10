import {Component, type ErrorInfo, type ReactNode} from 'react';

interface ErrorBoundaryProps {
  children: ReactNode;
  resetKey: string;
}

interface ErrorBoundaryState {
  error: Error | null;
}

export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = {error: null};

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return {error};
  }

  componentDidUpdate(previous: ErrorBoundaryProps) {
    if (previous.resetKey !== this.props.resetKey && this.state.error) {
      this.setState({error: null});
    }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('ForgeQueue page render failed', {
      name: error.name,
      componentStack: info.componentStack,
    });
  }

  render() {
    if (!this.state.error) return this.props.children;
    return (
      <section className="state-card error-screen" role="alert">
        <strong>This page could not be displayed</strong>
        <span>
          The failure was isolated. Your session and other console pages are still available.
        </span>
        <button onClick={() => this.setState({error: null})}>Try again</button>
      </section>
    );
  }
}

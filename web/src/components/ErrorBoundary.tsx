import { Component, type ErrorInfo, type ReactNode } from 'react';

type ErrorBoundaryProps = {
  /** Changes whenever the surrounding route changes; a crashed view recovers without a full reload. */
  resetKey: string;
  children: ReactNode;
  /** Injectable for tests; defaults to a full page reload. */
  reload?: () => void;
};

type ErrorBoundaryState = { error: Error | null };

/** The only class component in the app: React still requires one for error boundaries. */
export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    console.error('Blunt Code view crashed', error, info.componentStack);
  }

  componentDidUpdate(prevProps: ErrorBoundaryProps) {
    if (this.state.error && prevProps.resetKey !== this.props.resetKey) this.setState({ error: null });
  }

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;
    return <section className="error-panel" role="alert">
      <h2>Something went wrong</h2>
      <p>This view stopped unexpectedly. Your workspaces and analysis data are safe on disk — use the navigation above to continue, or reload the view.</p>
      <button className="button secondary" onClick={this.props.reload ?? (() => window.location.reload())}>Reload view</button>
      <details>
        <summary>Error details</summary>
        <pre>{error.stack ?? `${error.name}: ${error.message}`}</pre>
      </details>
    </section>;
  }
}

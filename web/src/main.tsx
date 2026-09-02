import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './App';
import './styles.css';
import './css/animations.css';
// Clarity pass — consolidated navigation, dashboard and workspace layouts.
// Imported after styles.css so the simplified layouts win over the legacy rules.
import './css/nav-clarity.css';
import './css/board.css';
import './css/workspace-clarity.css';
// Analysis story — verdict header, filter toolbar, split list/source layout.
import './css/analysis.css';

const rootElement = document.getElementById('root');
if (!rootElement) {
  throw new Error('Blunt Code UI could not start: #root element is missing from index.html.');
}
createRoot(rootElement).render(
  <StrictMode><App /></StrictMode>,
);

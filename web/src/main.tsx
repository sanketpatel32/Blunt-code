import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './App';
import './styles.css';
import './css/animations.css';

const rootElement = document.getElementById('root');
if (!rootElement) {
  throw new Error('Blunt Code UI could not start: #root element is missing from index.html.');
}
createRoot(rootElement).render(
  <StrictMode><App /></StrictMode>,
);

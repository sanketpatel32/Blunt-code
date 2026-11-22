import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { App } from './App';
import { SignedOut } from './components/SignedOut';
import { establishSession } from './session';
import './styles.css';

const rootElement = document.getElementById('root');
if (!rootElement) {
  throw new Error('Blunt Code UI could not start: #root element is missing from index.html.');
}
const root = createRoot(rootElement);
void establishSession().then((state) => {
  root.render(<StrictMode>{state === 'signed-out' ? <SignedOut /> : <App />}</StrictMode>);
});

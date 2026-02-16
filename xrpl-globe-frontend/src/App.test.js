import { render, screen } from '@testing-library/react';
import App from './App';

jest.mock('./components/XRPLGlobe', () => () => <div>Mock XRPLGlobe</div>);

test('renders the app without crashing', () => {
  render(<App />);
  const globeElement = screen.getByText(/Mock XRPLGlobe/i);
  expect(globeElement).toBeInTheDocument();
});

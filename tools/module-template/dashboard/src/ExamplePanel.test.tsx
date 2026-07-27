import { render, screen } from '@testing-library/react';
import { SubscribeProvider, type Subscribe } from './bridge';
import { ExamplePanel } from './ExamplePanel';
import { ExampleStats } from './proto/example_pb';
import { Timestamp } from '@bufbuild/protobuf';

function renderWithStats(stats: ExampleStats | null) {
  const subscribe: Subscribe = (_subject, onBytes) => {
    if (stats) onBytes(stats.toBinary());
    return () => {};
  };
  return render(
    <SubscribeProvider value={subscribe}>
      <ExamplePanel roverId="rover-1" />
    </SubscribeProvider>,
  );
}

test('shows N/A before any frame arrives', () => {
  renderWithStats(null);
  expect(screen.getByText(/waiting for first frame/i)).toBeInTheDocument();
});

test('renders count and message from a decoded frame', () => {
  const stats = new ExampleStats({
    t: Timestamp.fromDate(new Date(0)),
    count: 7n,
    message: 'hello',
  });
  renderWithStats(stats);
  expect(screen.getByText('7')).toBeInTheDocument();
  expect(screen.getByText('hello')).toBeInTheDocument();
});

import { createRoot } from 'react-dom/client';
import { SubscribeProvider, type Subscribe } from './bridge';
import { ExamplePanel } from './ExamplePanel';
import './ui/tokens.css';

// ModuleContext is what the dashboard host passes to mount(). It is the only
// contract between a module panel and the host.
type ModuleContext = {
  roverId: string;
  subscribe: Subscribe;
  session?: { role: string };
  tokens?: unknown;
};

export default {
  mount(container: HTMLElement, ctx: ModuleContext): () => void {
    const root = createRoot(container);
    root.render(
      <SubscribeProvider value={ctx.subscribe}>
        <ExamplePanel roverId={ctx.roverId} />
      </SubscribeProvider>,
    );
    return () => root.unmount();
  },
};

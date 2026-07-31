import { createRoot } from 'react-dom/client';
import { TeleopMap } from './TeleopMap';
import type { ModuleContext } from './types';
import './survey.css';

export default {
  mount(container: HTMLElement, ctx: ModuleContext): () => void {
    const root = createRoot(container);
    root.render(<TeleopMap ctx={ctx} />);
    return () => root.unmount();
  },
};

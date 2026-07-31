import { createRoot } from 'react-dom/client';
import { SurveyPanel } from './SurveyPanel';
import type { ModuleContext } from './types';
import './survey.css';

export default {
  mount(container: HTMLElement, ctx: ModuleContext): () => void {
    const root = createRoot(container);
    root.render(<SurveyPanel ctx={ctx} />);
    return () => root.unmount();
  },
};

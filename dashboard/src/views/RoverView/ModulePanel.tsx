import { useEffect, useRef, useState } from 'react';
import { tokens } from '@waypoint/ui-tokens';
import { getBus } from '../../state/nats';
import { useMe } from '@/state/mode';
import { moduleStaticUrl, waypointImport } from '@/lib/moduleImport';

type ModuleContext = {
  roverId: string;
  nats?: unknown;
  session?: { role: string };
  tokens: typeof tokens;
  subscribe: (subject: string, onBytes: (b: Uint8Array) => void) => () => void;
  publish: (subject: string, bytes: Uint8Array) => void;
};

type ModulePanelBundle = {
  default: {
    mount(container: HTMLElement, ctx: ModuleContext): () => void;
  };
};


export type ModulePanelProps = {
  moduleId: string;
  roverId: string;
  session?: { role: string };
};

export function ModulePanel({ moduleId, roverId, session }: ModulePanelProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [error, setError] = useState<string | null>(null);
  const me = useMe();

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    let cleanup: (() => void) | undefined;
    let cancelled = false;
    setError(null);
    moduleStaticUrl(me?.mode, roverId, moduleId, 'panel.js')
      .then(url => waypointImport<ModulePanelBundle>(url))
      .then(mod => {
        if (cancelled) return;
        cleanup = mod.default.mount(el, {
          roverId,
          session,
          tokens,
          subscribe: (subject, onBytes) => getBus().subscribe(subject, onBytes),
          publish: (subject, bytes) => {
            const prefix = `waypoint.${roverId}.module.${moduleId}.`;
            if (!subject.startsWith(prefix)) {
              throw new Error(`module ${moduleId} may not publish to ${subject}`);
            }
            getBus().publish(subject, bytes);
          },
        });
      })
      .catch(err => {
        if (cancelled) return;
        setError(String(err?.message ?? err));
      });
    return () => {
      cancelled = true;
      if (cleanup) cleanup();
    };
  }, [moduleId, roverId, session, me?.mode]);

  if (error !== null) {
    return <p data-module-error={moduleId}>{`Failed to load module: ${error}`}</p>;
  }
  return <div ref={containerRef} data-module-panel={moduleId} />;
}

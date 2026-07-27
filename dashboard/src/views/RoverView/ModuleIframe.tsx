export type ModuleIframeProps = {
  moduleId: string;
};

export function ModuleIframe({ moduleId }: ModuleIframeProps) {
  return (
    <iframe
      src={`/module/${moduleId}/`}
      sandbox="allow-scripts allow-forms allow-same-origin"
      loading="lazy"
      style={{
        width: '100%',
        height: '100%',
        border: 'none',
        background: 'var(--color-surface)',
      }}
      title={`Module ${moduleId}`}
    />
  );
}

// dashboard/src/ui/data/CodeViewer.tsx
//
// Polished mission-console code/file viewer. Title bar uses the project's
// `[ NAME ]` bracket chrome; icon-only actions match the existing Button
// hover language. Body scrolls horizontally on long lines. Pass body={false}
// to render header-only (used for compact retrieval rows).
import React from 'react';
import { lexToml, type TomlToken } from '@/lib/highlight/toml';
import styles from './CodeViewer.module.css';

export type Lang = 'toml';
type Tone = 'ok' | 'caution' | 'fault' | 'muted';

type Actions = {
  onCopy?: (content: string) => void;
  onDownload?: (filename: string, content: string) => void;
};

type Props = {
  filename: string;
  content: string;
  lang: Lang;
  meta?: string;
  status?: { label: string; tone: Tone };
  actions?: Actions;
  body?: boolean;
  footerNote?: React.ReactNode;
};

export function CodeViewer({
  filename,
  content,
  lang,
  meta,
  status,
  actions,
  body = true,
  footerNote,
}: Props) {
  const lines = body ? lexLines(content, lang) : null;

  return (
    <div className={styles.viewer}>
      <div className={styles.head}>
        <span className={styles.title} role="heading" aria-level={3}>
          <span className={styles.bk}>[</span>
          {filename.toUpperCase()}
          <span className={styles.bk}>]</span>
        </span>
        {meta ? <span className={styles.meta}>{meta}</span> : null}
        {status ? (
          <span className={`${styles.status} ${styles['tone-' + status.tone]}`}>
            {status.label}
          </span>
        ) : null}

        {(actions?.onCopy || actions?.onDownload) && (
          <div className={styles.actions}>
            {actions?.onCopy ? (
              <button
                type="button"
                className={styles.ico}
                aria-label="Copy"
                onClick={() => actions.onCopy!(content)}
              >
                <CopyIcon />
                <span className={styles.tip}>Copy</span>
              </button>
            ) : null}
            {actions?.onDownload ? (
              <button
                type="button"
                className={styles.ico}
                aria-label="Download"
                onClick={() => actions.onDownload!(filename, content)}
              >
                <DownloadIcon />
                <span className={styles.tip}>Download</span>
              </button>
            ) : null}
          </div>
        )}
      </div>

      {body && lines ? (
        <div className={styles.body} data-testid="code-viewer-body">
          {lines.map((tokens, i) => (
            <div
              key={`${i}-${tokens[0]?.text ?? ''}`}
              className={`${styles.line} ${tokens.length === 0 ? styles.lineBlank : ''}`}
            >
              <span className={styles.lineNum}>{tokens.length === 0 ? '' : i + 1}</span>
              <span className={styles.lineSrc}>
                {tokens.map((t, j) => (
                  <span key={j} className={styles['tk-' + t.kind]}>
                    {t.text}
                  </span>
                ))}
              </span>
            </div>
          ))}
        </div>
      ) : null}

      {footerNote ? <div className={styles.foot}>{footerNote}</div> : null}
    </div>
  );
}

function lexLines(content: string, lang: Lang): TomlToken[][] {
  switch (lang) {
    case 'toml': return lexToml(content);
    default: {
      const exhaustive: never = lang;
      throw new Error(`unsupported lang: ${exhaustive as string}`);
    }
  }
}

function CopyIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <rect x="9" y="9" width="13" height="13" />
      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
    </svg>
  );
}

function DownloadIcon() {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.75" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
      <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
      <polyline points="7 10 12 15 17 10" />
      <line x1="12" y1="15" x2="12" y2="3" />
    </svg>
  );
}

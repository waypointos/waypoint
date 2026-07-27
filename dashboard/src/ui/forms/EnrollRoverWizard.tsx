// dashboard/src/ui/forms/EnrollRoverWizard.tsx
//
// Four-step wizard for enrolling a new rover. Centered modal with a labeled
// sidebar stepper (macOS-installer style).
// State is a discriminated union; steps advance forward via Continue and
// backward via Back. The bus prop is injected so tests can stub NATS.
//
// Cameras are NOT configured here: the agent auto-discovers capture devices
// at first boot (a fresh rover has no /dev/video* paths to type), so the
// wizard never asks. Modules are likewise not configured here: they are
// installed at runtime from the dashboard once the rover is online (no-rebuild
// module install), so enrollment only emits identity.toml.
import React, { useEffect, useMemo, useReducer, useRef } from 'react';
import { WizardModal, type WizardStep } from '@/ui/primitives/WizardModal';
import { Button } from '@/ui/primitives/Button';
import { getBus } from '@/state/nats';
import { CodeViewer } from '@/ui/data/CodeViewer';
import styles from './EnrollRoverWizard.module.css';

type Bus = {
  subscribe: (subject: string, handler: (data: Uint8Array) => void) => () => void;
};

type Props = {
  open: boolean;
  onClose: () => void;
  onEnrolled?: () => void;
  bus?: Bus;
};

type State =
  | { step: 'identify'; id: string; name: string; idErr?: string; serverErr?: string }
  | { step: 'submitting'; id: string; name: string }
  | { step: 'generate'; id: string; name: string; toml: string }
  | { step: 'flash-drop'; id: string; name: string; toml: string }
  | { step: 'verify-waiting'; id: string; name: string; toml: string }
  | { step: 'verify-online'; id: string; name: string; toml: string; firstSeenAt: Date };

type Action =
  | { type: 'set-id'; value: string }
  | { type: 'set-name'; value: string }
  | { type: 'set-id-err'; msg: string | undefined }
  | { type: 'submit-start' }
  | { type: 'submit-err'; msg: string }
  | { type: 'submit-ok'; toml: string }
  | { type: 'go-flash' }
  | { type: 'go-verify' }
  | { type: 'go-back' }
  | { type: 'heartbeat' }
  | { type: 'reset' };

const ID_RE = /^[a-z0-9][a-z0-9-]{2,31}$/;

function init(): State {
  return { step: 'identify', id: '', name: '' };
}

function reducer(s: State, a: Action): State {
  switch (a.type) {
    case 'reset':
      return init();
    case 'set-id':
      return s.step === 'identify' ? { ...s, id: a.value, serverErr: undefined } : s;
    case 'set-name':
      return s.step === 'identify' ? { ...s, name: a.value, serverErr: undefined } : s;
    case 'set-id-err':
      return s.step === 'identify' ? { ...s, idErr: a.msg } : s;
    case 'submit-start':
      return s.step === 'identify' ? { step: 'submitting', id: s.id, name: s.name } : s;
    case 'submit-err':
      return s.step === 'submitting'
        ? { step: 'identify', id: s.id, name: s.name, serverErr: a.msg }
        : s;
    case 'submit-ok':
      return s.step === 'submitting'
        ? { step: 'generate', id: s.id, name: s.name, toml: a.toml }
        : s;
    case 'go-flash':
      return s.step === 'generate'
        ? { step: 'flash-drop', id: s.id, name: s.name, toml: s.toml }
        : s;
    case 'go-verify':
      return s.step === 'flash-drop'
        ? { step: 'verify-waiting', id: s.id, name: s.name, toml: s.toml }
        : s;
    case 'go-back': {
      if (s.step === 'generate')        return { step: 'identify', id: s.id, name: s.name };
      if (s.step === 'flash-drop')      return { step: 'generate', id: s.id, name: s.name, toml: s.toml };
      if (s.step === 'verify-waiting')  return { step: 'flash-drop', id: s.id, name: s.name, toml: s.toml };
      if (s.step === 'verify-online')   return { step: 'flash-drop', id: s.id, name: s.name, toml: s.toml };
      return s;
    }
    case 'heartbeat':
      return s.step === 'verify-waiting'
        ? { ...s, step: 'verify-online', firstSeenAt: new Date() }
        : s;
    default:
      return s;
  }
}

const STEP_INDEX: Record<State['step'], number> = {
  'identify':        0,
  'submitting':      0,
  'generate':        1,
  'flash-drop':      2,
  'verify-waiting':  3,
  'verify-online':   3,
};

function buildSteps(stepIndex: number): WizardStep[] {
  const items: Array<{ number: string; label: string }> = [
    { number: '01', label: 'IDENTIFY' },
    { number: '02', label: 'GENERATE' },
    { number: '03', label: 'FLASH & DROP' },
    { number: '04', label: 'BOOT & VERIFY' },
  ];
  return items.map((item, i) => ({
    ...item,
    state: i < stepIndex ? 'done' : i === stepIndex ? 'current' : 'upcoming',
  }));
}

export function EnrollRoverWizard({ open, onClose, onEnrolled, bus }: Props) {
  const resolvedBus = bus ?? getBus();
  const [state, dispatch] = useReducer(reducer, undefined, init);
  const prevOpen = useRef(open);
  useEffect(() => {
    if (open && !prevOpen.current) {
      dispatch({ type: 'reset' });
    }
    prevOpen.current = open;
  }, [open]);
  const cur = STEP_INDEX[state.step];

  async function submit() {
    if (state.step !== 'identify') return;
    if (!ID_RE.test(state.id) || state.name.trim() === '') return;
    dispatch({ type: 'submit-start' });
    try {
      const r = await fetch('/api/rovers/enroll', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: state.id, name: state.name.trim() }),
      });
      if (r.ok) {
        const data = await r.json();
        dispatch({ type: 'submit-ok', toml: data.identityToml ?? '' });
        onEnrolled?.();
      } else {
        const msg = await r.text();
        dispatch({ type: 'submit-err', msg });
      }
    } catch (e) {
      dispatch({ type: 'submit-err', msg: e instanceof Error ? e.message : 'network error' });
    }
  }

  return (
    <WizardModal
      open={open}
      onClose={onClose}
      aria-label="Enroll rover"
      title="Enroll rover"
      steps={buildSteps(cur)}
      footer={wizardFooter(state, dispatch, onClose, submit)}
    >
      {(state.step === 'identify' || state.step === 'submitting') ? (
        <Identify state={state} dispatch={dispatch} />
      ) : null}
      {state.step === 'generate' ? <Generate state={state} /> : null}
      {state.step === 'flash-drop' ? <FlashDrop state={state} /> : null}
      {(state.step === 'verify-waiting' || state.step === 'verify-online') ? (
        <Verify state={state} bus={resolvedBus} dispatch={dispatch} />
      ) : null}
    </WizardModal>
  );
}

function Identify({
  state,
  dispatch,
}: {
  state: Extract<State, { step: 'identify' | 'submitting' }>;
  dispatch: React.Dispatch<Action>;
}) {
  const idValue = state.id;
  const nameValue = state.name;
  const idErr = state.step === 'identify' ? state.idErr : undefined;
  const serverErr = state.step === 'identify' ? state.serverErr : undefined;

  return (
    <>
      <div className={styles.head}>
        <div className={styles.eyebrow}>01 IDENTIFY</div>
        <div className={styles.title}>Name your rover</div>
        <div className={styles.subtitle}>
          Used as the persistent ID across the proxy and NATS.
        </div>
      </div>
      <div className={styles.pane}>
        <label className={styles.field}>
          <span className={styles.label}>Rover ID</span>
          <input
            className={styles.input}
            type="text"
            value={idValue}
            placeholder="sim-02"
            autoFocus
            aria-label="Rover ID"
            onChange={(e) => dispatch({ type: 'set-id', value: e.target.value })}
            onBlur={() =>
              dispatch({
                type: 'set-id-err',
                msg:
                  idValue === '' || ID_RE.test(idValue)
                    ? undefined
                    : 'Must be lowercase alphanumeric, 3–32 chars, kebab-style.',
              })
            }
          />
          {idErr ? <div className={styles.fieldErr}>{idErr}</div> : null}
        </label>
        <label className={styles.field}>
          <span className={styles.label}>Name</span>
          <input
            className={styles.input}
            type="text"
            value={nameValue}
            placeholder="Recon"
            aria-label="Name"
            onChange={(e) => dispatch({ type: 'set-name', value: e.target.value })}
          />
        </label>
        {serverErr ? <div className={styles.err}>{serverErr}</div> : null}
      </div>
    </>
  );
}

function Generate({ state }: { state: Extract<State, { step: 'generate' }> }) {
  const { lineCount, byteCount } = tomlStats(state.toml);
  return (
    <>
      <div className={styles.head}>
        <div className={styles.eyebrow}>02 GENERATE</div>
        <div className={styles.title}>Identity ready</div>
        <div className={styles.subtitle}>
          This file identifies your rover to the proxy. You'll drop it on the SD
          card next. Don't share the <code className={styles.inlineCode}>[nats]</code> section — it's a private
          credential.
        </div>
      </div>
      <div className={styles.pane}>
        <CodeViewer
          filename="identity.toml"
          content={state.toml}
          lang="toml"
          meta={`${lineCount} LINES · ${byteCount} B`}
          actions={{
            onCopy: (text) => {
              navigator.clipboard.writeText(text).catch((err) => {
                console.error('clipboard write failed', err);
              });
            },
            onDownload: (name, text) => downloadBlob(name, text),
          }}
        />
      </div>
    </>
  );
}

function FlashDrop({ state }: { state: Extract<State, { step: 'flash-drop' }> }) {
  const identityToml = state.toml;
  const identityStats = tomlStats(identityToml);
  return (
    <>
      <div className={styles.head}>
        <div className={styles.eyebrow}>03 FLASH & DROP</div>
        <div className={styles.title}>Prepare the SD card</div>
        <div className={styles.subtitle}>Two physical steps before powering the rover.</div>
      </div>
      <div className={styles.pane}>
        <section className={styles.sectionBody}>
          <div className={styles.sectionHead}>
            <span className={styles.bk}>[</span>A · FLASH THE IMAGE<span className={styles.bk}>]</span>
          </div>
          <p>
            Burn <span className={styles.filePath}>waypoint-prod-&lt;version&gt;.img</span> to a microSD (16 GB minimum).
          </p>
          <div className={styles.linkRow}>
            →&nbsp;
            <a href="https://www.raspberrypi.com/software/" target="_blank" rel="noopener noreferrer">
              Raspberry Pi Imager
            </a>
          </div>
          <div className={styles.linkRow}>
            →&nbsp;
            <a href="https://github.com/waypointos/waypoint/releases" target="_blank" rel="noopener noreferrer">
              Latest release on GitHub
            </a>
          </div>
        </section>

        <section className={styles.sectionBody}>
          <div className={styles.sectionHead}>
            <span className={styles.bk}>[</span>B · DROP IDENTITY.TOML<span className={styles.bk}>]</span>
          </div>
          <p>
            Insert the SD card into your computer and open the FAT32 boot partition.
            Save this <span className={styles.filePath}>identity.toml</span> at the
            root of the partition, next to <span className={styles.filePath}>config.txt</span>.
          </p>
          <CodeViewer
            filename="identity.toml"
            content={identityToml}
            lang="toml"
            meta={`${identityStats.lineCount}L · ${identityStats.byteCount} B`}
            body={false}
            actions={{
              onCopy: (text) => {
                navigator.clipboard.writeText(text).catch((err) => {
                  console.error('clipboard write failed', err);
                });
              },
              onDownload: (name, text) => downloadBlob(name, text),
            }}
          />
        </section>
      </div>
    </>
  );
}

function Verify({
  state,
  bus,
  dispatch,
}: {
  state: Extract<State, { step: 'verify-waiting' | 'verify-online' }>;
  bus: Bus;
  dispatch: React.Dispatch<Action>;
}) {
  const subject = useMemo(
    () => `waypoint.${state.id}.infra.heartbeat`,
    [state.id],
  );

  useEffect(() => {
    if (state.step !== 'verify-waiting') return;
    const unsubscribe = bus.subscribe(subject, () => {
      dispatch({ type: 'heartbeat' });
    });
    return unsubscribe;
    // state.step is intentionally omitted: the guard already enforces single-subscription;
    // including it would unsubscribe-then-re-evaluate on every heartbeat-triggered re-render.
  }, [bus, subject, dispatch]);

  const online = state.step === 'verify-online';

  return (
    <>
      <div className={styles.head}>
        <div className={styles.eyebrow}>04 BOOT & VERIFY</div>
        <div className={styles.title}>Power on {state.id}</div>
        <div className={styles.subtitle}>
          Connect power and an ethernet cable. The rover should appear here within
          ~20 seconds of boot.
        </div>
      </div>
      <div className={styles.pane}>
        <div className={`${styles.statusLine} ${online ? styles.online : styles.waiting}`}>
          <span className={styles.statusDot} />
          {online ? 'ONLINE · last seen now' : 'WAITING FOR FIRST HEARTBEAT'}
        </div>
        <div className={styles.subjectLine}>
          {state.step === 'verify-online'
            ? `${state.id} · first heartbeat at ${state.firstSeenAt.toLocaleTimeString()}`
            : `${state.id} · ${subject}`}
        </div>
      </div>
    </>
  );
}

function tomlStats(toml: string): { lineCount: number; byteCount: number } {
  return {
    lineCount: toml.split('\n').length,
    byteCount: new Blob([toml]).size,
  };
}

function wizardFooter(
  state: State,
  dispatch: React.Dispatch<Action>,
  onClose: () => void,
  submit: () => void,
): React.ReactNode {
  if (state.step === 'identify' || state.step === 'submitting') {
    return (
      <div className={styles.actions}>
        <Button onClick={onClose}>Cancel</Button>
        <Button
          variant="primary"
          disabled={
            state.step === 'submitting' ||
            !ID_RE.test(state.id) ||
            state.name.trim() === ''
          }
          onClick={submit}
        >
          {state.step === 'submitting' ? 'Generating…' : 'Generate identity →'}
        </Button>
      </div>
    );
  }
  if (state.step === 'generate') {
    return (
      <div className={styles.actions}>
        <Button onClick={() => dispatch({ type: 'go-back' })}>Back</Button>
        <Button variant="primary" onClick={() => dispatch({ type: 'go-flash' })}>
          Continue →
        </Button>
      </div>
    );
  }
  if (state.step === 'flash-drop') {
    return (
      <div className={styles.actions}>
        <Button onClick={() => dispatch({ type: 'go-back' })}>Back</Button>
        <Button variant="primary" onClick={() => dispatch({ type: 'go-verify' })}>
          I've done this →
        </Button>
      </div>
    );
  }
  if (state.step === 'verify-waiting' || state.step === 'verify-online') {
    return (
      <div className={styles.actions}>
        <button type="button" className={styles.checkLater} onClick={onClose}>
          I'll check later
        </button>
        <Button
          variant="primary"
          disabled={state.step !== 'verify-online'}
          onClick={onClose}
        >
          Done
        </Button>
      </div>
    );
  }
  return null;
}

function downloadBlob(filename: string, content: string) {
  const blob = new Blob([content], { type: 'text/plain;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

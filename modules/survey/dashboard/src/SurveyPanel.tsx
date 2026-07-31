import { useEffect, useMemo, useState, type ReactNode } from 'react';
import { MissionMap } from './MissionMap';
import { parseTargets, toLocal, type LocalWaypoint } from './geo';
import { tokenVars } from './tokens';
import { useSurvey } from './useSurvey';
import type { FileEntry, MissionDoc, ModuleContext } from './types';

export function SurveyPanel({ ctx }: { ctx: ModuleContext }) {
  const survey = useSurvey(ctx);
  return (
    <div className="sv-root" style={tokenVars(ctx.tokens)} data-testid="panel-m-survey">
      <StatusStrip doc={survey.doc} />
      <section className="sv-section">
        <h3 className="sv-title">[ MAP ]</h3>
        <MissionMap doc={survey.doc} trail={survey.trail} />
      </section>
      <WaypointsSection survey={survey} />
      <LogsSection survey={survey} />
      <SnapsSection survey={survey} />
    </div>
  );
}

function stateClass(state: string): string {
  switch (state) {
    case 'TRANSIT':
    case 'RETURN':
      return 'sv-val-accent';
    case 'SCAN':
    case 'SENSE':
      return 'sv-val-caution';
    case 'DONE':
      return 'sv-val-ok';
    default:
      return '';
  }
}

export function StatusStrip({ doc }: { doc: MissionDoc | null }) {
  const det = doc?.last_detection ?? null;
  return (
    <div className="sv-strip">
      <Tile label="State">
        {doc ? <span className={stateClass(doc.state)}>{doc.state}</span> : <NA hint="module offline" />}
      </Tile>
      <Tile label="Mode">{doc ? doc.mode : <NA hint="module offline" />}</Tile>
      <Tile label="Leg">
        {doc && doc.waypoints.length > 0 ? (
          `${Math.min(doc.leg + 1, totalLegs(doc))} / ${totalLegs(doc)}`
        ) : (
          <NA hint="no waypoints" />
        )}
      </Tile>
      <Tile label="Source">{doc ? doc.active_source : <NA hint="module offline" />}</Tile>
      <Tile label="Last detection">
        {det ? `wp ${det.wp + 1} · id ${det.id}` : <NA hint="none yet" />}
      </Tile>
    </div>
  );
}

function totalLegs(doc: MissionDoc): number {
  // Planned polyline has legs = points - 1; matches the engine's leg list.
  return Math.max(doc.planned.length - 1, doc.waypoints.length);
}

function Tile({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="sv-tile">
      <span className="sv-tile-label">{label}</span>
      <span className="sv-tile-value">{children}</span>
    </div>
  );
}

function NA({ hint }: { hint: string }) {
  return <span className="sv-na">N/A: {hint}</span>;
}

type SurveyState = ReturnType<typeof useSurvey>;

function WaypointsSection({ survey }: { survey: SurveyState }) {
  const { doc, requester } = survey;
  const [raw, setRaw] = useState('');
  const [tagIds, setTagIds] = useState('');
  const [startPose, setStartPose] = useState('0,0,0');
  const [result, setResult] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const parsed = useMemo<{ wps: LocalWaypoint[]; error: string | null }>(() => {
    if (!raw.trim()) return { wps: [], error: null };
    try {
      return { wps: toLocal(parseTargets(raw)), error: null };
    } catch (e) {
      return { wps: [], error: e instanceof Error ? e.message : String(e) };
    }
  }, [raw]);

  const tags = useMemo<{ ids: number[]; error: string | null }>(() => {
    const t = tagIds.trim();
    if (!t) return { ids: [], error: null };
    const ids = t.split(',').map((s) => Number(s.trim()));
    if (ids.some((n) => !Number.isInteger(n))) return { ids: [], error: 'tag ids must be integers' };
    if (ids.length !== parsed.wps.length) {
      return { ids: [], error: `${ids.length} tag ids for ${parsed.wps.length} waypoints` };
    }
    return { ids, error: null };
  }, [tagIds, parsed.wps.length]);

  const start = useMemo<{ pose: number[] | null; error: string | null }>(() => {
    const t = startPose.trim();
    if (!t) return { pose: null, error: null };
    const parts = t.split(',').map((s) => Number(s.trim()));
    if (parts.length !== 3 || parts.some((n) => !Number.isFinite(n))) {
      return { pose: null, error: 'start pose must be x,y,theta_deg' };
    }
    return { pose: parts, error: null };
  }, [startPose]);

  const idle = doc !== null && (doc.state === 'IDLE' || doc.state === 'DONE');
  const applyDisabled =
    busy || !idle || parsed.wps.length === 0 || parsed.error !== null || tags.error !== null || start.error !== null;

  const apply = () => {
    setBusy(true);
    setResult(null);
    requester
      .request('waypoints.set', {
        waypoints: parsed.wps.map((w) => [round2(w.x), round2(w.y)]),
        ...(tags.ids.length > 0 ? { tag_ids: tags.ids } : {}),
        ...(start.pose ? { start_pose: start.pose } : {}),
      })
      .then((r) => setResult(`Applied ${r.count} waypoints (override active)`))
      .catch((e) => setResult(`Error: ${e.message}`))
      .finally(() => setBusy(false));
  };

  const clear = () => {
    setBusy(true);
    setResult(null);
    requester
      .request('waypoints.clear')
      .then(() => setResult('Override cleared, config waypoints active'))
      .catch((e) => setResult(`Error: ${e.message}`))
      .finally(() => setBusy(false));
  };

  return (
    <section className="sv-section">
      <h3 className="sv-title">
        [ WAYPOINTS ]
        <span className={doc?.active_source === 'override' ? 'sv-source sv-source-override' : 'sv-source'}>
          {doc ? `source: ${doc.active_source}` : 'source: N/A'}
        </span>
      </h3>
      <p className="sv-help">
        Paste the committee list, one waypoint per line: seq, lat, lon[, alt]. Degree signs and
        parentheses are fine. Converted to local meters (x=East, y=North, origin = lowest seq).
      </p>
      <textarea
        className="sv-textarea"
        rows={5}
        placeholder={'1, 39.9017797°, 32.7704813°, 890\n2, 39.9017482°, 32.7704942°, 890'}
        value={raw}
        onChange={(e) => setRaw(e.target.value)}
        aria-label="Waypoint list"
      />
      {parsed.error && <p className="sv-error">{parsed.error}</p>}
      {parsed.wps.length > 0 && (
        <table className="sv-table">
          <thead>
            <tr>
              <th>seq</th>
              <th className="sv-num">x east (m)</th>
              <th className="sv-num">y north (m)</th>
              <th className="sv-num">leg (m)</th>
            </tr>
          </thead>
          <tbody>
            {parsed.wps.map((w) => (
              <tr key={w.seq}>
                <td>{w.seq}</td>
                <td className="sv-num">{w.x.toFixed(2)}</td>
                <td className="sv-num">{w.y.toFixed(2)}</td>
                <td className="sv-num">{w.leg.toFixed(2)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <div className="sv-formrow">
        <label className="sv-field">
          <span className="sv-tile-label">Tag IDs (optional)</span>
          <input
            className="sv-input"
            placeholder="11,40,17"
            value={tagIds}
            onChange={(e) => setTagIds(e.target.value)}
          />
        </label>
        <label className="sv-field">
          <span className="sv-tile-label">Start pose x,y,theta_deg</span>
          <input className="sv-input" value={startPose} onChange={(e) => setStartPose(e.target.value)} />
        </label>
      </div>
      {tags.error && <p className="sv-error">{tags.error}</p>}
      {start.error && <p className="sv-error">{start.error}</p>}
      <div className="sv-actions">
        <button type="button" className="sv-btn sv-btn-primary" disabled={applyDisabled} onClick={apply}>
          Apply waypoints
        </button>
        <button type="button" className="sv-btn" disabled={busy || !idle} onClick={clear}>
          Clear override
        </button>
        {!idle && doc && <span className="sv-na">mission {doc.state}: apply only while IDLE or DONE</span>}
        {!doc && <span className="sv-na">N/A: module offline</span>}
      </div>
      {result && <p className={result.startsWith('Error') ? 'sv-error' : 'sv-result'}>{result}</p>}
    </section>
  );
}

function round2(n: number): number {
  return Math.round(n * 100) / 100;
}

function downloadB64(name: string, b64: string, mime: string) {
  const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
  const url = URL.createObjectURL(new Blob([bytes], { type: mime }));
  const a = document.createElement('a');
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}

function LogsSection({ survey }: { survey: SurveyState }) {
  const { requester } = survey;
  const [files, setFiles] = useState<FileEntry[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  const refresh = () => {
    requester
      .request('logs.list')
      .then((r) => {
        setFiles((r.files as FileEntry[]) ?? []);
        setError(null);
      })
      .catch((e) => setError(e.message));
  };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(refresh, []);

  const download = (name: string) => {
    requester
      .request('logs.get', { name })
      .then((r) => downloadB64(name, r.b64 as string, 'text/csv'))
      .catch((e) => setError(e.message));
  };

  return (
    <section className="sv-section">
      <h3 className="sv-title">
        [ LOGS ]
        <button type="button" className="sv-btn sv-btn-small" onClick={refresh}>
          Refresh
        </button>
      </h3>
      {error && <p className="sv-error">{error}</p>}
      {files === null && !error && <p className="sv-na">N/A: waiting for module</p>}
      {files !== null && files.length === 0 && <p className="sv-na">N/A: no mission logs yet</p>}
      {files !== null && files.length > 0 && (
        <ul className="sv-files">
          {files.map((f) => (
            <li key={f.name} className="sv-file">
              <span className="sv-file-name">{f.name}</span>
              <span className="sv-file-size">{formatSize(f.size)}</span>
              <button type="button" className="sv-btn sv-btn-small" onClick={() => download(f.name)}>
                Download
              </button>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function SnapsSection({ survey }: { survey: SurveyState }) {
  const { requester } = survey;
  const [files, setFiles] = useState<FileEntry[] | null>(null);
  const [thumbs, setThumbs] = useState<Record<string, string>>({});
  const [error, setError] = useState<string | null>(null);

  const refresh = () => {
    requester
      .request('snaps.list')
      .then((r) => {
        const list = (r.files as FileEntry[]) ?? [];
        setFiles(list);
        setError(null);
        for (const f of list) {
          requester
            .request('snaps.get', { name: f.name })
            .then((g) =>
              setThumbs((t) => ({ ...t, [f.name]: `data:image/jpeg;base64,${g.b64 as string}` })),
            )
            .catch(() => {});
        }
      })
      .catch((e) => setError(e.message));
  };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(refresh, []);

  const download = (name: string) => {
    requester
      .request('snaps.get', { name })
      .then((r) => downloadB64(name, r.b64 as string, 'image/jpeg'))
      .catch((e) => setError(e.message));
  };

  return (
    <section className="sv-section">
      <h3 className="sv-title">
        [ SNAPSHOTS ]
        <button type="button" className="sv-btn sv-btn-small" onClick={refresh}>
          Refresh
        </button>
      </h3>
      {error && <p className="sv-error">{error}</p>}
      {files === null && !error && <p className="sv-na">N/A: waiting for module</p>}
      {files !== null && files.length === 0 && (
        <p className="sv-na">N/A: no detection snapshots yet (none in sim mode)</p>
      )}
      {files !== null && files.length > 0 && (
        <div className="sv-snaps">
          {files.map((f) => (
            <button
              key={f.name}
              type="button"
              className="sv-snap"
              onClick={() => download(f.name)}
              title={`Download ${f.name}`}
            >
              {thumbs[f.name] ? (
                <img className="sv-snap-img" src={thumbs[f.name]} alt={f.name} />
              ) : (
                <span className="sv-snap-loading">…</span>
              )}
              <span className="sv-snap-caption">
                {f.wp !== undefined ? `wp ${f.wp + 1} · id ${f.id}` : f.name}
              </span>
            </button>
          ))}
        </div>
      )}
    </section>
  );
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

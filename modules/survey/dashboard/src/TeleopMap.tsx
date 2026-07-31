import { MissionMap } from './MissionMap';
import { tokenVars } from './tokens';
import { useSurvey } from './useSurvey';
import type { MissionDoc, ModuleContext } from './types';

// Compact teleop window: one status line plus the map, no forms or files.
export function TeleopMap({ ctx }: { ctx: ModuleContext }) {
  const { doc, trail } = useSurvey(ctx);
  return (
    <div className="sv-root sv-teleop" style={tokenVars(ctx.tokens)} data-testid="teleop-w-survey">
      <div className="sv-teleop-status">{statusLine(doc)}</div>
      <MissionMap doc={doc} trail={trail} compact />
    </div>
  );
}

function statusLine(doc: MissionDoc | null): string {
  if (!doc) return 'N/A: no mission doc from module';
  const det = doc.last_detection;
  const parts = [doc.state, doc.mode.toLowerCase(), `leg ${doc.leg + 1}`];
  if (det) parts.push(`last: wp ${det.wp + 1} id ${det.id}`);
  return parts.join('  ·  ');
}

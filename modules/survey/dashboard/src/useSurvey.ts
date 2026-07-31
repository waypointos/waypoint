import { useEffect, useMemo, useRef, useState } from 'react';
import { createRequester, type Requester } from './reqres';
import { appendCapped, shouldAppend, type PosePoint } from './trail';
import type { MissionDoc, ModuleContext } from './types';

export type Survey = {
  doc: MissionDoc | null;
  trail: [number, number][];
  requester: Requester;
  prefix: string;
};

// useSurvey is the shared live state for the tab and the teleop window:
// mission doc subscription, trail fetch + local extension, and the ui.req
// requester. Trail refetches when the mission epoch changes (new arm or
// module restart), matching the module's trail reset.
export function useSurvey(ctx: ModuleContext): Survey {
  const prefix = `waypoint.${ctx.roverId}.module.survey.`;
  const [doc, setDoc] = useState<MissionDoc | null>(null);
  const [trail, setTrail] = useState<[number, number][]>([]);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const requester = useMemo(() => createRequester(prefix, ctx.publish, ctx.subscribe), []);
  const lastPose = useRef<PosePoint | null>(null);
  const epoch = useRef<number | null>(null);

  useEffect(() => {
    let alive = true;
    const dec = new TextDecoder();

    const fetchTrail = () => {
      requester
        .request('trail.get')
        .then((r) => {
          if (!alive) return;
          setTrail((r.points as [number, number][]) ?? []);
          lastPose.current = null;
        })
        .catch(() => {}); // module offline: keep whatever we have
    };

    const unsub = ctx.subscribe(prefix + 'mission', (bytes) => {
      let d: MissionDoc;
      try {
        d = JSON.parse(dec.decode(bytes)) as MissionDoc;
      } catch {
        return;
      }
      if (!alive) return;
      setDoc(d);
      if (epoch.current === null) {
        epoch.current = d.epoch;
      } else if (d.epoch !== epoch.current) {
        epoch.current = d.epoch;
        setTrail([]);
        fetchTrail();
        return;
      }
      const p: PosePoint = { x: d.pose.x, y: d.pose.y, theta: d.pose.theta };
      if (shouldAppend(lastPose.current, p)) {
        lastPose.current = p;
        setTrail((t) => appendCapped(t, [p.x, p.y]));
      }
    });

    fetchTrail();
    return () => {
      alive = false;
      unsub();
      requester.dispose();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  return { doc, trail, requester, prefix };
}

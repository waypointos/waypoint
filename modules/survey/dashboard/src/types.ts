// ModuleContext is what the dashboard host passes to mount(); the teleop
// window host passes the same shape minus session, so session is never used.
export type Tokens = {
  colors?: Record<string, string>;
  fonts?: Record<string, string>;
  space?: Record<string, string>;
};

export type ModuleContext = {
  roverId: string;
  tokens?: Tokens;
  subscribe: (subject: string, onBytes: (b: Uint8Array) => void) => () => void;
  publish: (subject: string, bytes: Uint8Array) => void;
};

// Mission doc published by the module on ...module.survey.mission.
// detected_id / tag_id use -1 for none/any; epoch is the mission start unix
// time (0 before the first arm) and doubles as the trail-reset signal.
export type WaypointStatus = 'pending' | 'reached' | 'detected';

export type DocWaypoint = {
  i: number;
  x: number;
  y: number;
  tag_id: number;
  status: WaypointStatus;
  detected_id: number;
};

export type MissionDoc = {
  state: string;
  mode: string;
  leg: number;
  waypoints: DocWaypoint[];
  planned: [number, number][];
  pose: { x: number; y: number; theta: number };
  active_source: 'config' | 'override';
  last_detection: { wp: number; id: number; t: number } | null;
  epoch: number;
};

export type FileEntry = { name: string; size: number; wp?: number; id?: number };

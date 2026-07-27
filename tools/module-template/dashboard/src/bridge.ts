import { createContext, useContext } from 'react';

// Subscribe is the host-provided bridge to the rover's NATS bus. It hands the
// panel raw message bytes for a subject and returns an unsubscribe function.
export type Subscribe = (subject: string, onBytes: (b: Uint8Array) => void) => () => void;

const SubscribeContext = createContext<Subscribe | null>(null);
export const SubscribeProvider = SubscribeContext.Provider;

export function useSubscribe(): Subscribe {
  const s = useContext(SubscribeContext);
  if (!s) throw new Error('useSubscribe must be used inside a mounted module');
  return s;
}

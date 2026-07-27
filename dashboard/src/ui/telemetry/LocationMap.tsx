// Reusable MapLibre map for rover location. Used by the Overview LOCATION card
// (2D), the 3D dialog, and the full Map view. Rovers with no fix fall back to
// Paris.
import { useEffect, useRef } from 'react';
import maplibregl from 'maplibre-gl';
import 'maplibre-gl/dist/maplibre-gl.css';
import { basemapStyle } from '@/lib/map/basemapStyle';
import { PARIS } from '@/lib/map/tiles';
import styles from './LocationMap.module.css';

export type LngLat = { lng: number; lat: number };
export type RoverMarker = { id: string; nick?: string; position: LngLat | null };

type Props = {
  /** 3D tilt + extruded buildings. Fixed for the lifetime of the instance. */
  pitched?: boolean;
  /** Rovers to plot. A null position is shown at Paris. */
  markers?: RoverMarker[];
  /** Fly to this rover when it changes. */
  focusId?: string | null;
  /** Show MapLibre's zoom/compass control. Off for the compact card. */
  controls?: boolean;
  className?: string;
};

const resolve = (m: RoverMarker): LngLat => m.position ?? PARIS;

export function LocationMap({ pitched = false, markers = [], focusId = null, controls = true, className }: Props) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mapRef = useRef<maplibregl.Map | null>(null);
  const markerRef = useRef<maplibregl.Marker[]>([]);
  const didFit = useRef(false);

  // Create the map once.
  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;

    const map = new maplibregl.Map({
      container: el,
      style: basemapStyle({ pitched }),
      center: [PARIS.lng, PARIS.lat],
      zoom: pitched ? 15 : 12,
      pitch: pitched ? 45 : 0,
      bearing: pitched ? -18 : 0,
      attributionControl: { compact: true },
    });
    if (controls) {
      // bottom-left keeps clear of the modal close (top-right), the Map view's
      // rover card (top-left), and the attribution (bottom-right).
      map.addControl(new maplibregl.NavigationControl({ showCompass: pitched, visualizePitch: pitched }), 'bottom-left');
    }
    if (!pitched) {
      map.dragRotate.disable();
      map.touchZoomRotate.disableRotation();
    }
    mapRef.current = map;

    // Keep the canvas sized to its container (modal/card resizes).
    const ro = new ResizeObserver(() => map.resize());
    ro.observe(el);

    return () => {
      ro.disconnect();
      map.remove();
      mapRef.current = null;
      markerRef.current = [];
      didFit.current = false;
    };
  }, [pitched]);

  // Reconcile markers + initial fit.
  useEffect(() => {
    const map = mapRef.current;
    if (!map) return;

    markerRef.current.forEach((mk) => mk.remove());
    markerRef.current = markers.map((m) => {
      const { lng, lat } = resolve(m);
      const el = document.createElement('div');
      el.className = styles.roverDot;
      if (m.nick) el.title = m.nick;
      return new maplibregl.Marker({ element: el }).setLngLat([lng, lat]).addTo(map);
    });

    if (!didFit.current) {
      const pts = markers.filter((m) => m.position).map(resolve);
      const apply = () => {
        if (pts.length === 0) map.jumpTo({ center: [PARIS.lng, PARIS.lat], zoom: pitched ? 14 : 12 });
        else if (pts.length === 1) map.jumpTo({ center: [pts[0].lng, pts[0].lat], zoom: 15 });
        else {
          const b = new maplibregl.LngLatBounds();
          pts.forEach((p) => b.extend([p.lng, p.lat]));
          map.fitBounds(b, { padding: 48, maxZoom: 16.5, animate: false });
        }
      };
      if (map.isStyleLoaded()) apply();
      else map.once('load', apply);
      didFit.current = true;
    }
  }, [markers, pitched]);

  // Fly to a focused rover.
  useEffect(() => {
    const map = mapRef.current;
    if (!map || !focusId) return;
    const target = markers.find((m) => m.id === focusId);
    if (!target) return;
    const { lng, lat } = resolve(target);
    map.flyTo({ center: [lng, lat], zoom: Math.max(map.getZoom(), 14), speed: 1.2 });
  }, [focusId, markers]);

  return <div ref={containerRef} className={[styles.map, className].filter(Boolean).join(' ')} data-testid="location-map" />;
}

import '@testing-library/jest-dom/vitest';
import { vi } from 'vitest';

// jsdom has no ResizeObserver; LocationMap observes its container.
if (!('ResizeObserver' in globalThis)) {
  (globalThis as any).ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
}

// MapLibre needs WebGL, which jsdom lacks. Mock it so any view that embeds
// a map renders in tests without a real GL context.
vi.mock('maplibre-gl', () => {
  class FakeMarker {
    setLngLat() { return this; }
    addTo() { return this; }
    remove() { return this; }
    setPopup() { return this; }
    getElement() { return document.createElement('div'); }
  }
  class FakeMap {
    dragRotate = { disable() {}, enable() {} };
    touchZoomRotate = { disableRotation() {} };
    on() { return this; }
    once() { return this; }
    off() { return this; }
    addControl() { return this; }
    addSource() {}
    addLayer() {}
    getLayer() { return undefined; }
    setPaintProperty() {}
    isStyleLoaded() { return true; }
    fitBounds() {}
    flyTo() {}
    jumpTo() {}
    getZoom() { return 12; }
    resize() {}
    remove() {}
    getCanvas() { return document.createElement('canvas'); }
  }
  const mod = {
    Map: FakeMap,
    Marker: FakeMarker,
    NavigationControl: class {},
    Popup: class { setLngLat() { return this; } setDOMContent() { return this; } addTo() { return this; } remove() { return this; } },
    LngLatBounds: class { extend() { return this; } },
  };
  return { default: mod, ...mod };
});

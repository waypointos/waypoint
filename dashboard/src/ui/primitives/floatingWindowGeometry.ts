// dashboard/src/ui/primitives/floatingWindowGeometry.ts
//
// Pure geometry for FloatingWindow drag/resize, extracted for testability.
export interface Point { x: number; y: number; }
export interface Size { width: number; height: number; }
export interface Viewport { width: number; height: number; }

/** Clamp a window's top-left so it stays fully inside the viewport. */
export function clampPosition(pos: Point, size: Size, vp: Viewport): Point {
  const maxX = Math.max(0, vp.width - size.width);
  const maxY = Math.max(0, vp.height - size.height);
  return {
    x: Math.min(maxX, Math.max(0, pos.x)),
    y: Math.min(maxY, Math.max(0, pos.y)),
  };
}

/** Clamp a resize to a minimum size and to the viewport from the window origin. */
export function clampSize(size: Size, pos: Point, vp: Viewport, min: Size): Size {
  return {
    width: Math.min(vp.width - pos.x, Math.max(min.width, size.width)),
    height: Math.min(vp.height - pos.y, Math.max(min.height, size.height)),
  };
}

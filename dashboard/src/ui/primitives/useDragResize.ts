// dashboard/src/ui/primitives/useDragResize.ts
//
// Pointer-driven move + resize for floating UI. Gesture-start values are
// captured into `drag` so the listener effect depends only on `drag` (no
// per-frame re-subscribe) and tears down on pointerup or unmount.
import React, { useEffect, useState } from 'react';
import { clampPosition, clampSize, type Point, type Size } from './floatingWindowGeometry';

const viewport = () => ({ width: window.innerWidth, height: window.innerHeight });

export interface DragResizeOptions {
  initialPosition?: Point;
  initialSize?: Size;
  minSize?: Size;
}

export interface DragResize {
  pos: Point;
  size: Size;
  startMove: (e: React.PointerEvent) => void;
  startResize: (e: React.PointerEvent) => void;
}

export function useDragResize({
  initialPosition = { x: 80, y: 80 },
  initialSize = { width: 360, height: 480 },
  minSize = { width: 280, height: 220 },
}: DragResizeOptions = {}): DragResize {
  const [pos, setPos] = useState<Point>(initialPosition);
  const [size, setSize] = useState<Size>(initialSize);
  const [drag, setDrag] = useState<{
    mode: 'move' | 'resize';
    startX: number; startY: number;
    originPos: Point; originSize: Size; minSize: Size;
  } | null>(null);

  useEffect(() => {
    if (!drag) return;
    const onMove = (e: PointerEvent) => {
      const dx = e.clientX - drag.startX;
      const dy = e.clientY - drag.startY;
      if (drag.mode === 'move') {
        setPos(clampPosition(
          { x: drag.originPos.x + dx, y: drag.originPos.y + dy },
          drag.originSize, viewport(),
        ));
      } else {
        setSize(clampSize(
          { width: drag.originSize.width + dx, height: drag.originSize.height + dy },
          drag.originPos, viewport(), drag.minSize,
        ));
      }
    };
    const onUp = () => setDrag(null);
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
    return () => {
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', onUp);
    };
  }, [drag]);

  const startMove = (e: React.PointerEvent) =>
    setDrag({ mode: 'move', startX: e.clientX, startY: e.clientY, originPos: pos, originSize: size, minSize });
  const startResize = (e: React.PointerEvent) => {
    e.stopPropagation();
    setDrag({ mode: 'resize', startX: e.clientX, startY: e.clientY, originPos: pos, originSize: size, minSize });
  };

  return { pos, size, startMove, startResize };
}

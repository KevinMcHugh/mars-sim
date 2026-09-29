// The map camera: a center in tiles and a zoom in CSS pixels per tile. Pure
// math; input handling lives in input.ts.

export const MIN_ZOOM = 1;
export const MAX_ZOOM = 64;

export interface TileRect { x0: number; y0: number; x1: number; y1: number }

export class Camera {
  cx = 0;
  cy = 0;
  zoom = 12;
  /** Viewport size in CSS pixels. */
  width = 1;
  height = 1;

  /** The tile under a point in CSS pixels, as fractional tile coordinates. */
  toTile(sx: number, sy: number): [number, number] {
    return [this.cx + (sx - this.width / 2) / this.zoom, this.cy + (sy - this.height / 2) / this.zoom];
  }

  panPixels(dx: number, dy: number): void {
    this.cx -= dx / this.zoom;
    this.cy -= dy / this.zoom;
  }

  /** Zoom by factor, keeping the tile under (sx, sy) where it is. */
  zoomAt(factor: number, sx: number, sy: number): void {
    const [tx, ty] = this.toTile(sx, sy);
    this.zoom = Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, this.zoom * factor));
    const [nx, ny] = this.toTile(sx, sy);
    this.cx += tx - nx;
    this.cy += ty - ny;
  }

  /** Keep the center on the map, so the view can never be lost off its edge. */
  clampTo(mapW: number, mapH: number): void {
    this.cx = Math.min(mapW, Math.max(0, this.cx));
    this.cy = Math.min(mapH, Math.max(0, this.cy));
  }

  /** The tiles on screen, widened by margin tiles, clipped to the map. */
  visibleTiles(mapW: number, mapH: number, margin = 0): TileRect {
    const [x0, y0] = this.toTile(0, 0);
    const [x1, y1] = this.toTile(this.width, this.height);
    return {
      x0: Math.max(0, Math.floor(x0) - margin),
      y0: Math.max(0, Math.floor(y0) - margin),
      x1: Math.min(mapW, Math.ceil(x1) + margin),
      y1: Math.min(mapH, Math.ceil(y1) + margin),
    };
  }
}

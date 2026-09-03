export interface SpriteFrame {
  sx: number;
  sy: number;
  width: number;
  height: number;
}

export function splitHorizontal(sheetWidth: number, sheetHeight: number, frameWidth: number, frameHeight = sheetHeight): SpriteFrame[] {
  if (!Number.isInteger(sheetWidth) || !Number.isInteger(sheetHeight) || !Number.isInteger(frameWidth) || !Number.isInteger(frameHeight) || frameWidth <= 0 || frameHeight <= 0 || sheetWidth < frameWidth || sheetHeight < frameHeight || sheetWidth % frameWidth !== 0) return [];
  const count = Math.floor(sheetWidth / frameWidth);
  return Array.from({ length: count }, (_, index) => ({ sx: index * frameWidth, sy: 0, width: frameWidth, height: frameHeight }));
}

export function drawPixelFrame(ctx: CanvasRenderingContext2D, image: CanvasImageSource, frame: SpriteFrame, dx: number, dy: number, dw: number, dh: number): void {
  ctx.imageSmoothingEnabled = false;
  ctx.drawImage(image, frame.sx, frame.sy, frame.width, frame.height, dx, dy, dw, dh);
}

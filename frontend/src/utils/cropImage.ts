export interface ImageBox {
  x1: number;
  y1: number;
  x2: number;
  y2: number;
}

/** Crops srcUrl down to box (in the source image's own pixel coordinates)
 * and returns an object URL for the result. The caller owns both URLs and
 * is responsible for revoking them. */
export async function cropImage(srcUrl: string, box: ImageBox): Promise<string> {
  const img = new window.Image();
  img.crossOrigin = "anonymous";
  img.src = srcUrl;
  await new Promise<void>((resolve, reject) => {
    img.onload = () => resolve();
    img.onerror = () => reject(new Error("Could not load image to crop"));
  });

  const width = Math.max(1, Math.round(box.x2 - box.x1));
  const height = Math.max(1, Math.round(box.y2 - box.y1));
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("Could not crop image");
  ctx.drawImage(img, box.x1, box.y1, width, height, 0, 0, width, height);

  return new Promise<string>((resolve, reject) => {
    canvas.toBlob((blob) => {
      if (!blob) {
        reject(new Error("Could not crop image"));
        return;
      }
      resolve(URL.createObjectURL(blob));
    }, "image/png");
  });
}

// Levels (Photoshop's Níveis) as the image lab sets them; see imageproc.Levels.

export interface LevelsValue {
  inBlack: number;
  inWhite: number;
  gamma: number;
  outBlack: number;
  outWhite: number;
}

export const DEFAULT_LEVELS: LevelsValue = { inBlack: 0, inWhite: 255, gamma: 1, outBlack: 0, outWhite: 255 };

/** Input black and white at the 0.5th and 99.5th percentiles of the histogram. */
export function autoLevels(histogram: number[]): LevelsValue {
  const total = histogram.reduce((a, b) => a + b, 0);
  let acc = 0;
  let black = 0;
  let white = 255;
  for (let i = 0; i < 256; i++) {
    acc += histogram[i];
    if (acc <= total * 0.005) black = i;
    if (acc < total * 0.995) white = i + 1;
  }
  return { ...DEFAULT_LEVELS, inBlack: black, inWhite: Math.max(Math.min(white, 255), black + 1) };
}

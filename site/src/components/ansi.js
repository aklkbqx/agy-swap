/**
 * Maps xterm 256 color codes (0-255) to hex RGB strings.
 * Accurately covers standard colors, 6x6x6 RGB cube, and 24-step grayscale ramp.
 */
export function xterm256ToHex(code) {
  if (typeof code !== 'number' || code < 0 || code > 255) return null;

  // Standard 16 ANSI colors
  const STANDARD_16 = [
    '#000000', '#cd0000', '#00cd00', '#cdcd00', '#0000ee', '#cd00cd', '#00cdcd', '#e5e5e5',
    '#7f7f7f', '#ff0000', '#00ff00', '#ffff00', '#5c5cff', '#ff00ff', '#00ffff', '#ffffff',
  ];
  if (code < 16) return STANDARD_16[code];

  // 24-step grayscale ramp (codes 232 - 255)
  if (code >= 232) {
    const gray = 8 + (code - 232) * 10;
    const h = gray.toString(16).padStart(2, '0');
    return `#${h}${h}${h}`;
  }

  // 6x6x6 color cube (codes 16 - 231)
  const idx = code - 16;
  const r = Math.floor(idx / 36);
  const g = Math.floor((idx % 36) / 6);
  const b = idx % 6;
  const STEPS = [0, 95, 135, 175, 215, 255];
  const rh = STEPS[r].toString(16).padStart(2, '0');
  const gh = STEPS[g].toString(16).padStart(2, '0');
  const bh = STEPS[b].toString(16).padStart(2, '0');
  return `#${rh}${gh}${bh}`;
}

/**
 * Parses a single line with ANSI SGR escape sequences into styled token objects.
 * Whitelists only safe SGR codes (0 reset, 1 bold, 22 normal, 38;5;n 256 foreground, 39 default).
 */
export function parseAnsi(line) {
  if (typeof line !== 'string') return [];
  const ANSI_REGEX = /\x1b\[([0-9;]*)m/g;
  const tokens = [];
  let lastIndex = 0;
  let isBold = false;
  let currentColor = null;
  let match;

  while ((match = ANSI_REGEX.exec(line)) !== null) {
    const textBefore = line.slice(lastIndex, match.index);
    if (textBefore.length > 0) {
      tokens.push({
        text: textBefore,
        bold: isBold,
        color: currentColor,
      });
    }

    const codeStr = match[1] || '0';
    const codes = codeStr.split(';').map((c) => parseInt(c, 10) || 0);

    for (let i = 0; i < codes.length; i++) {
      const code = codes[i];
      if (code === 0) {
        isBold = false;
        currentColor = null;
      } else if (code === 1) {
        isBold = true;
      } else if (code === 22) {
        isBold = false;
      } else if (code === 38 && codes[i + 1] === 5 && i + 2 < codes.length) {
        const colorNum = codes[i + 2];
        currentColor = xterm256ToHex(colorNum);
        i += 2;
      } else if (code === 39) {
        currentColor = null;
      }
    }

    lastIndex = ANSI_REGEX.lastIndex;
  }

  const trailingText = line.slice(lastIndex);
  if (trailingText.length > 0) {
    tokens.push({
      text: trailingText,
      bold: isBold,
      color: currentColor,
    });
  }

  return tokens;
}

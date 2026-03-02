import { t, getLocale } from './i18n/index.svelte.js';

export function statusBadge(code) {
  if (code >= 200 && code < 300) return 'badge-success';
  if (code >= 300 && code < 400) return 'badge-info';
  if (code >= 400 && code < 500) return 'badge-warning';
  return 'badge-error';
}

export function fmt(ms) {
  return ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(1)}s`;
}
export function fmtSize(b) {
  return b < 1024
    ? `${b}B`
    : b < 1048576
      ? `${(b / 1024).toFixed(1)}KB`
      : b < 1073741824
        ? `${(b / 1048576).toFixed(1)}MB`
        : `${(b / 1073741824).toFixed(2)}GB`;
}
export function fmtN(n) {
  return new Intl.NumberFormat(getLocale()).format(n);
}
export function trunc(s, n) {
  return s && s.length > n ? s.slice(0, n) + '...' : s || '-';
}

export function timeAgo(date) {
  const d = new Date(date);
  const now = new Date();
  const diff = Math.floor((now - d) / 1000);
  if (diff < 60) return t('timeAgo.justNow');
  if (diff < 3600) return t('timeAgo.minutesAgo', { count: Math.floor(diff / 60) });
  if (diff < 86400) return t('timeAgo.hoursAgo', { count: Math.floor(diff / 3600) });
  return d.toLocaleDateString(getLocale());
}

export function a11yKeydown(callback) {
  return (e) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      callback(e);
    }
  };
}

export function copyToClipboard(text) {
  navigator.clipboard.writeText(text);
}

/**
 * Fetch all records from a paginated API endpoint.
 * @param {(limit: number, offset: number) => Promise<any[]>} fetchFn
 * @param {number} pageSize
 * @returns {Promise<any[]>}
 */
export async function fetchAll(fetchFn, pageSize = 100) {
  let all = [];
  let offset = 0;
  while (true) {
    const batch = await fetchFn(pageSize, offset);
    if (!batch || batch.length === 0) break;
    all = all.concat(batch);
    if (batch.length < pageSize) break;
    offset += pageSize;
  }
  return all;
}

/**
 * Generate a CSV string and trigger download.
 * @param {string} filename
 * @param {string[]} headers - Column labels
 * @param {string[]} keys - Property keys to extract from each row
 * @param {any[]} data - Array of row objects
 */
export function downloadCSV(filename, headers, keys, data) {
  const escape = (v) => {
    if (v == null) return '';
    const s = String(v);
    if (s.includes(',') || s.includes('"') || s.includes('\n')) {
      return '"' + s.replace(/"/g, '""') + '"';
    }
    return s;
  };

  const lines = [headers.map(escape).join(',')];
  for (const row of data) {
    lines.push(keys.map((k) => escape(row[k])).join(','));
  }

  const blob = new Blob(['\uFEFF' + lines.join('\n')], { type: 'text/csv;charset=utf-8;' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

// Squarified treemap layout algorithm
export function squarify(items, x, y, w, h) {
  if (items.length === 0 || w <= 0 || h <= 0) return [];
  const totalValue = items.reduce((s, it) => s + it.value, 0);
  if (totalValue <= 0) return [];
  const rects = [];
  let remaining = [...items];
  let cx = x,
    cy = y,
    cw = w,
    ch = h;

  while (remaining.length > 0) {
    const isWide = cw >= ch;
    const _side = isWide ? ch : cw;
    const totalRemaining = remaining.reduce((s, it) => s + it.value, 0);
    let row = [remaining[0]];
    let rowValue = remaining[0].value;

    const _worstRatio = (rv, s) => {
      const area = (rv / totalRemaining) * cw * ch;
      const rowLen = area / s;
      return Math.max(s / rowLen, rowLen / s);
    };

    for (let i = 1; i < remaining.length; i++) {
      const newRowValue = rowValue + remaining[i].value;
      const newArea = (newRowValue / totalRemaining) * cw * ch;
      const oldArea = (rowValue / totalRemaining) * cw * ch;
      const newSide = isWide ? newArea / ch : newArea / cw;
      const oldSide = isWide ? oldArea / ch : oldArea / cw;

      const oldWorst = Math.max(
        ...row.map((it) => {
          const a = (it.value / rowValue) * oldArea;
          const _r =
            oldSide > 0
              ? Math.max((a / (oldSide * oldSide)) * oldSide, oldSide / (a / oldSide))
              : Infinity;
          return Math.max(oldSide / (a / oldSide), a / oldSide / oldSide);
        }),
      );
      const newRow = [...row, remaining[i]];
      const newWorst = Math.max(
        ...newRow.map((it) => {
          const a = (it.value / newRowValue) * newArea;
          return Math.max(newSide / (a / newSide), a / newSide / newSide);
        }),
      );

      if (newWorst <= oldWorst) {
        row.push(remaining[i]);
        rowValue = newRowValue;
      } else {
        break;
      }
    }

    // Lay out the row
    const rowArea = (rowValue / totalRemaining) * cw * ch;
    const rowSide = isWide ? (ch > 0 ? rowArea / ch : 0) : cw > 0 ? rowArea / cw : 0;
    let offset = 0;
    for (const item of row) {
      const fraction = rowValue > 0 ? item.value / rowValue : 0;
      const itemLen = fraction * (isWide ? ch : cw);
      rects.push({
        ...item,
        x: isWide ? cx : cx + offset,
        y: isWide ? cy + offset : cy,
        w: isWide ? rowSide : itemLen,
        h: isWide ? itemLen : rowSide,
      });
      offset += itemLen;
    }

    // Reduce remaining area
    if (isWide) {
      cx += rowSide;
      cw -= rowSide;
    } else {
      cy += rowSide;
      ch -= rowSide;
    }
    remaining = remaining.slice(row.length);
  }
  return rects;
}

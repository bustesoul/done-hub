export function formatCompact(value, suffix = '') {
  const number = Number(value) || 0;
  const absolute = Math.abs(number);
  let display;
  if (absolute >= 1_000_000_000) display = `${trim(number / 1_000_000_000)} B`;
  else if (absolute >= 1_000_000) display = `${trim(number / 1_000_000)} M`;
  else if (absolute >= 1_000) display = `${trim(number / 1_000)} K`;
  else display = new Intl.NumberFormat().format(number);
  return `${display}${suffix}`;
}

export function exactNumber(value) {
  return new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 }).format(Number(value) || 0);
}

export function formatPercent(value) {
  return `${trim(Number(value) || 0)}%`;
}

export function formatLatency(value) {
  const milliseconds = Number(value) || 0;
  return milliseconds >= 1000 ? `${trim(milliseconds / 1000)} s` : `${trim(milliseconds)} ms`;
}

function trim(value) {
  return Number(value.toFixed(value >= 100 ? 0 : value >= 10 ? 1 : 2)).toString();
}

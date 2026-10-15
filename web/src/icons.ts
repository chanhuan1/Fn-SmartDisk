// Minimal inline SVG icon set (stroke-based, lucide-style) to avoid an icon
// dependency. Each is a function returning SVG path content.
import { h, type VNode } from "vue";

function svg(paths: string, size = 16): VNode {
  // Icons are plain functions used directly in templates as <IconX :size="n" />.
  // Vue then passes { size: n } as the first argument, so unwrap that shape;
  // a bare number still works when invoked as a plain function call.
  let px = size;
  if (typeof size === "object" && size !== null) {
    const v = (size as { size?: unknown }).size;
    px = typeof v === "number" ? v : 16;
  }
  return h(
    "svg",
    {
      viewBox: "0 0 24 24",
      fill: "none",
      stroke: "currentColor",
      "stroke-width": 2,
      "stroke-linecap": "round",
      "stroke-linejoin": "round",
      width: px,
      height: px,
      "aria-hidden": "true",
      innerHTML: paths,
    },
  );
}

export const IconDisk = (size = 18): VNode =>
  svg(
    '<line x1="22" y1="12" x2="2" y2="12"/><path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/><line x1="6" y1="16" x2="6.01" y2="16"/><line x1="10" y1="16" x2="10.01" y2="16"/>',
    size,
  );

export const IconRefresh = (size = 15): VNode =>
  svg(
    '<path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M3 21v-5h5"/>',
    size,
  );

export const IconBack = (size = 15): VNode =>
  svg('<path d="m12 19-7-7 7-7"/><path d="M19 12H5"/>', size);

export const IconWarn = (size = 16): VNode =>
  svg(
    '<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"/><path d="M12 9v4"/><path d="M12 17h.01"/>',
    size,
  );

export const IconCheck = (size = 14): VNode =>
  svg('<path d="M20 6 9 17l-5-5"/>', size);

// Moon: put a drive to sleep.
export const IconMoon = (size = 15): VNode =>
  svg('<path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z"/>', size);

// Power off: deep kernel-level offline.
export const IconPowerOff = (size = 15): VNode =>
  svg('<path d="M18.36 6.64a9 9 0 1 1-12.73 0"/><line x1="12" y1="2" x2="12" y2="12"/>', size);

// Zap / sunrise: wake a drive.
export const IconWake = (size = 15): VNode =>
  svg('<path d="M13 2 3 14h9l-1 8 10-12h-9l1-8z"/>', size);

// Scroll/list: event log.
export const IconLog = (size = 15): VNode =>
  svg(
    '<path d="M8 6h13"/><path d="M8 12h13"/><path d="M8 18h13"/><path d="M3 6h.01"/><path d="M3 12h.01"/><path d="M3 18h.01"/>',
    size,
  );

// HDD: spinning platter with actuator arm.
export const IconHdd = (size = 18): VNode =>
  svg(
    '<circle cx="12" cy="12" r="8"/><circle cx="12" cy="12" r="2.2"/><path d="M12 14.2 17 19"/><circle cx="17.6" cy="19.6" r=".4"/>',
    size,
  );

// SSD: memory stick silhouette.
export const IconSsd = (size = 18): VNode =>
  svg(
    '<rect x="3" y="7" width="18" height="10" rx="2"/><path d="M7 17v2"/><path d="M11 17v2"/><path d="M15 17v2"/><path d="M6.5 10.5h3v3h-3z"/><path d="M13 12h5"/>',
    size,
  );

// NVMe: M.2 stick with notch.
export const IconNvme = (size = 18): VNode =>
  svg(
    '<path d="M2 10.5 6 7h13a3 3 0 0 1 3 3v4a3 3 0 0 1-3 3H6l-4-3.5z"/><path d="M17 7v10"/><path d="M6.5 10.8h2"/><path d="M10 10.8h2"/><path d="M13.5 10.8h1"/>',
    size,
  );

// Lightning bolt (power running).
export const IconBolt = (size = 13): VNode =>
  svg('<path d="M13 2 3 14h9l-1 8 10-12h-9l1-8z"/>', size);

// Question mark in circle (unknown state).
export const IconQuestion = (size = 13): VNode =>
  svg('<circle cx="12" cy="12" r="9"/><path d="M9.2 9a2.8 2.8 0 0 1 5.5.7c0 1.8-2.7 2.3-2.7 3.8"/><path d="M12 16.8h.01"/>', size);

// Magnifier (filter).
export const IconSearch = (size = 14): VNode =>
  svg('<circle cx="11" cy="11" r="7"/><path d="m21 21-4.35-4.35"/>', size);

// Info (tooltip hint).
export const IconInfo = (size = 13): VNode =>
  svg('<circle cx="12" cy="12" r="9"/><path d="M12 11v5"/><path d="M12 8h.01"/>', size);

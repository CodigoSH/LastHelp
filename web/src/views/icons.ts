/**
 * The icon set.
 *
 * Inline SVG rather than a font or a sprite file: a font needs a network round
 * trip before anything is legible and renders as a box until it arrives, and a
 * sprite needs a request of its own for a handful of paths that weigh less than
 * the request. These are a few hundred bytes inside a bundle that was being
 * downloaded anyway.
 *
 * One geometry for all of them — 24×24, no fill, 2px stroke, round caps, and
 * `currentColor` — which is the house's, and is what lets an icon sit inside a
 * button and take the button's colour without being told twice.
 */
function stroke(paths: string, size = 16): string {
  return `<svg class="icon" viewBox="0 0 24 24" width="${size}" height="${size}" fill="none"
    stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"
    aria-hidden="true" focusable="false">${paths}</svg>`;
}

export const icons = {
  add: (s?: number) => stroke(`<path d="M12 5v14M5 12h14"/>`, s),
  edit: (s?: number) =>
    stroke(
      `<path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/>
       <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"/>`,
      s,
    ),
  remove: (s?: number) =>
    stroke(
      `<path d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
       <path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/>
       <path d="M10 11v6M14 11v6"/>`,
      s,
    ),
  close: (s?: number) => stroke(`<path d="M18 6L6 18M6 6l12 12"/>`, s),
  check: (s?: number) => stroke(`<path d="M20 6L9 17l-5-5"/>`, s),

  organization: (s?: number) =>
    stroke(
      `<path d="M3 21h18M5 21V7l7-4 7 4v14"/><path d="M9 21v-6h6v6"/><path d="M9 9h.01M15 9h.01"/>`,
      s,
    ),
  repository: (s?: number) =>
    stroke(
      `<path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/>
       <path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/>`,
      s,
    ),
  person: (s?: number) =>
    stroke(`<path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/>`, s),
  people: (s?: number) =>
    stroke(
      `<path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/>
       <path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/>`,
      s,
    ),

  overview: (s?: number) =>
    stroke(`<rect x="3" y="3" width="7" height="9"/><rect x="14" y="3" width="7" height="5"/>
            <rect x="14" y="12" width="7" height="9"/><rect x="3" y="16" width="7" height="5"/>`, s),
  palette: (s?: number) =>
    stroke(
      `<circle cx="13.5" cy="6.5" r=".5"/><circle cx="17.5" cy="10.5" r=".5"/>
       <circle cx="8.5" cy="7.5" r=".5"/><circle cx="6.5" cy="12.5" r=".5"/>
       <path d="M12 2C6.5 2 2 6.5 2 12s4.5 10 10 10c.926 0 1.648-.746 1.648-1.688 0-.437-.18-.835-.437-1.125-.29-.289-.438-.652-.438-1.125a1.64 1.64 0 0 1 1.668-1.668h1.996c3.051 0 5.555-2.503 5.555-5.554C21.965 6.012 17.461 2 12 2z"/>`,
      s,
    ),
  // Personalization is a brush; appearance is a palette. Two settings screens
  // wearing the same picture is a menu that looks like it repeats itself.
  brush: (s?: number) =>
    stroke(
      `<path d="M9.06 11.9l8.07-8.06a2.85 2.85 0 1 1 4.03 4.03l-8.06 8.08"/>
       <path d="M7.07 14.94c-1.66 0-3 1.35-3 3.02 0 1.33-2.5 1.52-2 2.02 1.08 1.1 2.49 2.02 4 2.02 2.2 0 4-1.8 4-4.04a3.01 3.01 0 0 0-3-3.02z"/>`,
      s,
    ),
  backup: (s?: number) =>
    stroke(
      `<ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M21 12c0 1.66-4 3-9 3s-9-1.34-9-3"/>
       <path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5"/>`,
      s,
    ),
  download: (s?: number) =>
    stroke(`<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="M7 10l5 5 5-5"/><path d="M12 15V3"/>`, s),
  upload: (s?: number) =>
    stroke(`<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="M17 8l-5-5-5 5"/><path d="M12 3v12"/>`, s),
  update: (s?: number) =>
    stroke(`<path d="M23 4v6h-6"/><path d="M1 20v-6h6"/>
            <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>`, s),
  info: (s?: number) => stroke(`<circle cx="12" cy="12" r="10"/><path d="M12 16v-4M12 8h.01"/>`, s),
  warning: (s?: number) =>
    stroke(
      `<path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/>
       <path d="M12 9v4M12 17h.01"/>`,
      s,
    ),
  bug: (s?: number) =>
    stroke(
      `<rect x="8" y="6" width="8" height="14" rx="4"/><path d="M19 7l-3 2M5 7l3 2"/>
       <path d="M19 13h3M2 13h3M19 19l-3-2M5 19l3-2"/><path d="M9 6V4a3 3 0 0 1 6 0v2"/>`,
      s,
    ),
  idea: (s?: number) =>
    stroke(
      `<path d="M9 18h6M10 22h4"/>
       <path d="M12 2a7 7 0 0 0-4 12.7V17h8v-2.3A7 7 0 0 0 12 2z"/>`,
      s,
    ),
  queue: (s?: number) =>
    stroke(`<path d="M12 8v4l3 3"/><circle cx="12" cy="12" r="10"/>`, s),
  copy: (s?: number) =>
    stroke(
      `<rect x="9" y="9" width="13" height="13" rx="2"/>
       <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>`,
      s,
    ),
  open: (s?: number) =>
    stroke(`<path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/><path d="M15 3h6v6"/><path d="M10 14L21 3"/>`, s),
};

export type IconName = keyof typeof icons;

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

/**
 * The one icon in this set that is filled rather than stroked.
 *
 * Three dots at a 2px stroke are three tiny rings, which read as noise at 16px.
 * The house's mark for a menu, so every menu of three dots is the same
 * button.
 */
function dotsIcon(size = 16): string {
  return `<svg class="icon" viewBox="0 0 24 24" width="${size}" height="${size}"
    fill="currentColor" aria-hidden="true" focusable="false"><circle cx="12" cy="5" r="1.7"/><circle cx="12" cy="12" r="1.7"/><circle cx="12" cy="19" r="1.7"/></svg>`;
}

/**
 * Up to date: a filled circle with the tick cut out of it. Filled, like dots,
 * because at 18px a stroked ring and a stroked tick crowd each other.
 */
function doneIcon(size = 18): string {
  return `<svg class="icon" viewBox="0 0 24 24" width="${size}" height="${size}"
    fill="currentColor" aria-hidden="true" focusable="false"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm-2 15l-5-5 1.41-1.41L10 14.17l7.59-7.59L19 8l-9 9z"/></svg>`;
}

export const icons = {
  dots: dotsIcon,
  /** Six dots in two columns: the handle something is dragged by. */
  grip: (size = 16) =>
    `<svg class="icon" viewBox="0 0 24 24" width="${size}" height="${size}" fill="currentColor" aria-hidden="true" focusable="false"><circle cx="9" cy="6" r="1.6"/><circle cx="15" cy="6" r="1.6"/><circle cx="9" cy="12" r="1.6"/><circle cx="15" cy="12" r="1.6"/><circle cx="9" cy="18" r="1.6"/><circle cx="15" cy="18" r="1.6"/></svg>`,
  done: doneIcon,
  add: (s?: number) => stroke(`<path d="M12 5v14M5 12h14"/>`, s),
  minus: (s?: number) => stroke(`<path d="M5 12h14"/>`, s),
  /** A message on a report: the speech bubble the product already draws. */
  comment: (s?: number) =>
    stroke(`<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>`, s),

  undo: (s?: number) => stroke(`<path d="M9 14L4 9l5-5"/><path d="M4 9h11a5 5 0 0 1 0 10h-4"/>`, s),
  redo: (s?: number) => stroke(`<path d="M15 14l5-5-5-5"/><path d="M20 9H9a5 5 0 0 0 0 10h4"/>`, s),
  bold: (s?: number) =>
    stroke(`<path d="M7 4h6a4 4 0 0 1 0 8H7z"/><path d="M7 12h7a4 4 0 0 1 0 8H7z"/>`, s),
  italic: (s?: number) => stroke(`<path d="M15 4h-5M14 20H9M15 4l-4 16"/>`, s),
  underline: (s?: number) => stroke(`<path d="M7 4v7a5 5 0 0 0 10 0V4"/><path d="M5 20h14"/>`, s),
  strike: (s?: number) =>
    stroke(`<path d="M5 12h14"/><path d="M8 7a4 3 0 0 1 8 0"/><path d="M16 16a4 3 0 0 1-8 0"/>`, s),
  alignStart: (s?: number) => stroke(`<path d="M4 6h16M4 12h10M4 18h13"/>`, s),
  alignCenter: (s?: number) => stroke(`<path d="M4 6h16M7 12h10M6 18h12"/>`, s),
  alignEnd: (s?: number) => stroke(`<path d="M4 6h16M10 12h10M7 18h13"/>`, s),
  alignJustify: (s?: number) => stroke(`<path d="M4 6h16M4 12h16M4 18h16"/>`, s),
  bullets: (s?: number) =>
    stroke(`<path d="M9 6h11M9 12h11M9 18h11"/><path d="M5 6h.01M5 12h.01M5 18h.01"/>`, s),
  numbers: (s?: number) =>
    stroke(`<path d="M10 6h10M10 12h10M10 18h10"/><path d="M4 5h1v4"/><path d="M4 15h2v1H4v2h2"/>`, s),
  image: (s?: number) =>
    stroke(
      `<rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="8.5" cy="8.5" r="1.5"/><path d="M21 15l-5-5L5 21"/>`,
      s,
    ),
  link: (s?: number) =>
    stroke(
      `<path d="M10 13a5 5 0 0 0 7 0l2-2a5 5 0 0 0-7-7l-1 1"/><path d="M14 11a5 5 0 0 0-7 0l-2 2a5 5 0 0 0 7 7l1-1"/>`,
      s,
    ),

  /**
   * The two marks the add menu uses: four squares for a thing that goes in
   * the grid, and a dashed outline for the thing that holds them.
   */
  module: (s?: number) =>
    stroke(
      `<rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/>`,
      s,
    ),
  section: (s = 16) =>
    `<svg class="icon" viewBox="0 0 24 24" width="${s}" height="${s}" fill="none"
    stroke="currentColor" stroke-width="2" stroke-dasharray="4 2"
    aria-hidden="true" focusable="false"><rect x="3" y="3" width="18" height="18" rx="2"/></svg>`,
  /* The arrow on anything that leads somewhere. It points along the line, and
     the line runs the other way in three of the twenty languages: the flip is
     done in the stylesheet with --rtl-flip rather than by drawing it twice. */
  chevron: (s?: number) => stroke(`<path d="m9 6 6 6-6 6"/>`, s),
  /* Leaving. It points at the door, so it is flipped where the line runs the
     other way — the stylesheet does that with --rtl-flip. */
  signOut: (s?: number) =>
    stroke(`<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4m7 14l5-5-5-5m5 5H9"/>`, s),
  search: (s?: number) => stroke(`<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>`, s),
  /* The three answers to "which theme": a sun, a moon and a screen. They are
     drawn rather than written because the question is answered by pointing, and
     because a row of three words is a control somebody reads instead of one
     they recognise. */
  sun: (s?: number) =>
    stroke(
      `<circle cx="12" cy="12" r="4"/><path d="M12 2v2m0 16v2M4.9 4.9l1.4 1.4m11.4 11.4 1.4 1.4M2 12h2m16 0h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>`,
      s,
    ),
  moon: (s?: number) => stroke(`<path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8Z"/>`, s),
  monitor: (s?: number) =>
    stroke(`<rect x="2" y="4" width="20" height="13" rx="2"/><path d="M8 21h8m-4-4v4"/>`, s),
  feed: (s?: number) =>
    stroke(`<path d="M4 11a9 9 0 0 1 9 9"/><path d="M4 4a16 16 0 0 1 16 16"/><circle cx="5" cy="19" r="1.2"/>`, s),
  clock: (s?: number) => stroke(`<circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/>`, s),
  /** Source code: the two angle brackets. */
  code: (s?: number) => stroke(`<polyline points="16 18 22 12 16 6"/><polyline points="8 6 2 12 8 18"/>`, s),
  heart: (s?: number) =>
    stroke(
      `<path d="M20.8 4.6a5.5 5.5 0 0 0-7.8 0L12 5.7l-1-1.1a5.5 5.5 0 0 0-7.8 7.8l1 1.1L12 21l7.8-7.5 1-1.1a5.5 5.5 0 0 0 0-7.8z"/>`,
      s,
    ),
  /** A disk, for storage that is not the database. */
  disk: (s?: number) =>
    stroke(
      `<path d="M22 12H2"/><path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z"/><path d="M6 16h.01M10 16h.01"/>`,
      s,
    ),
  /** A chip, for memory. */
  chip: (s?: number) =>
    stroke(
      `<rect x="5" y="5" width="14" height="14" rx="2"/><rect x="9" y="9" width="6" height="6"/><path d="M9 2v3M15 2v3M9 19v3M15 19v3M19 9h3M19 15h3M2 9h3M2 15h3"/>`,
      s,
    ),
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
  bell: (s?: number) =>
    stroke(
      `<path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/>
       <path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/>`,
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

/**
 * Pure helpers for two-step sign-in: what is typed, shown and saved.
 */

/** The setup key in groups of four, the way authenticator apps show it. */
export const formatSecret = (secret: string): string =>
    secret.replace(/\s+/g, '').replace(/(.{4})(?=.)/g, '$1 ');

/** The recovery codes as a text file: one per line, nothing else. */
export const recoveryCodesText = (codes: string[]): string =>
    codes.length ? codes.join('\n') + '\n' : '';

/** True when the text is the six digits of an authenticator app (spaces allowed). */
export const looksLikeAppCode = (typed: string): boolean =>
    /^\d{6}$/.test(typed.replace(/\s+/g, ''));

/**
 * What goes to the server: an app code loses the spaces apps print in the
 * middle ("123 456"); anything else (a recovery code) is sent as typed, trimmed.
 */
export const normalizeCode = (typed: string): string =>
    looksLikeAppCode(typed) ? typed.replace(/\s+/g, '') : typed.trim();

/** The longest thing the field takes: a recovery code with its dashes, and room for a pasted space. */
export const CODE_MAX_LENGTH = 24;

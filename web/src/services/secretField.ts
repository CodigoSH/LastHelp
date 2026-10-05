/**
 * A stored secret in a form: shown as dots, never as its value.
 *
 * The server never returns a secret, only whether there is one, so the browser
 * cannot show what it does not have. The field carries a mask of dots when a secret
 * is saved, and there are two gestures: write (it replaces) or empty the field (it
 * removes). The dots are a sign and not data, and this is the function that reads
 * them, so what the form sends does not depend on how somebody edited the mask.
 */
export const MASK = "••••••••";

export type SecretIntent =
  | { kind: "keep" }
  | { kind: "set"; secret: string }
  | { kind: "clear" };

/**
 * What the field says to do with the secret. Dots alone, or half deleted, change
 * nothing; anything written beside or over them is the new secret; an emptied field,
 * when there was a secret, removes it.
 */
export function secretIntent(value: string, hadSecret: boolean): SecretIntent {
  if (value === "") return hadSecret ? { kind: "clear" } : { kind: "keep" };
  const written = value.replaceAll("•", "");
  return written === "" ? { kind: "keep" } : { kind: "set", secret: written };
}

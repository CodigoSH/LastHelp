/**
 * A saved secret's field, as the house shows every one: a password box, dots when
 * one is saved, no way to reveal it and no button to remove it. The administrator
 * can see THAT there is a secret, never WHAT it is. Typing replaces it; emptying
 * the box takes it away.
 *
 * The page never receives the secret, so the dots are only a sign. They have to be
 * told apart from something typed, and that is all this decides.
 */

export const SECRET_MASK = "••••••••";

export type SecretChange =
  | { action: "keep" }
  | { action: "clear" }
  | { action: "set"; token: string };

/**
 * What a change in the box means. The box starts as the mask when a secret is
 * saved, empty when not. Anything typed over or beside the dots is a new secret (a
 * secret never contains a dot of this kind); the dots alone, whole or partly erased,
 * are the sign and change nothing; an empty box is an empty secret.
 */
export function secretChange(raw: string, hasToken: boolean): SecretChange {
  const typed = raw.replace(/•/g, "").trim();
  if (typed) return { action: "set", token: typed };
  if (raw === "") return hasToken ? { action: "clear" } : { action: "keep" };
  return { action: "keep" };
}

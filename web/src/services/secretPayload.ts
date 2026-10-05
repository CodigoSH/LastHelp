/**
 * The bodies a secret's form is saved with.
 *
 * Kept apart so a test can look at them: the server has its own tests for what a
 * secret does when it arrives, and none of them can see that a mapping left out the
 * mark that says "the box was emptied" — the secret would never be removed and the
 * save would still answer "Saved".
 */
import { secretChange } from "./secretField";

/** What a notification channel's form is saved with. */
export function channelPayload(
  form: { kind: string; url: string; token: string; to: string },
  hadToken: boolean,
): { kind: string; url: string; token: string; to: string; clear_token: boolean } {
  const change = secretChange(form.token, hadToken);
  return {
    kind: form.kind,
    url: form.url,
    // Empty means "keep": a browser that has not reloaded sends the form without
    // this field, and the removal travels as its own mark.
    token: change.action === "set" ? change.token : "",
    to: form.to,
    clear_token: change.action === "clear",
  };
}

/** What an organisation's forge credentials are saved with. */
export function credentialsPayload(
  form: { token: string; webhook_secret: string },
  has: { token: boolean; secret: boolean },
): { token: string; webhook_secret: string; clear_token: boolean; clear_webhook_secret: boolean } {
  const token = secretChange(form.token, has.token);
  const secret = secretChange(form.webhook_secret, has.secret);
  return {
    token: token.action === "set" ? token.token : "",
    webhook_secret: secret.action === "set" ? secret.token : "",
    clear_token: token.action === "clear",
    clear_webhook_secret: secret.action === "clear",
  };
}

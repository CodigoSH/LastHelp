import { describe, expect, test } from "bun:test";
import { SECRET_MASK } from "./secretField";
import { channelPayload, credentialsPayload } from "./secretPayload";

const channel = { kind: "ntfy", url: "https://ntfy.example/t", to: "" };

describe("a channel's body", () => {
  test("carries the mark that says the box was emptied", () => {
    expect(channelPayload({ ...channel, token: "" }, true)).toMatchObject({ token: "", clear_token: true });
  });
  test("keeps the saved token when the dots are left", () => {
    expect(channelPayload({ ...channel, token: SECRET_MASK }, true)).toMatchObject({ token: "", clear_token: false });
  });
  test("carries a new token and no removal", () => {
    expect(channelPayload({ ...channel, token: "tk_new" }, true)).toMatchObject({ token: "tk_new", clear_token: false });
  });
  test("removes nothing when nothing was saved", () => {
    expect(channelPayload({ ...channel, token: "" }, false)).toMatchObject({ token: "", clear_token: false });
  });
});

describe("an organisation's credentials body", () => {
  const both = { token: true, secret: true };
  test("each secret is decided on its own", () => {
    expect(credentialsPayload({ token: SECRET_MASK, webhook_secret: "" }, both)).toEqual({
      token: "",
      webhook_secret: "",
      clear_token: false,
      clear_webhook_secret: true,
    });
    expect(credentialsPayload({ token: "tk_new", webhook_secret: SECRET_MASK }, both)).toEqual({
      token: "tk_new",
      webhook_secret: "",
      clear_token: false,
      clear_webhook_secret: false,
    });
  });
  test("a secret that was never saved is not removed by being empty", () => {
    expect(credentialsPayload({ token: "tk", webhook_secret: "" }, { token: false, secret: false })).toEqual({
      token: "tk",
      webhook_secret: "",
      clear_token: false,
      clear_webhook_secret: false,
    });
  });
});

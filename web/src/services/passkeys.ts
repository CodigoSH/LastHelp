/**
 * Passkeys (WebAuthn) without the browser: everything that can be done on
 * plain data lives here so it can be tested. The server speaks base64url
 * without padding; the browser's API wants ArrayBuffers.
 */
import { userAgentLabel } from './userAgentLabel';

const ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_';

/** Bytes to base64url, no padding. */
export function bufferToBase64url(buf: ArrayBuffer | Uint8Array): string {
    const bytes = buf instanceof Uint8Array ? buf : new Uint8Array(buf);
    let out = '';
    for (let i = 0; i < bytes.length; i += 3) {
        const b0 = bytes[i]!;
        const b1 = i + 1 < bytes.length ? bytes[i + 1]! : 0;
        const b2 = i + 2 < bytes.length ? bytes[i + 2]! : 0;
        out += ALPHABET[b0 >> 2];
        out += ALPHABET[((b0 & 3) << 4) | (b1 >> 4)];
        if (i + 1 < bytes.length) out += ALPHABET[((b1 & 15) << 2) | (b2 >> 6)];
        if (i + 2 < bytes.length) out += ALPHABET[b2 & 63];
    }
    return out;
}

/** base64url (padded or not) to bytes. Characters outside the alphabet are an error. */
export function base64urlToBuffer(text: string): ArrayBuffer {
    const clean = text.replace(/=+$/, '');
    if (clean.length % 4 === 1) throw new Error('invalid base64url length');
    const out = new Uint8Array(Math.floor((clean.length * 3) / 4));
    let acc = 0;
    let bits = 0;
    let n = 0;
    for (const ch of clean) {
        const v = ALPHABET.indexOf(ch);
        if (v < 0) throw new Error('invalid base64url character');
        acc = (acc << 6) | v;
        bits += 6;
        if (bits >= 8) {
            bits -= 8;
            out[n++] = (acc >> bits) & 255;
        }
    }
    return out.buffer;
}

type Json = Record<string, any>;

/** The server's creation options, with the binary fields turned into ArrayBuffers. */
export function creationOptionsFromServer(json: Json): Json {
    const out: Json = { ...json };
    out.challenge = base64urlToBuffer(String(json.challenge ?? ''));
    if (json.user) out.user = { ...json.user, id: base64urlToBuffer(String(json.user.id ?? '')) };
    if (Array.isArray(json.excludeCredentials)) {
        out.excludeCredentials = json.excludeCredentials.map((c: Json) => ({ ...c, id: base64urlToBuffer(String(c.id ?? '')) }));
    }
    return out;
}

/** The server's sign-in options, with the binary fields turned into ArrayBuffers. */
export function requestOptionsFromServer(json: Json): Json {
    const out: Json = { ...json };
    out.challenge = base64urlToBuffer(String(json.challenge ?? ''));
    if (Array.isArray(json.allowCredentials)) {
        out.allowCredentials = json.allowCredentials.map((c: Json) => ({ ...c, id: base64urlToBuffer(String(c.id ?? '')) }));
    }
    return out;
}

/** What `navigator.credentials.get` returned, as plain buffers. */
export interface AssertionParts {
    rawId: ArrayBuffer;
    clientDataJSON: ArrayBuffer;
    authenticatorData: ArrayBuffer;
    signature: ArrayBuffer;
    userHandle: ArrayBuffer | null;
}

/** What `navigator.credentials.create` returned, as plain buffers. */
export interface AttestationParts {
    clientDataJSON: ArrayBuffer;
    attestationObject: ArrayBuffer;
}

export function assertionToFinishBody(a: AssertionParts) {
    return {
        credentialId: bufferToBase64url(a.rawId),
        clientDataJSON: bufferToBase64url(a.clientDataJSON),
        authenticatorData: bufferToBase64url(a.authenticatorData),
        signature: bufferToBase64url(a.signature),
        userHandle: a.userHandle ? bufferToBase64url(a.userHandle) : '',
    };
}

export function attestationToFinishBody(name: string, a: AttestationParts) {
    return {
        name,
        clientDataJSON: bufferToBase64url(a.clientDataJSON),
        attestationObject: bufferToBase64url(a.attestationObject),
    };
}

/** Whether this page can ask for passkeys at all (the server may still refuse the address). */
export function passkeysSupported(win: { PublicKeyCredential?: unknown; isSecureContext?: boolean; location?: { hostname: string } }): boolean {
    // A passkey belongs to a name: a page opened by IP address can never have
    // one, though the browser counts 127.0.0.1 as a secure context.
    const host = win.location?.hostname ?? '';
    const isIP = /^\d{1,3}(\.\d{1,3}){3}$/.test(host) || host.includes(':') || host.startsWith('[');
    return !!win.PublicKeyCredential && !!win.isSecureContext && !isIP;
}

const BROWSERS: Record<string, string> = { firefox: 'Firefox', chrome: 'Chrome', edge: 'Edge', safari: 'Safari', opera: 'Opera' };
const SYSTEMS: Record<string, string> = {
    windows: 'Windows', macos: 'macOS', linux: 'Linux', android: 'Android', ios: 'iOS', chromeos: 'ChromeOS',
};

/** A name to propose for a new passkey: "Firefox · Linux", the way the list of sessions names a browser. Product names, so not translated. */
export function defaultPasskeyName(userAgent: string): string {
    const { browser, os } = userAgentLabel(userAgent);
    const b = BROWSERS[browser];
    const s = SYSTEMS[os];
    if (b && s) return `${b} · ${s}`;
    return b || s || 'Passkey';
}

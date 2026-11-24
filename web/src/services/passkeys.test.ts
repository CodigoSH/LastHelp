import { describe, expect, test } from 'bun:test';
import {
    bufferToBase64url, base64urlToBuffer, creationOptionsFromServer, requestOptionsFromServer,
    assertionToFinishBody, attestationToFinishBody, passkeysSupported, defaultPasskeyName,
} from './passkeys';

const bytes = (n: number) => Uint8Array.from({ length: n }, (_, i) => (i * 37 + 250) & 255);
const same = (a: ArrayBuffer, b: Uint8Array) => expect(Array.from(new Uint8Array(a))).toEqual(Array.from(b));

describe('base64url', () => {
    test('round trips at every remainder', () => {
        for (const n of [0, 1, 2, 3, 31, 32, 33, 64]) {
            const b = bytes(n);
            const s = bufferToBase64url(b);
            expect(s).not.toMatch(/[+/=]/);
            same(base64urlToBuffer(s), b);
        }
    });
    test('known vectors', () => {
        expect(bufferToBase64url(new TextEncoder().encode('f'))).toBe('Zg');
        expect(bufferToBase64url(new TextEncoder().encode('fo'))).toBe('Zm8');
        expect(bufferToBase64url(new TextEncoder().encode('foo'))).toBe('Zm9v');
        expect(bufferToBase64url(Uint8Array.from([251, 255]))).toBe('-_8');
    });
    test('accepts padded input', () => {
        same(base64urlToBuffer('Zg=='), Uint8Array.from([102]));
        same(base64urlToBuffer('Zm8='), Uint8Array.from([102, 111]));
    });
    test('rejects what is not base64url', () => {
        expect(() => base64urlToBuffer('a+b/')).toThrow();
        expect(() => base64urlToBuffer('abcde')).toThrow();
    });
});

describe('options', () => {
    test('creation: challenge, user id and excluded ids become buffers; the rest passes through', () => {
        const json = {
            challenge: 'AQID', rp: { name: 'x' }, timeout: 60000,
            user: { id: 'BAUG', name: 'a', displayName: 'a' },
            excludeCredentials: [{ type: 'public-key', id: 'Bwg' }],
            authenticatorSelection: { userVerification: 'required' },
        };
        const out = creationOptionsFromServer(json);
        same(out.challenge, Uint8Array.from([1, 2, 3]));
        same(out.user.id, Uint8Array.from([4, 5, 6]));
        same(out.excludeCredentials[0].id, Uint8Array.from([7, 8]));
        expect(out.excludeCredentials[0].type).toBe('public-key');
        expect(out.rp).toEqual({ name: 'x' });
        expect(out.authenticatorSelection).toEqual({ userVerification: 'required' });
        // pure: the input is untouched
        expect(json.challenge).toBe('AQID');
        expect(json.user.id).toBe('BAUG');
        expect(json.excludeCredentials[0]!.id).toBe('Bwg');
    });
    test('creation without excluded credentials', () => {
        const out = creationOptionsFromServer({ challenge: 'AQ', user: { id: 'Ag' } });
        expect(out.excludeCredentials).toBeUndefined();
    });
    test('request: challenge becomes a buffer, the rest passes through', () => {
        const json = { challenge: 'AQID', rpId: 'example.com', timeout: 1, userVerification: 'required' };
        const out = requestOptionsFromServer(json);
        same(out.challenge, Uint8Array.from([1, 2, 3]));
        expect(out.rpId).toBe('example.com');
        expect(out.userVerification).toBe('required');
        expect(json.challenge).toBe('AQID');
    });
});

describe('finish bodies', () => {
    test('assertion', () => {
        const body = assertionToFinishBody({
            rawId: bytes(3).buffer as ArrayBuffer, clientDataJSON: Uint8Array.from([1]).buffer as ArrayBuffer,
            authenticatorData: Uint8Array.from([2]).buffer as ArrayBuffer, signature: Uint8Array.from([3]).buffer as ArrayBuffer,
            userHandle: Uint8Array.from([4]).buffer as ArrayBuffer,
        });
        expect(Object.keys(body).sort()).toEqual(['authenticatorData', 'clientDataJSON', 'credentialId', 'signature', 'userHandle']);
        expect(body.clientDataJSON).toBe('AQ');
        expect(body.authenticatorData).toBe('Ag');
        expect(body.signature).toBe('Aw');
        expect(body.userHandle).toBe('BA');
    });
    test('assertion without a user handle sends an empty one', () => {
        const e = new ArrayBuffer(0);
        expect(assertionToFinishBody({ rawId: e, clientDataJSON: e, authenticatorData: e, signature: e, userHandle: null }).userHandle).toBe('');
    });
    test('attestation', () => {
        const body = attestationToFinishBody('Laptop', {
            clientDataJSON: Uint8Array.from([1]).buffer as ArrayBuffer, attestationObject: Uint8Array.from([2, 3]).buffer as ArrayBuffer,
        });
        expect(body).toEqual({ name: 'Laptop', clientDataJSON: 'AQ', attestationObject: 'AgM' });
    });
});

describe('support and names', () => {
    test('passkeysSupported is false on an IP address, which can never own a passkey', () => {
        const api = { PublicKeyCredential: function () {}, isSecureContext: true };
        expect(passkeysSupported({ ...api, location: { hostname: '127.0.0.1' } })).toBe(false);
        expect(passkeysSupported({ ...api, location: { hostname: '192.168.1.20' } })).toBe(false);
        expect(passkeysSupported({ ...api, location: { hostname: '[::1]' } })).toBe(false);
        expect(passkeysSupported({ ...api, location: { hostname: 'localhost' } })).toBe(true);
        expect(passkeysSupported({ ...api, location: { hostname: 'dash.example.lan' } })).toBe(true);
    });
    test('passkeysSupported needs the API and a secure context', () => {
        expect(passkeysSupported({ PublicKeyCredential: function () {}, isSecureContext: true })).toBe(true);
        expect(passkeysSupported({ PublicKeyCredential: function () {}, isSecureContext: false })).toBe(false);
        expect(passkeysSupported({ isSecureContext: true })).toBe(false);
        expect(passkeysSupported({})).toBe(false);
    });
    test('defaultPasskeyName', () => {
        expect(defaultPasskeyName('Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0')).toBe('Firefox · Linux');
        expect(defaultPasskeyName('Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 Chrome/120 Safari/537.36 Edg/120')).toBe('Edge · Windows');
        expect(defaultPasskeyName('')).toBe('Passkey');
    });
});

import { describe, expect, test } from 'bun:test';
import { formatSecret, recoveryCodesText, looksLikeAppCode, normalizeCode } from './twoStep';

describe('formatSecret', () => {
    test('groups of four, the last one short', () => {
        expect(formatSecret('JBSWY3DPEHPK3PXP')).toBe('JBSW Y3DP EHPK 3PXP');
        expect(formatSecret('ABCDEFGHIJ')).toBe('ABCD EFGH IJ');
    });
    test('no trailing space on an exact multiple, and spaces already there are redone', () => {
        expect(formatSecret('ABCDEFGH')).toBe('ABCD EFGH');
        expect(formatSecret('AB CD EF')).toBe('ABCD EF');
        expect(formatSecret('')).toBe('');
    });
});

describe('recoveryCodesText', () => {
    test('one code per line with a final newline', () => {
        expect(recoveryCodesText(['aaaa-bbbb', 'cccc-dddd'])).toBe('aaaa-bbbb\ncccc-dddd\n');
    });
    test('nothing in, nothing out', () => {
        expect(recoveryCodesText([])).toBe('');
    });
});

describe('looksLikeAppCode / normalizeCode', () => {
    test('six digits, with or without a space in the middle', () => {
        expect(looksLikeAppCode('123456')).toBe(true);
        expect(looksLikeAppCode('123 456')).toBe(true);
        expect(looksLikeAppCode('12345')).toBe(false);
        expect(looksLikeAppCode('k7fq-2mzx-9wpr-h4tn')).toBe(false);
        expect(looksLikeAppCode('1234567')).toBe(false);
    });
    test('an app code loses its spaces; a recovery code is only trimmed', () => {
        expect(normalizeCode(' 123 456 ')).toBe('123456');
        expect(normalizeCode(' k7fq-2mzx-9wpr-h4tn ')).toBe('k7fq-2mzx-9wpr-h4tn');
    });
});

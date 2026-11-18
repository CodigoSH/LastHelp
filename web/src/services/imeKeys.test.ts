import { describe, expect, test } from 'bun:test';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { isComposing, isEnter } from './imeKeys';

const key = (k: string, extra: { isComposing?: boolean; keyCode?: number } = {}) => ({ key: k, isComposing: false, keyCode: 0, ...extra });

describe('Enter, for a person who types with an input method', () => {
    test('a plain Enter is Enter', () => {
        expect(isEnter(key('Enter', { keyCode: 13 }))).toBe(true);
    });

    test('the Enter that confirms what is being composed is not (Chrome and Firefox: the composition is still open)', () => {
        expect(isEnter(key('Enter', { isComposing: true, keyCode: 229 }))).toBe(false);
        expect(isEnter(key('Enter', { isComposing: true, keyCode: 13 }))).toBe(false);
    });

    test('nor is the one Safari sends after it has closed the composition: only its keyCode of 229 says so', () => {
        expect(isEnter(key('Enter', { isComposing: false, keyCode: 229 }))).toBe(false);
    });

    test('other keys are not Enter, composing or not', () => {
        for (const k of ['a', ' ', 'Tab', 'Escape', 'NumpadEnter', 'enter', '']) expect(isEnter(key(k, { keyCode: 13 }))).toBe(false);
    });

    test('what counts as composing', () => {
        expect(isComposing({ isComposing: true, keyCode: 65 })).toBe(true);
        expect(isComposing({ isComposing: false, keyCode: 229 })).toBe(true);
        expect(isComposing({ isComposing: false, keyCode: 13 })).toBe(false);
        // A browser that leaves the fields out is not composing.
        expect(isComposing({} as KeyboardEvent)).toBe(false);
    });
});

/** Every .ts file under web/src that is the app's own code: not a test, and not the one place that may ask. */
function sources(): { file: string; text: string }[] {
    const root = join(import.meta.dir, '..');
    return (readdirSync(root, { recursive: true }) as string[])
        .filter((f) => f.endsWith('.ts') && !f.endsWith('.test.ts') && !f.endsWith('.d.ts') && !f.endsWith('services/imeKeys.ts'))
        .map((f) => ({ file: f, text: readFileSync(join(root, f), 'utf8') }));
}

describe('nobody asks for Enter by hand', () => {
    // A listener that tests the key itself takes the Enter that confirms a Japanese, Chinese or Korean
    // word for "submit". They all ask imeKeys.isEnter, which tells the two apart.
    const raw = [
        /\bkey\s*[!=]==?\s*['"]Enter['"]/,          // event.key === 'Enter', key !== 'Enter'
        /\bcode\s*[!=]==?\s*['"](Numpad)?Enter['"]/, // event.code === 'Enter'
        /\b(keyCode|which|charCode)\s*[!=]==?\s*13\b/,
        /case\s+['"]Enter['"]\s*:/,
    ];

    test('the key is read through imeKeys, in every file', () => {
        const found: string[] = [];
        for (const { file, text } of sources()) {
            text.split('\n').forEach((line, i) => {
                if (line.trim().startsWith('//') || line.trim().startsWith('*')) return;
                if (raw.some((re) => re.test(line))) found.push(`${file}:${i + 1}  ${line.trim().slice(0, 90)}`);
            });
        }
        expect(found).toEqual([]);
    });

    test('and the check is able to see one (so an empty answer means something)', () => {
        const sample = "if (e.key === 'Enter') { submit(); }";
        expect(raw.some((re) => re.test(sample))).toBe(true);
        expect(raw.some((re) => re.test("if (isEnter(e)) { submit(); }"))).toBe(false);
        expect(sources().length).toBeGreaterThan(20);
    });
});

import { describe, expect, it } from 'bun:test';
import { byteCapacity, formatBits, qrMatrix, qrSvg, rsEncode, versionBits } from './qrCode';

const CAPACITY = [14, 26, 42, 62, 84, 106, 122, 152, 180, 213];
const text = (n: number) => 'a'.repeat(n);
const sizeOf = (s: string) => qrMatrix(s).length;

describe('Reed-Solomon', () => {
    it('matches the worked example of a 1-M symbol', () => {
        const data = [32, 91, 11, 120, 209, 114, 220, 77, 67, 64, 236, 17, 236, 17, 236, 17];
        expect(rsEncode(data, 10)).toEqual([196, 35, 39, 119, 235, 215, 231, 226, 93, 23]);
    });

    it('leaves nothing behind for an empty block', () => {
        expect(rsEncode([0, 0, 0], 4)).toEqual([0, 0, 0, 0]);
    });
});

describe('format and version information', () => {
    it('gives the table values for level M and each mask', () => {
        const expected = [
            '101010000010010', '101000100100101', '101111001111100', '101101101001011',
            '100010111111001', '100000011001110', '100111110010111', '100101010100000',
        ];
        for (let mask = 0; mask < 8; mask++) {
            expect(formatBits(mask).toString(2).padStart(15, '0')).toBe(expected[mask]!);
        }
    });

    it('gives the table values for versions 7 and 10', () => {
        expect(versionBits(7)).toBe(0x07c94);
        expect(versionBits(10)).toBe(0x0a4d3);
    });
});

describe('capacity and version selection', () => {
    it('knows the byte capacity of versions 1 to 10 at level M', () => {
        expect([1, 2, 3, 4, 5, 6, 7, 8, 9, 10].map(byteCapacity)).toEqual(CAPACITY);
    });

    it('uses the smallest version, and the next one a byte later', () => {
        for (let v = 1; v <= 10; v++) {
            expect(sizeOf(text(CAPACITY[v - 1]!))).toBe(21 + 4 * (v - 1));
            if (v < 10) expect(sizeOf(text(CAPACITY[v - 1]! + 1))).toBe(21 + 4 * v);
        }
    });

    it('counts UTF-8 bytes, not characters', () => {
        expect(sizeOf('ñ'.repeat(8))).toBe(25);   // 16 bytes: version 2
        expect(sizeOf('a'.repeat(8))).toBe(21);
    });

    it('refuses what does not fit in version 10', () => {
        expect(() => qrMatrix(text(214))).toThrow('qr: text too long');
        expect(() => qrMatrix(text(213))).not.toThrow();
    });
});

describe('function patterns', () => {
    const finder = (m: boolean[][], r: number, c: number) => {
        for (let dr = 0; dr < 7; dr++) {
            for (let dc = 0; dc < 7; dc++) {
                const ring = Math.max(Math.abs(dr - 3), Math.abs(dc - 3));
                expect(m[r + dr]![c + dc]!).toBe(ring !== 2);
            }
        }
    };

    it('draws a finder pattern in three corners, and not in the fourth', () => {
        for (const s of ['A', text(60), text(213)]) {
            const m = qrMatrix(s);
            const n = m.length;
            finder(m, 0, 0);
            finder(m, 0, n - 7);
            finder(m, n - 7, 0);
            expect(m[n - 1]![n - 1]! === m[n - 1]![n - 2]! && m[n - 1]![n - 1]! === m[n - 2]![n - 1]! && m[n - 2]![n - 2]! === m[n - 1]![n - 1]!).toBe(false);
        }
    });

    it('keeps the separators light, the timing alternating and the dark module dark', () => {
        const m = qrMatrix(text(100));
        const n = m.length;
        for (let i = 0; i < 8; i++) {
            expect(m[7]![i]!).toBe(false);
            expect(m[i]![7]!).toBe(false);
            expect(m[7]![n - 1 - i]!).toBe(false);
            expect(m[n - 8]![i]!).toBe(false);
        }
        for (let i = 8; i < n - 8; i++) {
            expect(m[6]![i]!).toBe(i % 2 === 0);
            expect(m[i]![6]!).toBe(i % 2 === 0);
        }
        expect(m[n - 8]![8]!).toBe(true);
    });

    it('draws an alignment pattern from version 2', () => {
        const m = qrMatrix(text(20)); // version 2: centre at (18, 18)
        for (let dr = -2; dr <= 2; dr++) {
            for (let dc = -2; dc <= 2; dc++) expect(m[18 + dr]![18 + dc]!).toBe(Math.max(Math.abs(dr), Math.abs(dc)) !== 1);
        }
    });

    it('writes the same format information twice, and it is one of the eight', () => {
        for (const s of ['A', text(20), text(150)]) {
            const m = qrMatrix(s);
            const n = m.length;
            const a: boolean[] = [];
            for (let i = 0; i <= 5; i++) a.push(m[i]![8]!);
            a.push(m[7]![8]!, m[8]![8]!, m[8]![7]!);
            for (let i = 9; i < 15; i++) a.push(m[8]![14 - i]!);
            const b: boolean[] = [];
            for (let i = 0; i < 8; i++) b.push(m[8]![n - 1 - i]!);
            for (let i = 8; i < 15; i++) b.push(m[n - 15 + i]![8]!);
            expect(b).toEqual(a);
            const value = a.reduce((acc, bit, i) => acc | ((bit ? 1 : 0) << i), 0);
            expect([0, 1, 2, 3, 4, 5, 6, 7].map(formatBits)).toContain(value);
        }
    });

    it('writes version information from version 7, both copies, and not before', () => {
        const readCopies = (m: boolean[][]) => {
            const n = m.length;
            let top = 0;
            let left = 0;
            for (let i = 0; i < 18; i++) {
                if (m[Math.floor(i / 3)]![n - 11 + (i % 3)]!) top |= 1 << i;
                if (m[n - 11 + (i % 3)]![Math.floor(i / 3)]!) left |= 1 << i;
            }
            return [top, left];
        };
        expect(readCopies(qrMatrix(text(CAPACITY[6]!)))).toEqual([versionBits(7), versionBits(7)]);
        expect(readCopies(qrMatrix(text(CAPACITY[9]!)))).toEqual([versionBits(10), versionBits(10)]);
        // Version 6 has data where version 7 has its information, so it must not match.
        const six = readCopies(qrMatrix(text(CAPACITY[5]!)));
        expect(six[0] === versionBits(6) && six[1] === versionBits(6)).toBe(false);
    });
});

describe('qrMatrix', () => {
    it('is deterministic and square', () => {
        const m = qrMatrix('otpauth://totp/Acme:admin?secret=JBSWY3DPEHPK3PXP&issuer=Acme');
        expect(qrMatrix('otpauth://totp/Acme:admin?secret=JBSWY3DPEHPK3PXP&issuer=Acme')).toEqual(m);
        for (const row of m) expect(row.length).toBe(m.length);
    });

    it('differs when the text does', () => {
        expect(qrMatrix('hello')).not.toEqual(qrMatrix('hellp'));
    });
});

describe('qrSvg', () => {
    const m = qrMatrix('hello');
    const svg = qrSvg(m);

    it('has a viewBox in modules with a quiet zone of 4, a background and one path', () => {
        const span = m.length + 8;
        expect(svg).toContain(`viewBox="0 0 ${span} ${span}"`);
        expect(svg).toContain('width="200" height="200"');
        expect(svg).toContain('shape-rendering="crispEdges"');
        expect(svg.match(/<rect /g)?.length).toBe(1);
        expect(svg.match(/<path /g)?.length).toBe(1);
        expect(svg).toContain('fill="#000"');
    });

    it('honours the quiet zone and size options', () => {
        const custom = qrSvg(m, { quiet: 2, size: 300 });
        expect(custom).toContain(`viewBox="0 0 ${m.length + 4} ${m.length + 4}"`);
        expect(custom).toContain('width="300" height="300"');
    });

    it('covers exactly the dark modules', () => {
        const d = /<path d="([^"]*)"/.exec(svg)![1]!;
        let area = 0;
        for (const run of d.matchAll(/M\d+ \d+h(\d+)v1h-\d+z/g)) area += Number(run[1]);
        expect(area).toBe(m.flat().filter(Boolean).length);
    });

    it('carries no style attribute and no script', () => {
        expect(svg).not.toContain('style=');
        expect(svg).not.toContain('<script');
        expect(qrSvg(qrMatrix(text(213)))).not.toContain('style=');
    });
});

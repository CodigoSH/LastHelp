/**
 * A QR Code encoder, written from the ISO 18004 rules and nothing else: byte mode,
 * error correction level M, versions 1 to 10. It exists to draw the otpauth:// URI of
 * two-factor setup, so it is small on purpose and brings no dependency.
 *
 * Matrices are indexed [row][column]; true is a dark module.
 */

/** Per version at level M: error-correction codewords per block and the blocks as [count, data codewords]. */
const BLOCKS_M: ReadonlyArray<{ ec: number; groups: ReadonlyArray<[number, number]> }> = [
    { ec: 10, groups: [[1, 16]] },             // 1
    { ec: 16, groups: [[1, 28]] },             // 2
    { ec: 26, groups: [[1, 44]] },             // 3
    { ec: 18, groups: [[2, 32]] },             // 4
    { ec: 24, groups: [[2, 43]] },             // 5
    { ec: 16, groups: [[4, 27]] },             // 6
    { ec: 18, groups: [[4, 31]] },             // 7
    { ec: 22, groups: [[2, 38], [2, 39]] },    // 8
    { ec: 22, groups: [[3, 36], [2, 37]] },    // 9
    { ec: 26, groups: [[4, 43], [1, 44]] },    // 10
];

/** Alignment pattern centres per version (index 0 is version 1, which has none). */
const ALIGNMENT: ReadonlyArray<ReadonlyArray<number>> = [
    [], [6, 18], [6, 22], [6, 26], [6, 30], [6, 34], [6, 22, 38], [6, 24, 42], [6, 26, 46], [6, 28, 50],
];

const MAX_VERSION = BLOCKS_M.length;

function dataCodewords(version: number): number {
    return BLOCKS_M[version - 1]!.groups.reduce((sum, [count, size]) => sum + count * size, 0);
}

/** Width of the character-count field in byte mode: 8 bits up to version 9, 16 from version 10. */
function countBits(version: number): number {
    return version < 10 ? 8 : 16;
}

/** The most bytes that fit in a version at level M. */
export function byteCapacity(version: number): number {
    return Math.floor((dataCodewords(version) * 8 - 4 - countBits(version)) / 8);
}

// ---- Reed-Solomon over GF(256), polynomial x^8 + x^4 + x^3 + x^2 + 1 ----

const EXP = new Uint8Array(512);
const LOG = new Uint8Array(256);
(() => {
    let x = 1;
    for (let i = 0; i < 255; i++) {
        EXP[i] = x;
        LOG[x] = i;
        x <<= 1;
        if (x & 0x100) x ^= 0x11d;
    }
    for (let i = 255; i < 512; i++) EXP[i] = EXP[i - 255]!;
})();

function gfMul(a: number, b: number): number {
    return a === 0 || b === 0 ? 0 : EXP[LOG[a]! + LOG[b]!]!;
}

/** The generator polynomial with roots 2^0 .. 2^(degree-1), highest power first, leading 1 left out. */
function generator(degree: number): number[] {
    let poly = [1];
    for (let i = 0; i < degree; i++) {
        const next = new Array<number>(poly.length + 1).fill(0);
        for (let j = 0; j < poly.length; j++) {
            next[j]! ^= poly[j]!;
            next[j + 1]! ^= gfMul(poly[j]!, EXP[i]!);
        }
        poly = next;
    }
    return poly.slice(1);
}

/** The `ecLen` error-correction codewords of a block of data codewords. */
export function rsEncode(data: ReadonlyArray<number>, ecLen: number): number[] {
    const gen = generator(ecLen);
    const rem = new Array<number>(ecLen).fill(0);
    for (const byte of data) {
        const factor = byte ^ rem[0]!;
        rem.shift();
        rem.push(0);
        for (let i = 0; i < ecLen; i++) rem[i]! ^= gfMul(gen[i]!, factor);
    }
    return rem;
}

// ---- Format and version information ----

/** The 15 format bits for level M and a mask: 5 data bits, 10 BCH bits, XOR 0x5412. */
export function formatBits(mask: number): number {
    const data = mask; // level M is 00, so the five data bits are 00 + the mask
    let rem = data;
    for (let i = 0; i < 10; i++) rem = (rem << 1) ^ ((rem >>> 9) * 0x537);
    return ((data << 10) | rem) ^ 0x5412;
}

/** The 18 version bits: 6 version bits and 12 BCH bits. */
export function versionBits(version: number): number {
    let rem = version;
    for (let i = 0; i < 12; i++) rem = (rem << 1) ^ ((rem >>> 11) * 0x1f25);
    return (version << 12) | rem;
}

// ---- The codeword stream ----

function utf8(text: string): number[] {
    return Array.from(new TextEncoder().encode(text));
}

function pickVersion(length: number): number {
    for (let v = 1; v <= MAX_VERSION; v++) if (length <= byteCapacity(v)) return v;
    throw new Error('qr: text too long');
}

/** Header, bytes, terminator and padding, as exactly the data codewords of the version. */
function dataStream(bytes: number[], version: number): number[] {
    const bits: number[] = [];
    const put = (value: number, width: number) => {
        for (let i = width - 1; i >= 0; i--) bits.push((value >>> i) & 1);
    };
    put(0b0100, 4);
    put(bytes.length, countBits(version));
    for (const b of bytes) put(b, 8);

    const capacityBits = dataCodewords(version) * 8;
    put(0, Math.min(4, capacityBits - bits.length));
    while (bits.length % 8 !== 0) bits.push(0);

    const out: number[] = [];
    for (let i = 0; i < bits.length; i += 8) {
        let byte = 0;
        for (let j = 0; j < 8; j++) byte = (byte << 1) | bits[i + j]!;
        out.push(byte);
    }
    for (let pad = 0xec; out.length < dataCodewords(version); pad ^= 0xec ^ 0x11) out.push(pad);
    return out;
}

/** Split into blocks, add error correction to each, and interleave. */
function interleave(data: number[], version: number): number[] {
    const { ec, groups } = BLOCKS_M[version - 1]!;
    const blocks: number[][] = [];
    let pos = 0;
    for (const [count, size] of groups) {
        for (let i = 0; i < count; i++) {
            blocks.push(data.slice(pos, pos + size));
            pos += size;
        }
    }
    const ecBlocks = blocks.map((b) => rsEncode(b, ec));
    const out: number[] = [];
    const longest = Math.max(...blocks.map((b) => b.length));
    for (let i = 0; i < longest; i++) for (const b of blocks) if (i < b.length) out.push(b[i]!);
    for (let i = 0; i < ec; i++) for (const b of ecBlocks) out.push(b[i]!);
    return out;
}

// ---- The matrix ----

const MASKS: ReadonlyArray<(r: number, c: number) => boolean> = [
    (r, c) => (r + c) % 2 === 0,
    (r) => r % 2 === 0,
    (_r, c) => c % 3 === 0,
    (r, c) => (r + c) % 3 === 0,
    (r, c) => (Math.floor(r / 2) + Math.floor(c / 3)) % 2 === 0,
    (r, c) => ((r * c) % 2) + ((r * c) % 3) === 0,
    (r, c) => (((r * c) % 2) + ((r * c) % 3)) % 2 === 0,
    (r, c) => (((r + c) % 2) + ((r * c) % 3)) % 2 === 0,
];

class Grid {
    readonly size: number;
    readonly dark: boolean[][];
    /** Function modules (patterns and reserved areas): data never goes there and masks never touch them. */
    readonly fixed: boolean[][];

    constructor(readonly version: number) {
        this.size = 17 + 4 * version;
        this.dark = Array.from({ length: this.size }, () => new Array<boolean>(this.size).fill(false));
        this.fixed = Array.from({ length: this.size }, () => new Array<boolean>(this.size).fill(false));
        this.drawPatterns();
    }

    private set(r: number, c: number, value: boolean): void {
        this.dark[r]![c] = value;
        this.fixed[r]![c] = true;
    }

    private drawPatterns(): void {
        const n = this.size;
        for (let i = 0; i < n; i++) {
            this.set(6, i, i % 2 === 0);   // timing patterns
            this.set(i, 6, i % 2 === 0);
        }
        for (const [r, c] of [[0, 0], [0, n - 7], [n - 7, 0]] as const) this.finder(r, c);

        const pos = ALIGNMENT[this.version - 1]!;
        for (let i = 0; i < pos.length; i++) {
            for (let j = 0; j < pos.length; j++) {
                const corner = (i === 0 && j === 0) || (i === 0 && j === pos.length - 1) || (i === pos.length - 1 && j === 0);
                if (!corner) this.alignment(pos[i]!, pos[j]!);
            }
        }

        this.drawFormat(0);   // reserves the areas; rewritten once the mask is chosen
        if (this.version >= 7) {
            const bits = versionBits(this.version);
            for (let i = 0; i < 18; i++) {
                const on = ((bits >>> i) & 1) === 1;
                const a = n - 11 + (i % 3);
                const b = Math.floor(i / 3);
                this.set(b, a, on);
                this.set(a, b, on);
            }
        }
    }

    /** A finder pattern with its light separator, whose top-left module is (r, c). */
    private finder(r: number, c: number): void {
        for (let dr = -1; dr <= 7; dr++) {
            for (let dc = -1; dc <= 7; dc++) {
                const rr = r + dr;
                const cc = c + dc;
                if (rr < 0 || cc < 0 || rr >= this.size || cc >= this.size) continue;
                const ring = Math.max(Math.abs(dr - 3), Math.abs(dc - 3));
                this.set(rr, cc, ring !== 2 && ring !== 4);
            }
        }
    }

    private alignment(r: number, c: number): void {
        for (let dr = -2; dr <= 2; dr++) {
            for (let dc = -2; dc <= 2; dc++) this.set(r + dr, c + dc, Math.max(Math.abs(dr), Math.abs(dc)) !== 1);
        }
    }

    /** Both copies of the format information, plus the always-dark module. */
    drawFormat(mask: number): void {
        const bits = formatBits(mask);
        const bit = (i: number) => ((bits >>> i) & 1) === 1;
        const n = this.size;
        for (let i = 0; i <= 5; i++) this.set(i, 8, bit(i));
        this.set(7, 8, bit(6));
        this.set(8, 8, bit(7));
        this.set(8, 7, bit(8));
        for (let i = 9; i < 15; i++) this.set(8, 14 - i, bit(i));
        for (let i = 0; i < 8; i++) this.set(8, n - 1 - i, bit(i));
        for (let i = 8; i < 15; i++) this.set(n - 15 + i, 8, bit(i));
        this.set(n - 8, 8, true);
    }

    /** Zigzag over the free modules in pairs of columns from the right, skipping the timing column. */
    placeData(codewords: number[], remainder: number): void {
        const total = codewords.length * 8 + remainder;
        let i = 0;
        for (let right = this.size - 1; right >= 1; right -= 2) {
            if (right === 6) right = 5;
            const upward = ((this.size - 1 - right) & 2) === 0;
            for (let step = 0; step < this.size; step++) {
                const r = upward ? this.size - 1 - step : step;
                for (let k = 0; k < 2; k++) {
                    const c = right - k;
                    if (this.fixed[r]![c] || i >= total) continue;
                    this.dark[r]![c] = i < codewords.length * 8 && ((codewords[i >>> 3]! >>> (7 - (i & 7))) & 1) === 1;
                    i++;
                }
            }
        }
    }

    applyMask(mask: number): void {
        for (let r = 0; r < this.size; r++) {
            for (let c = 0; c < this.size; c++) if (!this.fixed[r]![c] && MASKS[mask]!(r, c)) this.dark[r]![c] = !this.dark[r]![c];
        }
    }
}

/** The four penalty rules of ISO 18004 section 7.8.3, summed. */
function penalty(m: boolean[][]): number {
    const n = m.length;
    let score = 0;

    // Rule 1: runs of five or more of one colour, in rows and in columns.
    for (const byRow of [true, false]) {
        for (let a = 0; a < n; a++) {
            let run = 1;
            for (let b = 1; b < n; b++) {
                const same = (byRow ? m[a]![b] : m[b]![a]) === (byRow ? m[a]![b - 1] : m[b - 1]![a]);
                run = same ? run + 1 : 1;
                if (run === 5) score += 3;
                else if (run > 5) score += 1;
            }
        }
    }

    // Rule 2: 2x2 blocks of one colour.
    for (let r = 0; r < n - 1; r++) {
        for (let c = 0; c < n - 1; c++) {
            const v = m[r]![c];
            if (v === m[r]![c + 1] && v === m[r + 1]![c] && v === m[r + 1]![c + 1]) score += 3;
        }
    }

    // Rule 3: the finder-like 1:1:3:1:1 pattern with four light modules on one side.
    const a = [true, false, true, true, true, false, true, false, false, false, false];
    const b = [false, false, false, false, true, false, true, true, true, false, true];
    for (const byRow of [true, false]) {
        for (let x = 0; x < n; x++) {
            for (let y = 0; y + 11 <= n; y++) {
                let ma = true;
                let mb = true;
                for (let k = 0; k < 11 && (ma || mb); k++) {
                    const v = byRow ? m[x]![y + k] : m[y + k]![x];
                    if (v !== a[k]) ma = false;
                    if (v !== b[k]) mb = false;
                }
                if (ma) score += 40;
                if (mb) score += 40;
            }
        }
    }

    // Rule 4: how far the share of dark modules is from half, in steps of 5 %.
    let dark = 0;
    for (const row of m) for (const v of row) if (v) dark++;
    const total = n * n;
    score += (Math.ceil(Math.abs(dark * 20 - total * 10) / total) - 1) * 10;
    return score;
}

/** The module matrix of `text` in byte mode at level M. Throws `qr: text too long` past version 10. */
export function qrMatrix(text: string): boolean[][] {
    const bytes = utf8(text);
    const version = pickVersion(bytes.length);
    const codewords = interleave(dataStream(bytes, version), version);
    const remainder = version === 1 ? 0 : version <= 6 ? 7 : 0;

    let best: boolean[][] | null = null;
    let bestScore = Infinity;
    for (let mask = 0; mask < 8; mask++) {
        const grid = new Grid(version);
        grid.placeData(codewords, remainder);
        grid.applyMask(mask);
        grid.drawFormat(mask);
        const score = penalty(grid.dark);
        if (score < bestScore) {
            bestScore = score;
            best = grid.dark;
        }
    }
    return best as boolean[][];
}

/**
 * The matrix as an SVG string: a white background and one path with a run per row of
 * dark modules. No style attribute and no script, so it passes the page's CSP.
 */
export function qrSvg(matrix: boolean[][], opts: { quiet?: number; size?: number } = {}): string {
    const quiet = opts.quiet ?? 4;
    const size = opts.size ?? 200;
    const span = matrix.length + quiet * 2;
    let d = '';
    for (let r = 0; r < matrix.length; r++) {
        let c = 0;
        while (c < matrix.length) {
            if (!matrix[r]![c]) { c++; continue; }
            const start = c;
            while (c < matrix.length && matrix[r]![c]) c++;
            const run = c - start;
            d += `M${start + quiet} ${r + quiet}h${run}v1h-${run}z`;
        }
    }
    return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${span} ${span}" width="${size}" height="${size}" shape-rendering="crispEdges">`
        + `<rect width="${span}" height="${span}" fill="#fff"/><path d="${d}" fill="#000"/></svg>`;
}

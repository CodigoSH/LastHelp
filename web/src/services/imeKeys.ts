/**
 * The Enter key, for a person who types with an input method.
 *
 * Japanese, Chinese and Korean (and others) are typed in two steps: the keys make a candidate
 * text that is still being composed, and Enter, or the space bar, confirms it. That Enter is a
 * key press like any other as far as a `keydown` listener can tell, and a listener that took it
 * for "submit" sent the form, advanced the wizard or ran the list logic of a note while the
 * person was still choosing between two kanji.
 *
 * The browser says it two ways, and neither is enough alone: `isComposing` is true while the
 * composition is open (Chrome, Firefox), and Safari closes it a moment BEFORE the Enter that
 * confirmed it arrives, so that one is flagged only by its `keyCode` of 229, the code that means
 * "the input method took this key".
 *
 * Everything in the app that asks whether Enter was pressed asks here (a test keeps it so).
 */

/** The parts of a keyboard event this needs, so a test can hand in a plain object. */
export type KeyLike = Pick<KeyboardEvent, 'key' | 'isComposing' | 'keyCode'>;

/** Whether the key belongs to a composition in progress, or to the one it has just confirmed. */
export function isComposing(e: Pick<KeyboardEvent, 'isComposing' | 'keyCode'>): boolean {
    return e.isComposing === true || e.keyCode === 229;
}

/** Enter, pressed on purpose: not the key that picks a candidate or confirms what is being composed. */
export function isEnter(e: KeyLike): boolean {
    return e.key === 'Enter' && !isComposing(e);
}

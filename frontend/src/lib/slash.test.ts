import { describe, expect, it } from 'vitest';
import { findSlash } from './slash';

describe('findSlash', () => {
  it('opens at the start of a line', () => {
    expect(findSlash('/', 1)).toEqual({ at: 0, query: '' });
    expect(findSlash('/tod', 4)).toEqual({ at: 0, query: 'tod' });
  });

  it('opens after whitespace, mid-line', () => {
    expect(findSlash('note /tab', 9)).toEqual({ at: 5, query: 'tab' });
  });

  it('finds the token on the CURRENT line, not an earlier one', () => {
    const text = 'first /h1\nsecond /h2';
    expect(findSlash(text, text.length)).toEqual({ at: 17, query: 'h2' });
  });

  // The rule that earns its keep: a URL is mostly slashes.
  it('ignores slashes inside a word', () => {
    expect(findSlash('https://example.com/x', 21)).toBeNull();
    expect(findSlash('and/or', 6)).toBeNull();
    expect(findSlash('a//b', 4)).toBeNull();
  });

  it('closes once the query breaks', () => {
    expect(findSlash('/todo now', 9)).toBeNull(); // whitespace ended it
    expect(findSlash('/a/b', 4)).toBeNull(); // a second slash ended it
  });

  it('reads from the caret, not the end of the text', () => {
    // caret sits right after "/h", with more text beyond it
    expect(findSlash('/h and more', 2)).toEqual({ at: 0, query: 'h' });
  });
});

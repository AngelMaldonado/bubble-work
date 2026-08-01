import { describe, expect, it } from 'vitest';
import { fuzzyScore, fuzzyFilter } from './fuzzy';

describe('fuzzyScore', () => {
  it('matches subsequences', () => {
    expect(fuzzyScore('sgn', 'signup')).toBeGreaterThanOrEqual(0);
    expect(fuzzyScore('xyz', 'signup')).toBe(-1);
  });

  it('ranks consecutive/prefix higher', () => {
    expect(fuzzyScore('sign', 'signup')).toBeGreaterThan(fuzzyScore('sign', 'assigning'));
  });

  it('empty query matches everything', () => {
    expect(fuzzyScore('', 'anything')).toBe(0);
  });
});

describe('fuzzyFilter', () => {
  it('orders by score and drops non-matches', () => {
    const items = ['Onboarding flow', 'Billing revamp', 'Infra'];
    const out = fuzzyFilter('bil', items, (s) => s);
    expect(out[0]).toBe('Billing revamp');
    expect(out).not.toContain('Infra');
  });
});

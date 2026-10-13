import { expect, test } from 'bun:test';
import { parse, plannerUrl, readonlyInboxUrl } from '../src/lib/routes';

test('explicit Inbox route requests read-only even for a lead', () => {
  expect(parse(readonlyInboxUrl)).toEqual({ kind: 'planner', inboxOnly: true });
});

test('ordinary planner keeps its role-based route', () => {
  expect(parse(plannerUrl)).toEqual({ kind: 'planner' });
});

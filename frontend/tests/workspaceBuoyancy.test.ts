import { expect,test } from 'bun:test';
import { workspaceBuoyancy } from '../src/lib/workspaceBuoyancy';
import type { BubbleHeat, Lifecycle } from '../src/lib/api';
function bubble(workspace:string,life:Lifecycle,at=''):BubbleHeat { return {workspace,warm_at:at,heat:{lifecycle:life}} as BubbleHeat; }
test('workspace illustration takes its hottest bubble, not its initial',() => {
 const out=workspaceBuoyancy({bubbles:[bubble('a','closed'),bubble('a','rip'),bubble('a','dormant'),bubble('a','hot'),bubble('b','dormant')]});
 expect(out.a.life).toBe('hot');expect(out.b.life).toBe('dormant');
});
test('production recency is independent of the hottest band',() => {
 const out=workspaceBuoyancy({bubbles:[bubble('a','hot','2026-10-01T00:00:00Z'),bubble('a','closed','2026-10-03T00:00:00Z')]});
 expect(out.a.life).toBe('hot');expect(out.a.at).toBe('2026-10-03T00:00:00Z');
 expect(workspaceBuoyancy({bubbles:[]})).toEqual({});
});

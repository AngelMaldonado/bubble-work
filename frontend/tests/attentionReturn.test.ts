import { beforeEach, expect, test } from 'bun:test';
import { rememberAttentionReturn, takeAttentionReturn } from '../src/lib/attentionReturn';
beforeEach(() => {
 const rows=new Map<string,string>();
 Object.defineProperty(globalThis,'sessionStorage',{configurable:true,value:{getItem:(key:string) => rows.get(key) ?? null,setItem:(key:string,v:string) => rows.set(key,v)}});
});
test('pin in another workspace returns to its actual origin',() => {
 rememberAttentionReturn('pin-user','/w/second','/w/first/t/1');
 expect(takeAttentionReturn('pin-user','/w/first/t/1')).toBe('/w/second');
 expect(takeAttentionReturn('pin-user','/w/first/t/1')).toBe('');
});
test('artifact then pinned thread unwinds both contexts',() => {
 rememberAttentionReturn('nested','/w/second','/artefacto/a');
 rememberAttentionReturn('nested','/artefacto/a','/w/first/t/2');
 expect(takeAttentionReturn('nested','/w/first/t/2')).toBe('/artefacto/a');
 expect(takeAttentionReturn('nested','/artefacto/a')).toBe('/w/second');
});
test('users and unrelated navigation do not inherit the prior return',() => {
 rememberAttentionReturn('isolation','/w/a','/artefacto/a');
 expect(takeAttentionReturn('other','/artefacto/a')).toBe('');
 rememberAttentionReturn('isolation','/w/b','/artefacto/b');
 expect(takeAttentionReturn('isolation','/artefacto/b')).toBe('/w/b');
 expect(takeAttentionReturn('isolation','/artefacto/a')).toBe('');
});
test('blocked storage has an in-memory fallback and rejects external URLs',() => {
 Object.defineProperty(globalThis,'sessionStorage',{configurable:true,value:{getItem:() => null,setItem:() => {throw Error('blocked');}}});
 rememberAttentionReturn('blocked','/w/second','/artefacto/a');
 expect(takeAttentionReturn('blocked','/artefacto/a')).toBe('/w/second');
 rememberAttentionReturn('blocked','https://external.invalid','/artefacto/a');
 expect(takeAttentionReturn('blocked','/artefacto/a')).toBe('');
});

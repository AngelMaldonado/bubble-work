import { afterEach, beforeEach, expect, test } from 'bun:test';
import { readTreeSelection, saveTreeSelection } from '../src/lib/treeSelection';
import { parse, wikiUrl } from '../src/lib/routes';
const original = globalThis.localStorage;
beforeEach(() => {
 const values = new Map<string,string>();
 Object.defineProperty(globalThis,'localStorage',{configurable:true,value:{getItem:(k:string)=>values.get(k)??null,setItem:(k:string,v:string)=>values.set(k,v)}});
});
afterEach(() => Object.defineProperty(globalThis,'localStorage',{configurable:true,value:original}));
test('sin memoria elige el primer archivo, no una carpeta',()=>{
 expect(readTreeSelection('u','thread.a',['doc','page:notes.md'])).toBe('doc');
 expect(readTreeSelection('u','empty',[])).toBe('');
});
test('recuerda ids disponibles y aísla usuario y árbol',()=>{
 saveTreeSelection('u','thread.a','page:notes.md');
 expect(readTreeSelection('u','thread.a',['doc','page:notes.md'])).toBe('page:notes.md');
 expect(readTreeSelection('other','thread.a',['doc','page:notes.md'])).toBe('doc');
 expect(readTreeSelection('u','thread.b',['doc','page:notes.md'])).toBe('doc');
});
test('una selección borrada o sin permiso vuelve al primer archivo',()=>{
 saveTreeSelection('u','prompts','agents/planner');
 expect(readTreeSelection('u','prompts',['agents/operators','guide'])).toBe('agents/operators');
});
test('storage bloqueado no rompe la navegación',()=>{
 Object.defineProperty(globalThis,'localStorage',{configurable:true,get(){throw new Error('blocked');}});
 expect(()=>saveTreeSelection('u','wiki','README.md')).not.toThrow();
 expect(readTreeSelection('u','wiki',['README.md'])).toBe('README.md');
});
test('wiki sin archivo permite resolver memoria, el enlace explícito conserva su destino',()=>{
 expect(parse(wikiUrl('alpha'))).toEqual({kind:'wiki',slug:'alpha',page:''});
 expect(parse(wikiUrl('alpha','docs/una decisión.md'))).toEqual({kind:'wiki',slug:'alpha',page:'docs/una decisión.md'});
});

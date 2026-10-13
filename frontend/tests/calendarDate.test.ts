import { expect, test } from 'bun:test';
import { shiftCalendarDate } from '../src/lib/calendarDate';
const parts=(d:Date)=>[d.getFullYear(),d.getMonth()+1,d.getDate()];
test('enero 31 pasa a febrero sin saltarse el mes',()=>{
 expect(parts(shiftCalendarDate(new Date(2026,0,31),1))).toEqual([2026,2,28]);
});
test('un salto de año conserva el día hasta donde existe',()=>{
 expect(parts(shiftCalendarDate(new Date(2024,1,29),12))).toEqual([2025,2,28]);
});
test('retroceder cruza años y no modifica la fecha de origen',()=>{
 const at=new Date(2026,0,31);
 expect(parts(shiftCalendarDate(at,-1))).toEqual([2025,12,31]);
 expect(parts(at)).toEqual([2026,1,31]);
});

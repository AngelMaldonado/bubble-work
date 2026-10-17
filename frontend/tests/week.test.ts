import { expect, test } from 'bun:test';
import { dayInWeek, startOfWeek, validWeekStart, weekNumber } from '../src/lib/week';
const parts = (d: Date) => [d.getFullYear(), d.getMonth() + 1, d.getDate()];

test('el inicio de semana sigue al día elegido', () => {
  const sat = new Date(2026, 9, 17); // sábado 17 oct 2026
  expect(parts(startOfWeek(sat, 1))).toEqual([2026, 10, 12]); // lunes
  expect(parts(startOfWeek(sat, 0))).toEqual([2026, 10, 11]); // domingo
  expect(parts(startOfWeek(sat, 6))).toEqual([2026, 10, 17]); // sábado: él mismo
  expect(dayInWeek(sat, 1)).toBe(5);
  expect(dayInWeek(sat, 0)).toBe(6);
  expect(dayInWeek(sat, 6)).toBe(0);
});

test('en lunes, la semana es la de ISO 8601', () => {
  expect(weekNumber(new Date(2026, 9, 12), 1)).toBe(42);
  expect(weekNumber(new Date(2026, 0, 1), 1)).toBe(1); // jueves: semana 1
  expect(weekNumber(new Date(2027, 0, 1), 1)).toBe(53); // viernes: aún la 53 de 2026
  expect(weekNumber(new Date(2024, 11, 30), 1)).toBe(1); // lunes que ya es de 2025
});

test('en otro día, la semana 1 es la que contiene el 1 de enero', () => {
  expect(weekNumber(new Date(2026, 0, 1), 0)).toBe(1);
  expect(weekNumber(new Date(2025, 11, 28), 0)).toBe(1); // domingo 28 dic: su semana lleva el 1 de enero
  expect(weekNumber(new Date(2026, 0, 4), 0)).toBe(2);
  expect(weekNumber(new Date(2026, 9, 11), 0)).toBe(42);
});

test('un valor raro en las preferencias vuelve al lunes', () => {
  expect(validWeekStart(0)).toBe(0);
  expect(validWeekStart(6)).toBe(6);
  expect(validWeekStart(7)).toBe(1);
  expect(validWeekStart('0')).toBe(1);
  expect(validWeekStart(undefined)).toBe(1);
});

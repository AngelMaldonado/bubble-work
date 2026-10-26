import { expect, test } from 'bun:test';
import { effectiveFeatures, teamFeatureOn } from '../src/lib/features';
import type { Features } from '../src/lib/api';

const g: Features = { id: 'f', sequence: true, roadmap: false, space: true };

test('un equipo que no dice nada hereda el departamento', () => {
  expect(teamFeatureOn(g, { features: {} }, 'sequence')).toBe(true);
  expect(teamFeatureOn(g, { features: { sequence: '' } }, 'roadmap')).toBe(false);
  expect(teamFeatureOn(g, undefined, 'sequence')).toBe(true);
  // un servidor viejo no manda `features`
  expect(teamFeatureOn(g, {}, 'sequence')).toBe(true);
});

test('lo que dice el equipo gana, en los dos sentidos', () => {
  expect(teamFeatureOn(g, { features: { sequence: 'off' } }, 'sequence')).toBe(false);
  expect(teamFeatureOn(g, { features: { roadmap: 'on' } }, 'roadmap')).toBe(true);
});

test('un equipo: lo suyo; Mi espacio sigue siendo del departamento', () => {
  const f = effectiveFeatures(g, [{ features: { sequence: 'off', roadmap: 'on' } }]);
  expect(f).toMatchObject({ sequence: false, roadmap: true, space: true, id: 'f' });
});

test('varios equipos (Todos los equipos): encendida si alguno la tiene', () => {
  const f = effectiveFeatures({ ...g, sequence: false }, [{ features: {} }, { features: { sequence: 'on' } }]);
  expect(f.sequence).toBe(true);
  expect(effectiveFeatures(g, [{ features: { roadmap: 'off' } }, { features: {} }]).roadmap).toBe(false);
});

test('sin equipos, el departamento', () => {
  expect(effectiveFeatures(g, [])).toMatchObject({ sequence: true, roadmap: false });
});

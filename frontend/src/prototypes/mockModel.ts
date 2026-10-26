// La capa del modelo propuesto sobre los datos inventados de /theme/mock: los
// mismos nombres y calores de `lib/mock.ts`, más tags, furia, hilos y
// relaciones. Sólo se carga en desarrollo; nada de esto existe en el producto.
import { bubbles } from '../lib/mock';
import type { Lifecycle } from '../lib/api';
import { projectLinks, type Group, type GraphLink, type Level, type MapKind, type ModelNode } from './modelFixtures';

export const mockGroups: Group[] = [
  { id: 'clientes', name: 'clientes' },
  { id: 'plataforma', name: 'plataforma' },
  { id: 'gestion', name: 'gestión' },
  { id: '', name: 'Sin tags' },
];

type Extra = { group: string; priority: string; level: Level; deadline: string; stagnation: string; threads: string[] };
const calm = 'Sin alerta: hay evidencia reciente.';
const extra: Record<string, Extra> = {
  'Rate limiting del portal': { group: 'plataforma', priority: 'P1', level: 'warning', deadline: 'Advertencia: el pico de fin de mes es en 3 días.', stagnation: calm, threads: ['Límite por cliente', 'Cola de reintentos', 'Tablero de latencia', 'Prueba de carga'] },
  'Facturación electrónica': { group: 'clientes', priority: 'P1', level: 'furious', deadline: 'Furiosa: el timbrado CFDI 4.0 venció ayer. Plazo externo inamovible.', stagnation: calm, threads: ['Timbrado CFDI 4.0', 'Catálogo SAT', 'Cancelaciones', 'Complemento de pagos', 'Pruebas con el PAC'] },
  'Migración a PocketBase': { group: 'plataforma', priority: 'P1', level: 'calm', deadline: 'Sin compromiso de fecha en el resultado.', stagnation: calm, threads: ['Esquema de migraciones', 'Reglas por workspace', 'Importar v0', 'Evidencia observada, no inferida', 'Respaldos combinados', 'Ensayo de restauración'] },
  'Onboarding de clientes': { group: 'clientes', priority: 'P2', level: 'calm', deadline: 'Sin alerta de plazo.', stagnation: calm, threads: ['Invitación inicial', 'Primer proyecto guiado', 'Guía de bienvenida'] },
  'Reportes de rentabilidad': { group: 'gestion', priority: 'P2', level: 'angry', deadline: 'Sin compromiso de fecha.', stagnation: 'Enojada: 14 días activos sin evidencia (prioridad media).', threads: ['Costo por producto', 'Margen mensual', 'Exportar a hoja'] },
  'Rediseño del catálogo': { group: '', priority: 'P3', level: 'calm', deadline: 'Sin compromiso de fecha.', stagnation: 'Sin alerta: recién activada.', threads: ['Mapa de categorías', 'Prototipo de navegación'] },
  'Integración con el ERP': { group: 'plataforma', priority: 'P2', level: 'angry', deadline: 'Sin compromiso de fecha.', stagnation: 'Enojada: 4 días sin evidencia. Sin responsable: decisión del lead.', threads: ['Maestro de artículos', 'Sincronizar precios', 'Conciliar existencias'] },
  'App móvil de campo': { group: 'clientes', priority: 'P2', level: 'warning', deadline: 'Advertencia: compromiso con el cliente en 2 días.', stagnation: 'Sin alerta de estancamiento.', threads: ['Cerrar orden en sitio', 'Firma del cliente', 'Modo sin conexión'] },
  'Deuda técnica del monolito': { group: 'plataforma', priority: 'P3', level: 'calm', deadline: 'Sin compromiso de fecha.', stagnation: 'Sin alerta: nadie la ha activado todavía.', threads: ['Separar despliegue', 'Quitar dependencias muertas'] },
  'Auditoría ISO 9001': { group: 'gestion', priority: 'P2', level: 'calm', deadline: 'Completada: fuera de presión activa; historia conservada.', stagnation: 'Completada: fuera de presión activa.', threads: ['Manual de calidad', 'Auditoría interna'] },
};

const slug = (s: string) => s.toLowerCase().replace(/[^a-z0-9]+/g, '-');
const tid = (bubble: string, i: number) => `${slug(bubble)}:${i}`;

export const mockBubbleNodes: ModelNode[] = bubbles.map((b) => {
  const e = extra[b.name];
  return {
    kind: 'bubbles', id: slug(b.name), name: b.name, team: 'a', project: 'Software · Cuby',
    group: e?.group ?? '', life: b.life, level: e?.level ?? 'calm', priority: e?.priority ?? 'P3',
    mine: b.people.includes('Angel'), deadline: e?.deadline ?? 'Sin compromiso de fecha.', stagnation: e?.stagnation ?? calm,
  };
});

// Sólo un hilo vencido de verdad: el compromiso del resultado no se copia a sus hilos.
const threadLevel: Record<string, [Level, string, string]> = {
  [tid('Facturación electrónica', 0)]: ['furious', 'Furioso: timbrado comprometido para ayer; plazo externo inamovible.', calm],
  [tid('Reportes de rentabilidad', 0)]: ['angry', 'Sin compromiso de fecha en este hilo.', 'Enojado: 14 días activos sin evidencia.'],
  [tid('Rate limiting del portal', 3)]: ['warning', 'Advertencia: la prueba de carga está comprometida en 3 días.', calm],
};

export const mockThreadNodes: ModelNode[] = mockBubbleNodes.flatMap((b) => (extra[b.name]?.threads ?? []).map((name, i) => {
  const id = tid(b.name, i);
  const life: Lifecycle = b.life === 'closed' ? 'closed' : b.life === 'rip' ? (i === 0 ? 'dormant' : 'rip') : i % 3 === 2 ? 'dormant' : b.life;
  const [level, deadline, stagnation] = threadLevel[id] ?? ['calm', life === 'closed' ? 'Completado: fuera de presión activa.' : 'Sin compromiso de fecha en este hilo.', life === 'closed' ? 'Completado: fuera de presión activa.' : 'Sin alerta de estancamiento.'];
  return { ...b, kind: 'threads' as const, id, name, life, level, deadline, stagnation, mine: b.mine && i % 2 === 0, bubbleId: b.id, bubbleName: b.name };
}));

const rel = (a: [string, number], b: [string, number], kind: GraphLink['kind'] = 'dependency'): GraphLink =>
  ({ source: tid(...a), target: tid(...b), kind, count: 1 });
const relations: GraphLink[] = [
  rel(['Migración a PocketBase', 0], ['Migración a PocketBase', 1]),
  rel(['Migración a PocketBase', 1], ['Migración a PocketBase', 2]),
  rel(['Migración a PocketBase', 4], ['Migración a PocketBase', 5]),
  rel(['Migración a PocketBase', 0], ['Rate limiting del portal', 1], 'related'),
  rel(['Rate limiting del portal', 0], ['Rate limiting del portal', 3]),
  rel(['Facturación electrónica', 1], ['Facturación electrónica', 0]),
  rel(['Facturación electrónica', 0], ['Facturación electrónica', 2]),
  rel(['Facturación electrónica', 0], ['Onboarding de clientes', 0], 'related'),
  rel(['Integración con el ERP', 0], ['Integración con el ERP', 1]),
  rel(['Integración con el ERP', 0], ['Rediseño del catálogo', 0], 'related'),
  rel(['Integración con el ERP', 2], ['Reportes de rentabilidad', 0], 'related'),
  rel(['Reportes de rentabilidad', 0], ['Reportes de rentabilidad', 1]),
  rel(['App móvil de campo', 0], ['App móvil de campo', 1]),
  rel(['Deuda técnica del monolito', 0], ['Rate limiting del portal', 2], 'related'),
  rel(['Auditoría ISO 9001', 0], ['Auditoría ISO 9001', 1]),
];

export const mockLinks = (kind: MapKind, visible: readonly ModelNode[]) => projectLinks(kind, visible, mockThreadNodes, relations);

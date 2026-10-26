import type { Lifecycle } from '../lib/api';
import { levels, rank, projectLinks, type GraphGroup, type GraphLink, type GraphNode, type Level, type MapKind } from '../lib/graph';

// Los tipos y la geometría son los del producto (`lib/graph.ts`); aquí sólo
// viven los datos inventados y lo que un fixture añade a cada punto.
export { levels, rank, projectLinks };
export type { GraphLink, Level, MapKind };
export type Group = GraphGroup;
export type ModelNode = GraphNode & {
  team: 'a' | 'b'; mine: boolean; deadline: string; stagnation: string; life: Lifecycle;
};
export const teamItems = [{ value: 'a', label: 'Equipo A' }, { value: 'b', label: 'Equipo B' }];
// Los blobs son tags: el mismo tag de dos equipos es el mismo blob.
export const groups: Group[] = [
  { id: 'clientes', name: 'clientes' },
  { id: 'infra', name: 'infra' },
  { id: '', name: 'Sin tags' },
];

// Datos inventados. Ningún nivel de furia procede de un evaluador del producto.
export const bubbles: ModelNode[] = [
  { kind: 'bubbles', id: 'integration', name: 'Integración', team: 'a', project: 'Proyecto Alfa', group: 'clientes', life: 'hot', level: 'warning', priority: 'P2', mine: true, deadline: 'Advertencia: la entrega está a 3 días.', stagnation: 'Sin alerta: hay evidencia reciente.' },
  { kind: 'bubbles', id: 'billing', name: 'Facturación', team: 'a', project: 'Proyecto Alfa', group: 'clientes', life: 'dormant', level: 'furious', priority: 'P1', mine: true, deadline: 'Furiosa: compromiso crítico vencido hace 1 día.', stagnation: 'Enojada: 48 h activas sin evidencia. La espera registrada ya se descontó.' },
  { kind: 'bubbles', id: 'backup', name: 'Respaldo', team: 'a', project: 'Proyecto Infra', group: 'infra', life: 'dormant', level: 'calm', priority: 'P3', mine: false, deadline: 'Sin compromiso de fecha; no se inventa un vencimiento.', stagnation: 'Sin alerta: 2 días activos; prioridad media.' },
  { kind: 'bubbles', id: 'onboarding', name: 'Incorporación', team: 'b', project: 'Proyecto Beta', group: 'clientes', life: 'hot', level: 'calm', priority: 'P2', mine: true, deadline: 'Sin alerta de plazo.', stagnation: 'Sin alerta: hay evidencia reciente.' },
  { kind: 'bubbles', id: 'network', name: 'Red', team: 'b', project: 'Proyecto Infra B', group: 'infra', life: 'rip', level: 'angry', priority: 'P2', mine: false, deadline: 'Sin compromiso de fecha.', stagnation: 'Enojada: 4 días sin evidencia. Sin asignados: decisión para el lead.' },
  { kind: 'bubbles', id: 'draft', name: 'Plan siguiente', team: 'b', project: 'Proyecto Beta', group: '', life: 'dormant', level: 'calm', priority: 'P4', mine: false, deadline: 'Sin compromiso de fecha.', stagnation: 'Sin alerta de estancamiento.' },
  { kind: 'bubbles', id: 'closed', name: 'Entrega anterior', team: 'a', project: 'Proyecto Alfa', group: 'clientes', life: 'closed', level: 'calm', priority: 'P2', mine: true, deadline: 'Completada: fuera de presión activa; historia conservada.', stagnation: 'Completada: fuera de presión activa.' },
];

const tasks: Record<string, string[]> = {
  integration: ['Contrato de API', 'Recibir webhook', 'Reintentos', 'Trazas de integración'],
  billing: ['Reglas de cobro', 'Calcular impuestos', 'Conciliar pagos', 'Exportar movimientos', 'Texto del recibo'],
  backup: ['Ensayar restauración', 'Rotar claves', 'Copia externa', 'Informe de integridad'],
  onboarding: ['Invitación inicial', 'Perfil de usuario', 'Primer proyecto', 'Guía de bienvenida'],
  network: ['Inventario de red', 'Segmentar red', 'Comprobar acceso', 'Revisar latencia'],
  draft: ['Recoger propuestas', 'Comparar opciones'],
  closed: ['Publicar entrega', 'Verificar recepción'],
};
export const threads: ModelNode[] = bubbles.flatMap((bubble) => (tasks[bubble.id] ?? []).map((name, i) => {
  const critical = bubble.id === 'billing' && i === 0;
  const closed = bubble.life === 'closed' || (bubble.id === 'billing' && i === 4);
  return {
    ...bubble, kind: 'threads' as const, id: `${bubble.id}:${i + 1}`, name,
    bubbleId: bubble.id, bubbleName: bubble.name,
    life: closed ? 'closed' as const : critical ? 'dormant' as const : i % 3 === 0 ? bubble.life : i % 2 ? 'hot' as const : 'dormant' as const,
    level: critical ? 'furious' as const : 'calm' as const,
    mine: i % 2 === 0 && bubble.life !== 'rip',
    // Una fecha del resultado NO se copia como compromiso ficticio a sus hilos.
    deadline: closed ? 'Completado: fuera de presión activa.' : critical ? 'Furioso: compromiso crítico vencido hace 1 día.' : 'Sin compromiso de fecha en este hilo.',
    stagnation: critical ? 'Enojado: 48 h activas sin evidencia.' : closed ? 'Completado: fuera de presión activa.' : 'Sin alerta de estancamiento en este fixture.',
  };
}));

const relations: GraphLink[] = [
  { source: 'integration:1', target: 'integration:2', kind: 'dependency', count: 1 },
  { source: 'integration:2', target: 'integration:3', kind: 'related', count: 1 },
  { source: 'billing:1', target: 'billing:3', kind: 'dependency', count: 1 },
  { source: 'integration:1', target: 'billing:1', kind: 'related', count: 1 },
  { source: 'backup:1', target: 'integration:2', kind: 'dependency', count: 1 },
  { source: 'backup:2', target: 'backup:3', kind: 'dependency', count: 1 },
  { source: 'billing:4', target: 'backup:4', kind: 'related', count: 1 },
  { source: 'onboarding:1', target: 'network:1', kind: 'related', count: 1 },
  { source: 'network:1', target: 'network:2', kind: 'dependency', count: 1 },
  { source: 'network:3', target: 'onboarding:3', kind: 'dependency', count: 1 },
  { source: 'draft:1', target: 'onboarding:2', kind: 'related', count: 1 },
];

/** Sólo relaciones declaradas y con ambos extremos visibles. En burbujas se
 * proyectan las relaciones de sus hilos; compartir un tag no crea aristas. */
export function visibleLinks(kind: MapKind, visible: readonly ModelNode[]): GraphLink[] {
  return projectLinks(kind, visible, threads, relations);
}

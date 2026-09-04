// The invented data both /theme and /theme/mock draw on.
//
// It lives here rather than inside either page because the two have to agree:
// a mock of the whole interface and a page of components are only worth looking
// at side by side if the same bubble has the same name and the same heat in
// both. Shaped like the real thing on purpose — a lopsided board with a few
// things burning and a long tail going cold, because a demo where every band
// holds three tidy items proves nothing.
import type { Lifecycle } from './api';
import type { TreeNode } from '../components/SideTree.svelte';
import FileTextIcon from '@lucide/svelte/icons/file-text';
import FolderIcon from '@lucide/svelte/icons/folder';
import BookOpenIcon from '@lucide/svelte/icons/book-open';
import ImageIcon from '@lucide/svelte/icons/image';
import PenToolIcon from '@lucide/svelte/icons/pen-tool';

// The alphabet is not the mock's to invent — it re-exports the real one.
export { bandOrder, bandFace, bandName } from './bands';
export const bandWhy: Record<string, string> = {
  hot: 'produjo algo dentro de la ventana del ciclo',
  dormant: 'en silencio, pero con alguien responsable',
  rip: 'en silencio, y sin nadie responsable',
  closed: 'alcanzó su resultado',
};

export type MockBubble = {
  name: string;
  outcome: string;
  life: Lifecycle;
  burning: number;
  owner: string;
  people: string[];
  why: string;
};

export const bubbles: MockBubble[] = [
  { name: 'Rate limiting del portal', outcome: 'El portal aguanta el pico de fin de mes', life: 'hot', burning: 3, owner: 'Angel', people: ['Angel', 'Bea', 'Caro'], why: bandWhy.hot },
  { name: 'Facturación electrónica', outcome: 'CFDI 4.0 timbrando sin intervención', life: 'hot', burning: 2, owner: 'Bea', people: ['Bea', 'Dani'], why: bandWhy.hot },
  { name: 'Migración a PocketBase', outcome: 'Plane deja de ser el registro', life: 'hot', burning: 5, owner: 'Angel', people: ['Angel', 'Caro', 'Dani', 'Eva', 'Fran'], why: bandWhy.hot },
  { name: 'Onboarding de clientes', outcome: 'Un cliente nuevo opera solo el día 1', life: 'hot', burning: 1, owner: 'Caro', people: ['Caro'], why: bandWhy.hot },
  { name: 'Reportes de rentabilidad', outcome: 'Cada producto dice si se paga solo', life: 'dormant', burning: 0, owner: 'Bea', people: ['Bea', 'Eva'], why: bandWhy.dormant },
  { name: 'Rediseño del catálogo', outcome: 'El catálogo se navega sin buscador', life: 'dormant', burning: 0, owner: 'Dani', people: ['Dani'], why: 'recién nacida; todavía sin producir' },
  { name: 'Integración con el ERP', outcome: 'Un solo maestro de artículos', life: 'rip', burning: 0, owner: '', people: [], why: 'quieta, y sin nadie responsable' },
  { name: 'App móvil de campo', outcome: 'El técnico cierra la orden en sitio', life: 'dormant', burning: 0, owner: 'Eva', people: ['Eva'], why: 'en silencio 2 ciclos o más' },
  { name: 'Deuda técnica del monolito', outcome: 'Se puede desplegar un viernes', life: 'rip', burning: 0, owner: '', people: ['Fran'], why: 'nunca produjo nada, y sin responsable' },
  { name: 'Auditoría ISO 9001', outcome: 'La auditoría pasa sin hallazgos mayores', life: 'closed', burning: 0, owner: 'Angel', people: ['Angel', 'Bea'], why: bandWhy.closed },
];

// Who is looking. The planner belongs to the STRATEGIC layer, so its button
// only exists for a lead — flip this to false to see what an executor sees.
export const me = { name: 'Angel', isLead: true };

// Who else is here right now. Presence is about the CURRENT moment, so the
// offline half of the team is not in this list at all — a row of grey avatars
// answers "who exists", which nobody asked.
export const online = [
  { name: 'Angel', hue: 12 },
  { name: 'Bea', hue: 210 },
  { name: 'Caro', hue: 150 },
  { name: 'Dani', hue: 280 },
  { name: 'Eva', hue: 45 },
];

// The projects the sidebar lists. A workspace IS a project here: the boundary
// for a body of work, and the thing you switch between all day.
export const projects = [
  { id: 'p1', name: 'Software · Cuby', bubbles: 10, cycle: 'quedan 6 d' },
  { id: 'p2', name: 'AYETec', bubbles: 4, cycle: 'quedan 2 d' },
  { id: 'p3', name: 'Laboratorio', bubbles: 2, cycle: 'quedan 11 d' },
];

export const drawerThreads = [
  { seq: 14, title: 'Evidencia observada, no inferida', lifecycle: 'hot' as Lifecycle, priority: 'P1', state: 'En curso', owner: 'Angel', age: 'hace 2 h' },
  { seq: 12, title: 'El árbol de markdown y sus commits', lifecycle: 'hot' as Lifecycle, priority: 'P1', state: 'En curso', owner: 'Bea', age: 'hace 6 h' },
  { seq: 9, title: 'Las reglas de frontera por workspace', lifecycle: 'dormant' as Lifecycle, priority: 'P2', state: 'En revisión', owner: 'Caro', age: 'hace 5 d' },
  { seq: 7, title: 'Portar el motor de markdown', lifecycle: 'dormant' as Lifecycle, state: 'Backlog', age: 'hace 12 d' },
  { seq: 4, title: 'Decidir qué pasa con los adjuntos binarios', lifecycle: 'rip' as Lifecycle, priority: 'P4', state: 'Backlog', age: 'hace 41 d' },
];

export const threadDoc = `# Evidencia observada, no inferida

## Por qué existe

v0 INFERÍA la evidencia: diffeaba hashes y contadores en cada barrido, porque
las escrituras pasaban en Plane. Diez ediciones en un ciclo eran un evento y la
autoría nunca llegaba.

## Qué es verdad cuando esté hecho

- [x] El PATCH escribe el evento en el momento en que ocurre
- [x] Con el actor que lo causó
- [x] Una escritura que no cambia bytes no registra nada
- [ ] La vista de evidencia lo muestra en el thread
- [ ] Y el board deja de necesitar explicación aparte

## Pasos

- [ ] Portar heat sobre el stream real
- [ ] Decidir qué pasa con docs/`;

// What the server would return: goldmark's output for the markdown above. The
// page never renders markdown itself — one renderer, server-side, so that every
// surface shows the identical thing.
export const threadHTML = `<h1 id="evidencia-observada-no-inferida">Evidencia observada, no inferida</h1>
<h2 id="por-que-existe">Por qué existe</h2>
<p>v0 <em>INFERÍA</em> la evidencia: diffeaba hashes y contadores en cada barrido,
porque las escrituras pasaban en Plane. Diez ediciones en un ciclo eran un evento
y la autoría nunca llegaba.</p>
<h2 id="que-es-verdad-cuando-este-hecho">Qué es verdad cuando esté hecho</h2>
<ul>
<li><input checked="" disabled="" type="checkbox"> El PATCH escribe el evento en el momento en que ocurre</li>
<li><input checked="" disabled="" type="checkbox"> Con el actor que lo causó</li>
<li><input checked="" disabled="" type="checkbox"> Una escritura que no cambia bytes no registra nada</li>
<li><input disabled="" type="checkbox"> La vista de evidencia lo muestra en el thread</li>
<li><input disabled="" type="checkbox"> Y el board deja de necesitar explicación aparte</li>
</ul>
<h2 id="pasos">Pasos</h2>
<ul>
<li><input disabled="" type="checkbox"> Portar heat sobre el stream real</li>
<li><input disabled="" type="checkbox"> Decidir qué pasa con <code>docs/</code></li>
</ul>
<pre><code class="language-mermaid">flowchart LR
  A[PATCH] --> B[escribe el archivo]
  B --> C[commit de git]
  B --> D[evento]
</code></pre>`;

export const history = [
  { sha: 'a3f91c2', who: 'angel@cuby.mx', when: 'hace 2 h', what: 'tick: Evidencia observada' },
  { sha: '7d2e004', who: 'bea@cuby.mx', when: 'hace 6 h', what: 'edit: Evidencia observada' },
  { sha: '1b8c55a', who: 'angel@cuby.mx', when: 'hace 2 d', what: 'write: Evidencia observada' },
];

export const evidence = [
  { kind: 'document-changed', who: 'angel@cuby.mx', when: 'hace 2 h', detail: '3 de 7 casillas' },
  { kind: 'link-added', who: 'bea@cuby.mx', when: 'hace 5 h', detail: 'PR #412' },
  { kind: 'comment', who: 'caro@cuby.mx', when: 'hace 1 d', detail: '' },
  { kind: 'document-changed', who: 'bea@cuby.mx', when: 'hace 6 h', detail: '' },
  { kind: 'thread-created', who: 'angel@cuby.mx', when: 'hace 2 d', detail: '' },
];

// The wiki as the tree will really see it: docs/ with folders, mixed file kinds,
// and a README at the root of the workspace.
export const wiki: TreeNode[] = [
  { id: 'README.md', name: 'README.md', icon: BookOpenIcon },
  {
    id: 'docs',
    name: 'docs', icon: FolderIcon,
    children: [
      { id: 'docs/onboarding.md', name: 'onboarding.md', icon: FileTextIcon },
      {
        id: 'docs/arquitectura',
        name: 'arquitectura', icon: FolderIcon,
        children: [
          { id: 'docs/arquitectura/overview.md', name: 'overview.md', icon: FileTextIcon },
          { id: 'docs/arquitectura/flujo.excalidraw', name: 'flujo.excalidraw', icon: PenToolIcon },
          { id: 'docs/arquitectura/datos.md', name: 'datos.md', icon: FileTextIcon, badge: 'mermaid' },
        ],
      },
      { id: 'docs/decisiones.md', name: 'decisiones.md', icon: FileTextIcon },
    ],
  },
  {
    id: 'assets',
    name: 'assets', icon: ImageIcon,
    children: [
      { id: 'assets/board.png', name: 'board.png', icon: ImageIcon },
      { id: 'assets/logo.svg', name: 'logo.svg', icon: ImageIcon },
    ],
  },
];

export const wikiHTML = `<h1 id="arquitectura">Arquitectura</h1>
<p>Un binario. PocketBase embebido como framework, el markdown en disco bajo git,
y un MCP con forma de archivo cuyo servidor pone la pertenencia y la
concurrencia.</p>
<h2 id="el-flujo-de-una-escritura">El flujo de una escritura</h2>
<pre><code class="language-mermaid">flowchart TD
  A[MCP write_file] --> B{base coincide?}
  B -- no --> C[409, con el contenido vigente]
  B -- si --> D[escribe el archivo]
  D --> E[commit, autoria del actor]
  D --> F[evento]
  F --> G[heat]
</code></pre>
<h2 id="lo-que-no-hace">Lo que no hace</h2>
<ul>
<li>No guarda el cuerpo del documento en la base de datos.</li>
<li>No infiere evidencia barriendo: la escribe cuando ocurre.</li>
</ul>`;

// ── the planner (the strategic layer) ──────────────────────────────────────
export const objectives = [
  { n: 1, name: 'Clientes', why: 'Más ventas, retención, lealtad y valor al cliente', share: 34 },
  { n: 2, name: 'Rentabilidad de software', why: 'Que el departamento se sostenga solo', share: 26 },
  { n: 3, name: 'Optimizar recursos', why: 'No tirar dinero a la basura', share: 18 },
  { n: 4, name: 'ISO 9001', why: 'Sistema documental, trazabilidad, satisfacción', share: 12 },
  { n: 5, name: 'Mantenimiento', why: 'Evitar deuda técnica para después', share: 10 },
];

export const priorityMeaning = [
  ['P1', 'Crítica', 'Operación detenida', 'Interrumpe cualquier trabajo'],
  ['P2', 'Alta', 'Impacto fuerte, pero hay workaround', 'Se atiende rápidamente'],
  ['P3', 'Normal', 'Trabajo necesario normal', 'Entra en planeación'],
  ['P4', 'Baja', 'Mejora, nice-to-have', 'Backlog'],
];

// Impact × urgency, exactly the map that decides the priority. Consulted and
// edited here; never typed in by hand on a thread.
export const priorityMap = {
  cols: ['Urgencia alta', 'Media', 'Baja'],
  rows: [
    ['Impacto alto', 'P1', 'P2', 'P3'],
    ['Impacto medio', 'P2', 'P2', 'P3'],
    ['Impacto bajo', 'P3', 'P3', 'P4'],
  ],
};

export const inbox = [
  { id: 'i1', text: 'El cliente pide que la factura llegue en PDF y XML juntos', from: 'Ventas', when: 'hace 20 min' },
  { id: 'i2', text: '¿Podemos ver cuánto cuesta cada instancia al mes?', from: 'Dirección', when: 'hace 2 h' },
  { id: 'i3', text: 'Se cae el portal cuando entran todos a las 9', from: 'Soporte', when: 'ayer' },
  { id: 'i4', text: 'Auditoría pidió el registro de cambios de los últimos 6 meses', from: 'Calidad', when: 'hace 3 d' },
];

export const kanban = [
  {
    col: 'Por decidir',
    cards: [
      { title: 'Costo por instancia', obj: 2, prio: 'P3', due: '' },
      { title: 'Registro de cambios exportable', obj: 4, prio: 'P3', due: '30 sep' },
    ],
  },
  {
    col: 'Planeado',
    cards: [
      { title: 'Factura PDF + XML', obj: 1, prio: 'P2', due: '12 sep' },
      { title: 'Presupuesto de infraestructura', obj: 3, prio: 'P3', due: '20 sep' },
    ],
  },
  {
    col: 'En curso',
    cards: [
      { title: 'Rate limiting del portal', obj: 1, prio: 'P1', due: '8 sep' },
      { title: 'Migración a PocketBase', obj: 5, prio: 'P2', due: '25 sep' },
    ],
  },
  {
    col: 'En revisión',
    cards: [{ title: 'Reportes de rentabilidad', obj: 2, prio: 'P2', due: '9 sep' }],
  },
  {
    col: 'Hecho',
    cards: [{ title: 'Auditoría ISO 9001', obj: 4, prio: 'P2', due: '1 sep' }],
  },
];

// A week, with what is due in it. The calendar's whole job is to answer "what
// lands before Friday", so the mock has to have a Friday that hurts.
export const week = [
  { day: 'lun', date: 1, items: [{ t: 'Auditoría ISO 9001', p: 'P2' }] },
  { day: 'mar', date: 2, items: [] },
  { day: 'mié', date: 3, items: [{ t: 'Revisión de Bea', p: 'P3' }] },
  { day: 'jue', date: 4, items: [] },
  { day: 'vie', date: 5, items: [{ t: 'Factura PDF + XML', p: 'P2' }, { t: 'Rate limiting', p: 'P1' }] },
  { day: 'sáb', date: 6, items: [] },
  { day: 'dom', date: 7, items: [] },
];

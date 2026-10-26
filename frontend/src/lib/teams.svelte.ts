// El equipo en el que se está trabajando: un CONTEXTO.
//
// Cada equipo tiene sus proyectos, objetivos, secuencia, calendario, inbox y
// canal, y lo de uno no se mezcla con lo de otro. La elección se guarda en
// `prefs.team`, que es justo lo que el servidor usa como equipo por defecto
// —también para la IA—: lo que se ve aquí es a dónde va lo que no dice equipo.
//
// «Todos los equipos» es otra cosa: una VISTA del lead global que junta los
// equipos compartidos para leerlos a la vez. No es un equipo, así que nunca va
// a `prefs.team` (el servidor seguiría mandando lo que no dice equipo al último
// equipo real) y nunca es destino de creación: crear pide un equipo concreto.
// Lo personal —también el tuyo— no entra. Se recuerda por persona en este
// navegador, aparte de la preferencia.
import { api, type Team, type Workspace } from './api';

const viewKey = (owner: string) => `bw.team-view.${owner}`;

/** Lo que el selector de equipo manda al elegir «Todos los equipos». No es un
 *  id de PocketBase (15 caracteres a-z0-9): ningún equipo se llama así. */
export const ALL_TEAMS = '*all';

class Teams {
  list = $state<Team[]>([]);
  current = $state('');
  owner = '';
  /** pidió la vista «Todos los equipos»; cuenta sólo si puede tenerla */
  all = $state(false);
  /** es lead global: el único que lidera todos los equipos compartidos */
  global = $state(false);

  /** El equipo elegido, o el primero. */
  get team(): Team | undefined {
    return this.list.find((t) => t.id === this.current) ?? this.list[0];
  }

  /** Los equipos que entran en «Todos los equipos»: los no personales. */
  get sharedTeams(): Team[] {
    return this.list.filter((t) => !t.personal);
  }

  /** ¿Se le ofrece la vista? Lead global y al menos dos equipos que juntar. */
  get canAll() {
    return this.global && this.sharedTeams.length > 1;
  }

  /** ¿Está mirando «Todos los equipos»? */
  get viewing() {
    return this.all && this.canAll;
  }

  async load() {
    const owner = api.me?.id ?? '';
    if (!owner) return;
    const [out, prefs] = await Promise.all([api.teams(), api.prefs(true).catch(() => ({}) as { team?: string })]);
    if (api.me?.id !== owner) return;
    this.owner = owner;
    this.global = api.me?.role === 'lead';
    this.list = out.teams;
    const want = prefs.team && out.teams.some((t) => t.id === prefs.team) ? prefs.team : out.default;
    this.current = want || out.teams[0]?.id || '';
    try {
      this.all = localStorage.getItem(viewKey(owner)) === 'all';
    } catch {
      this.all = false;
    }
  }

  /** Entrar a un equipo: sale de «Todos los equipos» y lo hace el actual. */
  async set(id: string) {
    this.showAll(false);
    if (id === this.current) return;
    this.current = id;
    await api.setPrefs({ team: id }).catch(() => {});
  }

  /** Abrir o cerrar «Todos los equipos». No toca `prefs.team`. */
  showAll(on: boolean) {
    this.all = on;
    try {
      if (this.owner) localStorage.setItem(viewKey(this.owner), on ? 'all' : 'team');
    } catch {
      // sin almacenamiento, la vista dura lo que la pestaña
    }
  }

  mine(teamId?: string) {
    return !!teamId && this.list.some((t) => t.id === teamId);
  }

  leads(teamId?: string) {
    return this.list.find((t) => t.id === teamId)?.role === 'lead';
  }

  name(teamId?: string) {
    return this.list.find((t) => t.id === teamId)?.name ?? '';
  }

  /** ¿Entra lo de este equipo en lo que se mira? En «Todos los equipos», si es
   *  uno compartido; si no, si es el elegido. Para lo que tiene equipo y no es
   *  un proyecto: eventos, objetivos. */
  inView(teamId?: string) {
    if (this.viewing) return !!teamId && this.sharedTeams.some((t) => t.id === teamId);
    return teamId === this.current;
  }

  /** ¿Va este proyecto en la vista? Uno sin equipo va siempre. En «Todos los
   *  equipos», los de los compartidos y nada más: un equipo que no está en tu
   *  lista, siendo lead global, es el personal de otro. Fuera de esa vista, los
   *  del equipo elegido, y los de un equipo que no es tuyo (te invitaron a ESE
   *  proyecto) van siempre, aparte. */
  inScope(w: Pick<Workspace, 'team'>) {
    if (!w.team) return true;
    if (this.viewing) return this.inView(w.team);
    return !this.mine(w.team) || w.team === this.current;
  }

  shared(w: Pick<Workspace, 'team'>) {
    return !!w.team && !this.mine(w.team);
  }
}

export const teams = new Teams();

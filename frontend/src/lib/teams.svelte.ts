// El equipo en el que se está trabajando: un CONTEXTO.
//
// Cada equipo tiene sus proyectos, objetivos, secuencia, calendario, inbox y
// canal, y lo de uno no se mezcla con lo de otro. La elección se guarda en
// `prefs.team`, que es justo lo que el servidor usa como equipo por defecto
// —también para la IA—: lo que se ve aquí es a dónde va lo que no dice equipo.
import { api, type Team, type Workspace } from './api';

class Teams {
  list = $state<Team[]>([]);
  current = $state('');
  owner = '';

  /** El equipo elegido, o el primero. */
  get team(): Team | undefined {
    return this.list.find((t) => t.id === this.current) ?? this.list[0];
  }

  async load() {
    const owner = api.me?.id ?? '';
    if (!owner) return;
    const [out, prefs] = await Promise.all([api.teams(), api.prefs(true).catch(() => ({}) as { team?: string })]);
    if (api.me?.id !== owner) return;
    this.owner = owner;
    this.list = out.teams;
    const want = prefs.team && out.teams.some((t) => t.id === prefs.team) ? prefs.team : out.default;
    this.current = want || out.teams[0]?.id || '';
  }

  async set(id: string) {
    if (id === this.current) return;
    this.current = id;
    await api.setPrefs({ team: id }).catch(() => {});
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

  /** ¿Va este proyecto en la vista del equipo elegido? Los de un equipo que no
   *  es tuyo (te invitaron a ESE proyecto) van siempre, aparte. */
  inScope(w: Pick<Workspace, 'team'>) {
    return !w.team || !this.mine(w.team) || w.team === this.current;
  }

  shared(w: Pick<Workspace, 'team'>) {
    return !!w.team && !this.mine(w.team);
  }
}

export const teams = new Teams();

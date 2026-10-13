import {api} from './api';
// Pasteles: el papel de una nota y el color personal de un proyecto. Todos
// claros a propósito: el texto de un post-it es oscuro en los dos temas.
// En orden de tono, para que la rejilla se lea como un círculo cromático; los 12
// primeros que existieron siguen aquí, así ninguna nota pierde su color.
export const NOTE_COLORS=['#ffb8b8','#ffc9c2','#ffd0a8','#ffdfc4','#efe3d0','#f7e3a1','#fff0b3','#fff7dc','#e4f2b8','#d4ecb0','#cfe3cf','#bcebd9','#b8eee8','#bcecf5','#cfe6ff','#c9dafc','#d3d4fb','#dfd3ff','#f1d4f5','#f6c9ea','#fbd1df','#ffc2d8','#e3e6ec','#d6d9e0'];
export const NOTE_COLOR_NAMES=['Fresa','Coral','Mandarina','Durazno','Arena','Mostaza','Limón','Crema','Lima','Pistacho','Salvia','Menta','Agua','Cielo','Celeste','Azul','Pervinca','Lavanda','Lila','Orquídea','Rosa','Chicle','Niebla','Piedra'];
export function safeColor(value:unknown):string {return typeof value==='string'&&/^#[0-9a-f]{6}$/i.test(value)?value:'';}
class Colors {
 owner=''; colors=$state<Record<string,string>>({}); loaded=false;
 async load(){const owner=api.me?.id??'';if(!owner)return;if(this.owner===owner&&this.loaded)return;if(this.owner!==owner){this.owner=owner;this.colors={};this.loaded=false;}
 const prefs=await api.prefs(true);if(api.me?.id!==owner||this.owner!==owner)return;this.colors=prefs.project_colors??{};this.loaded=true;}
 get(id:string){return this.owner===api.me?.id?safeColor(this.colors[id]):'';}
 async set(id:string,color:string){await this.load();const owner=api.me?.id;if(!owner)return;const next={...this.colors};if(color)next[id]=safeColor(color);else delete next[id];await api.setPrefs({project_colors:next});if(api.me?.id===owner){this.colors=next;}}
}
export const projectColors=new Colors();

import { expect, test } from 'bun:test';
import { toggleMarkdownTodo } from '../src/lib/markdownTodo';
test('cambia sólo el checkbox solicitado, aunque dos textos sean iguales', () => {
 expect(toggleMarkdownTodo('- [ ] igual\n- [ ] igual\n',1,true)).toBe('- [ ] igual\n- [x] igual\n');
});
test('respeta el índice de fuente del servidor después de un ejemplo de código', () => {
 const md='```md\n- [ ] ejemplo\n```\n\n- [ ] real\n';
 expect(toggleMarkdownTodo(md,1,true)).toBe(md.replace('- [ ] real','- [x] real'));
});
test('preserva indentación, estrella, acentos y CRLF al desmarcar', () => {
 expect(toggleMarkdownTodo('  * [X] acción\r\n',0,false)).toBe('  * [ ] acción\r\n');
});
test('no inventa una tarea al recibir un índice inválido', () => {
 expect(() => toggleMarkdownTodo('- frase\n',0,true)).toThrow();
});

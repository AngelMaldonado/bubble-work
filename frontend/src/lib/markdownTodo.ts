/** Uses the server's source index, not the input's index in rendered HTML. */
export function toggleMarkdownTodo(markdown: string, index: number, done: boolean): string {
  let n = 0;
  let found = false;
  const out = markdown.split('\n').map((line) => {
    const task = /^([ \t\f\r]*[-*][ \t\f\r]+\[)[ xX](\][ \t\f\r]+.*)/.exec(line);
    if (!task || n++ !== index) return line;
    found = true;
    return line.slice(0, task[1].length) + (done ? 'x' : ' ') + line.slice(task[1].length + 1);
  }).join('\n');
  if (!found) throw new Error('Ese checkbox ya no está en el documento. Recarga antes de marcarlo.');
  return out;
}

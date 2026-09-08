// A markdown renderer for the MOCK, and only for the mock.
//
// The real app renders markdown on the SERVER — goldmark, one renderer, so that
// every surface shows the identical thing. There is no server behind
// /theme/mock, so the preview needs something, and this is deliberately the
// smallest thing that can stand in: headings, bold, italic, code, links, lists
// and checkboxes. It is not a markdown implementation and must never be used
// where the server can answer instead.
//
// It escapes first and inserts tags after, so a description containing HTML is
// shown rather than run.
const esc = (s: string) =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

function inline(s: string): string {
  return esc(s)
    .replace(/`([^`]+)`/g, '<code>$1</code>')
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    .replace(/(^|[^*])\*([^*]+)\*/g, '$1<em>$2</em>')
    .replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" rel="noreferrer noopener">$1</a>');
}

export function renderMock(md: string): string {
  const out: string[] = [];
  let list: 'ul' | null = null;

  const closeList = () => {
    if (list) {
      out.push(`</${list}>`);
      list = null;
    }
  };

  for (const raw of md.split('\n')) {
    const line = raw.trimEnd();

    const heading = /^(#{1,6})\s+(.*)$/.exec(line);
    if (heading) {
      closeList();
      const n = heading[1].length;
      const text = heading[2];
      const id = text
        .toLowerCase()
        .normalize('NFD')
        .replace(/[\u0300-\u036f]/g, '')
        .replace(/[^a-z0-9]+/g, '-')
        .replace(/^-|-$/g, '');
      out.push(`<h${n} id="${id}">${inline(text)}</h${n}>`);
      continue;
    }

    const todo = /^[-*]\s+\[( |x|X)\]\s+(.*)$/.exec(line);
    if (todo) {
      if (!list) {
        list = 'ul';
        out.push('<ul>');
      }
      const done = todo[1].toLowerCase() === 'x' ? ' checked=""' : '';
      out.push(`<li><input type="checkbox" disabled=""${done}> ${inline(todo[2])}</li>`);
      continue;
    }

    const item = /^[-*]\s+(.*)$/.exec(line);
    if (item) {
      if (!list) {
        list = 'ul';
        out.push('<ul>');
      }
      out.push(`<li>${inline(item[1])}</li>`);
      continue;
    }

    closeList();
    if (line.trim()) out.push(`<p>${inline(line)}</p>`);
  }
  closeList();
  return out.join('\n');
}

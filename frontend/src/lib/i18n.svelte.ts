// Language: English / Spanish. Detected from the browser on first visit,
// overridable from the board context menu, persisted like the theme.
//
// Deliberately no library. The catalogue is a plain object, the lookup is a
// function, and TypeScript checks that Spanish covers every English key — which
// is the only guarantee an i18n dependency would have bought us here.

export type Lang = 'en' | 'es';

const KEY = 'bubble.lang';

// en is the source of truth: its keys ARE the message ids, so a missing Spanish
// string is a type error rather than a blank label discovered in production.
const en = {
  // board / bands
  'band.in_progress': 'In progress',
  'band.reviewed': 'Reviewed',
  'band.zzzz': 'Zzzz',
  'band.rip': 'RIP',
  'band.done': 'Done',
  'board.refreshed': 'board refreshed',
  'board.unreadNotices': 'unread notices',
  'board.loading': 'Loading…',

  // bubble card + detail
  'bubble.threads': '{n} thread',
  'bubble.threads_plural': '{n} threads',
  'bubble.burning': '{n} of {total} threads in progress',
  'bubble.involved': '{n} involved',
  'bubble.owner': 'owner',
  'bubble.open': 'open',
  'bubble.markReviewed': 'reviewed',
  'bubble.unreview': 'un-review',
  'bubble.done': 'done',
  'bubble.reopen': 'reopen',
  'bubble.openTimeline': 'Open timeline',
  'bubble.birthThread': 'New thread…',
  'bubble.newBubble': 'New bubble…',

  // the heat sentence under a bubble — keys match heat.Reason* codes
  'reason.hot_current': 'meaningful output in the current cycle',
  'reason.warm_previous': 'output last cycle; active threads remain',
  'reason.cooling': 'no meaningful output this cycle or last',
  'reason.dormant_ownerless': 'no active owner',
  'reason.dormant_never': 'no meaningful output ever recorded',
  'reason.dormant_silent': 'silent for {cycles}+ cycles',
  'reason.thread_warm_open': 'progress last cycle; still open',
  'reason.thread_newborn': 'born recently; nothing produced yet',
  'reason.closed_bubble': 'outcome reached or explicitly abandoned',
  'reason.closed_thread': 'thread completed',
  'reason.all_threads_done': 'every thread is finished — close or redefine this bubble',
  'reason.no_threads': 'no threads yet',

  // discussion
  'chat.title': 'Discussion',
  'chat.empty': 'No comments yet.',
  'chat.send': 'Send',
  'chat.placeholder': 'Write a comment… (Markdown; Enter to send, Shift+Enter for newline)',
  'chat.readonly': 'Read-only display — sign in to comment.',
  'chat.unsent': 'unsent',
  'chat.retry': 'Retry',
  'chat.discard': 'Discard',
  'chat.unreachable': 'could not reach Plane',

  // degraded mode
  'status.stale': 'Plane sync is behind — this may be older data',
  'status.neverSynced': 'never synced',
  'status.lastSynced': 'last synced {ago} ago',
  'status.unsent': '{n} unsent comment — open the thread to retry or discard.',
  'status.unsent_plural': '{n} unsent comments — open the thread to retry or discard.',

  // context menus / commands
  'menu.jumpToBand': 'Jump to band',
  'menu.search': 'Search & commands',
  'menu.refreshNow': 'Refresh now',
  'menu.onlyMine': 'Only my bubbles',
  'menu.showEveryone': 'Show everyone',
  'menu.theme': 'Theme',
  'menu.language': 'Language',
  'menu.godMode': 'God Mode',
  'menu.exitKiosk': 'Exit kiosk',

  // thread interior
  'thread.aria': 'thread',
  'thread.back': 'back to board',
  'thread.loading': 'Loading thread…',
  'thread.contents': 'Contents',
  'thread.fromPlane': '✎ written in Plane',
  'thread.fromPlaneHelp':
    'This body was last edited in Plane and imported. Editing there is allowed — Bubble Work owns the format, not the exclusive right to write — but the import goes through the HTML bridge, so check that nothing was lost.',
  'thread.unpin': 'unpin',
  'thread.work': 'Work',
  'thread.logbook': 'Logbook',
  'thread.revisions': 'Revisions',
  'thread.noTasks': 'No tasks recorded.',
  'thread.dod': 'Definition of Done',
  'thread.done': 'Mark finished',
  'thread.doneHint': 'moves the work item to Done in Plane',
  'thread.reopenThread': 'Back to work',
  'thread.completing': 'finishing…',
  'thread.dodLeftOpen': 'Finished with the Definition of Done still open:',
  'thread.links': 'Evidence',
  'thread.addLink': 'add link',
  'thread.linkTitle': 'what it is (optional)',
  'thread.related': 'Related',
  'thread.addRelation': 'relate a thread',
  'thread.findThread': 'search a thread…',
  'thread.labels': 'labels',
  'thread.editLabels': 'edit labels',
  'thread.labelsPlaceholder': 'bug, infra — comma separated',
  'thread.save': 'save',
  'thread.nothing': 'Nothing to show.',
  'thread.closeChat': 'close discussion',
  'thread.openChat': 'open discussion',
  'thread.onThisPage': 'On this page',
  'fmt.bold': 'Bold (**)',
  'fmt.italic': 'Italic (*)',
  'fmt.code': 'Inline code (`)',
  'fmt.link': 'Link',
  'fmt.list': 'Bullet list',

  // bubble detail
  'detail.close': 'close',
  'detail.noThreads': 'No threads in this bubble yet.',
  'detail.openThread': 'open thread',
  'detail.planeState': 'Plane state',

  // comboboxes
  'combo.open': 'open options',
  'combo.viewScope': 'View scope',
  'combo.view': 'View',
  'combo.openViews': 'Open view scopes',
  'combo.projectFilter': 'Project filter',
  'combo.project': 'Project',
  'combo.openProjects': 'Open projects',
  'combo.instanceFilter': 'instance filter',

  // forms
  'form.instance': 'Instance',
  'form.project': 'Project',
  'form.pickProject': 'pick a project',
  'form.noProjects': 'No projects here yet — create one with',
  'form.name': 'Name',
  'form.namePlaceholder': 'Onboarding revamp',
  'form.outcome': 'Outcome',
  'form.outcomePlaceholder': 'signups convert 20% better',
  'form.owner': 'Owner',
  'form.ownerPlaceholder': 'search members…',
  'form.title': 'Title',
  'form.titlePlaceholder': 'Design the signup screen',
  'form.apiKey': 'Plane API key',

  // chrome
  'chrome.kiosk': 'bubble.work kiosk',
  'chrome.readonly': 'read-only display',
  'chrome.exitKiosk': 'exit kiosk mode',
  'chrome.searchCommands': 'search · commands (⌘K)',
  'chrome.searchHint': '⌘K search',
  'chrome.commandsHint': 'commands',
  'chrome.commandMode': 'command mode',
  'chrome.openGodMode': 'open God Mode (/god-mode)',
  'chrome.navigate': 'Navigate bubbles',
  'chrome.noBubbles': 'No bubbles here yet.',
  'chrome.press': 'Press',

  'form.cancel': 'cancel',
  'form.optionalDone': 'optional · what “done” looks like',
  'form.optionalOwner': "optional · who's accountable",
  'form.newBubble': 'new bubble',
  'form.newBubbleHint': 'a durable grouping of work · maps to a Plane module',
  'form.creating': 'creating…',
  'form.createBubble': 'create bubble',
  'form.birthThread': 'new thread',
  'form.into': 'into',
  'form.birthing': 'creating…',
  'form.birth': 'create thread',
  'form.document': 'Document',
  'form.documentHint': 'optional · markdown · your headings, your order',
  'form.documentPlaceholder':
    'Write it however the work has shape.\n\nWorth having somewhere: why this exists, what is true when it is done, the next concrete action.\n\n- [ ] checkboxes count anywhere — ticking one keeps this thread warm',
  'form.login': 'log in',

  'chrome.emptyHint': 'type {cmd}, and choose {new} — then birth threads into it.',
  'chrome.exitKioskShort': 'exit kiosk',
  'chrome.newBubbleCmd': 'new bubble',

  // god mode (admin only)
  'god.aria': 'god mode',
  'god.title': 'God Mode',
  'god.adminOnly': 'Service admin only.',
  'god.server': 'Server',
  'god.rateBudget': 'Plane rate budget',
  'god.atFloor': 'At the floor — background work is yielding to keep pages fast.',
  'god.noInstances': 'No instances configured.',
  'god.outboxEmpty': 'Empty — every write has reached Plane.',
  'cmd.h1': 'Heading 1',
  'cmd.h2': 'Heading 2',
  'cmd.h3': 'Heading 3',
  'cmd.bullet': 'Bulleted list',
  'cmd.numbered': 'Numbered list',
  'cmd.todo': 'To-do',
  'cmd.quote': 'Quote',
  'cmd.code': 'Code block',
  'cmd.mermaid': 'Diagram',
  'cmd.table': 'Table',
  'cmd.divider': 'Divider',
  'cmd.link': 'Link',
  'cmd.none': 'No matching block',
  'cmd.hint': 'type to filter · ↑↓ to move · ⏎ to insert',
  'editor.aria': 'artifact source',
  'editor.escHint': '/ for blocks · esc to go back',
  'editor.vimHint': '/ for blocks · :w saves · :q goes back',
  'editor.vimTitle': 'vim keybindings',
  'editor.placeholder': 'Write in Markdown. Type / for blocks.',
  'editor.upToDate': 'up to date',
  'editor.unsaved': 'unsaved…',
  'editor.saving': 'saving…',
  'editor.saved': 'saved',
  'editor.failed': 'not saved',
  'editor.conflictShort': 'changed elsewhere',
  'editor.conflict': 'Someone (or an agent) changed this while you were editing it.',
  'editor.reload': 'Load theirs',
  'editor.changedElsewhere': 'This thread changed while you were editing. Your unsaved work is kept.',
  'editor.todoMoved': 'That to-do moved — nothing was ticked. Showing what is there now.',
  'editor.heading': 'Heading',
  'editor.bold': 'Bold',
  'editor.boldText': 'bold text',
  'editor.italic': 'Italic',
  'editor.italicText': 'italic text',
  'editor.bullet': 'Bullet list',
  'editor.todo': 'To-do',
  'editor.quote': 'Quote',
  'editor.code': 'Code',
  'editor.link': 'Link',
  'editor.linkText': 'link text',
  'thread.renameHint': 'click to rename · enter to save · esc to cancel',
  'thread.rendered': 'Rendered',
  'thread.markdown': 'Markdown',
  'thread.viewMode': 'view mode',
  'thread.notEditable': 'Revisions are separate work items — open one in Plane to edit it.',
  'del.title': 'Delete {what}?',
  'del.irreversible': 'This deletes it from Plane. It cannot be undone.',
  'del.bubble': 'The bubble goes; its {n} thread(s) stay in Plane but belong to no bubble, so they leave the board.',
  'del.bubbleEmpty': 'The bubble goes. It has no threads.',
  'del.thread': 'Its Brief, Logbook, comments and revisions go with it.',
  'del.region': 'The thread stays; only this section is removed.',
  'del.prefer': 'Closing a bubble keeps the record of what was done.',
  'del.cancel': 'Cancel',
  'del.confirm': 'Delete',
  'del.deleting': 'deleting…',
  'del.bubbleWord': 'bubble',
  'del.threadWord': 'thread',
  'bubble.delete': 'Delete bubble',
  'bubble.rename': 'Rename bubble',
  'bubble.renameHint': 'click to rename · enter to save · esc to cancel',
  'thread.delete': 'Delete thread',
  'thread.deleteRegion': 'Delete this section',
  'tour.start': 'Take the tour',
  'tour.next': 'Next',
  'tour.prev': 'Back',
  'tour.done': 'Done',
  'tour.1.title': 'A bubble is a body of work',
  'tour.1.body': 'Not a folder and not a sprint — a durable grouping with an <b>outcome</b>, and the unit of your attention. It is allowed to die when that outcome no longer justifies the work.',
  'tour.2.title': 'These bands are computed, not set',
  'tour.2.body': 'Nobody drags a bubble into 🔥 or 🪦. Its band is derived from <b>evidence of changed reality</b> — a ticked todo, a plan that moved, a deliverable that landed — measured against the cycle.',
  'tour.3.title': 'A bubble is its threads',
  'tour.3.body': 'A thread is one executable unit of work. A bubble takes the band of its hottest unfinished thread, so an orb sinking means the work inside it stopped, not that someone forgot to update a status.',
  'tour.4.title': 'One page, not a second tracker',
  'tour.4.body': 'Every thread carries a <b>Brief</b> — the problem and its Definition of Done — and a <b>Logbook</b>, the plan in phases with its todos. Both live on the same Plane page, and a thread is not meant to start without them.',
  'tour.5.title': 'Ticking this is the evidence',
  'tour.5.body': 'A completed todo is production: it warms the thread and the bubble above it. Comments do not. You can tick one right here without opening the editor.',
  'tour.6.title': 'Rendered, or Markdown',
  'tour.6.body': 'The same document, two views. In Markdown you get syntax highlighting, lists that continue themselves, and <b>/</b> for blocks. It saves as you type; esc brings you back.',
  'tour.7.title': 'Heat is evidence, never motion',
  'tour.7.body': 'Nothing here rewards looking busy. If a bubble has gone cold, revive it with real work, redefine it, or close it — a bubble dying is the system working. Reopen this any time from the <b>?</b> in the corner.',
  'tour.empty.title': 'Start with a bubble',
  'tour.empty.body': 'There is nothing on the board yet. Press <b>⌘K</b> here to create a bubble, then create a thread inside it — a name and whatever you want to write. What keeps it warm is the writing, not the format.',
  'mcp.badge': 'MCP',
  'mcp.title': 'Connect your agent',
  'mcp.lead': 'Configure the MCP server with the http(s) transport in your global config, using the token below. Your agent then acts as you — the framework\'s rules are enforced on the server, so no client can bypass them.',
  'mcp.url': 'Server URL',
  'mcp.token': 'Your token',
  'mcp.copy': 'Copy',
  'mcp.copied': 'Copied',
  'mcp.reveal': 'Show',
  'mcp.hide': 'Hide',
  'mcp.claudeCode': 'Claude Code — one command',
  'mcp.globalConfig': 'Global config file',
  'mcp.warn': 'This is your personal Plane API key, not a scoped token. Anyone holding it can read and write as {who}. Do not paste it into a shared machine or a screenshot.',
  'mcp.promptLabel': 'Or ask your assistant to do it',
  'mcp.promptHint': 'Paste this into any assistant that can edit files or run commands on your machine.',
  'mcp.prompt':
    'Add the Bubble Work MCP server to your own MCP configuration.\n\nDo this for YOURSELF ONLY — the assistant I am talking to right now. Do not configure any other assistant, editor, CLI or tool, and do not edit a config file that belongs to one. If you are unsure which client you are, ask me instead of touching several.\n\nTransport: streamable HTTP\nURL: {url}\nAuth: send the header  Authorization: Bearer {token}\n\nWork out which global config file you yourself read and follow your own schema for an HTTP server with custom headers, rather than guessing at a path.\n\nThen confirm it worked by listing the server tools. You should see list_bubbles, read_thread, create_thread and update_thread among them. If it fails, tell me the exact error instead of retrying blindly.\n\nTreat the token as a secret: put it in the config file, do not echo it back to me, and do not write it anywhere else.',
  'mcp.hint': 'The URL must be reachable from wherever the agent runs — a laptop off the network cannot reach a LAN-only host.',
  'god.mirror': 'Plane mirror',
  'god.fidelity': 'Check fidelity',
  'god.fidelityRunning': 'checking…',
  'god.fidelityOk': '✓ every body survives a write ({n} checked)',
  'god.fidelitySurvive': '{stable}/{bodies} bodies survive a write',
  'god.fidelitySplice': '{clean}/{bodies} bodies splice back to identical bytes',
  'god.fidelityLost': '{mentions} mention(s) and {assets} image(s) would be destroyed',
  'god.fidelityHint': 'Mentions and images are not carried by the markdown bridge yet, which is why writes splice blocks instead of re-rendering.',
  'god.maintenance': 'Maintenance',
  'god.members': 'Members',
  'god.calibration': 'Buoyancy calibration',
  'god.differsDefault': 'differs from the stock default',
  'god.discardEdits': 'Discard edits',
  'god.noChanges': 'No changes',
  'god.applyChanges': 'Apply {n} change(s)',
  'god.resetDefaults': 'Reset to defaults',
  'god.instance': 'instance',
  'god.kioskLabel': 'label (e.g. lobby screen)',
  'god.mintToken': 'Mint token',
  'god.noKiosk': 'No kiosk tokens. Mint one to boot a read-only display board.',
  'god.copyKiosk': 'copy kiosk URL',
  'god.revoke': 'revoke',

  'tune.group.pulse': 'Pulse',
  'tune.group.bubble': 'Bubbles',
  'tune.group.thread': 'Threads',

  'god.allOrgs': 'all orgs',
  'god.revision': 'revision',
  'god.built': 'built',
  'god.started': 'started',
  'god.instances': 'instances',
  'god.identities': 'identities',
  'god.noMirror': 'no mirror',
  'god.slug': 'slug',
  'god.baseUrl': 'base url',
  'god.workspace': 'workspace',
  'god.projectCol': 'project',
  'god.webhook': 'webhook',
  'god.cached': 'cached',
  'god.nameCol': 'name',
  'god.emailCol': 'email',
  'god.roleCol': 'role',
  'god.modified': 'modified',
  'chrome.kioskBadge': 'kiosk',
  'combo.allInstances': 'all instances',

  'god.writesToPlane': 'writes to Plane',
  'omni.applyHint': '⏎ apply · esc back',
  'omni.navHint': '↑↓ move · ⏎ open · esc close · type',
  'omni.forCommands': 'for commands',

  // combobox OPTIONS — built in script, so a markup grep never sees them
  'combo.allProjects': 'All projects',
  'combo.everyone': 'Everyone',
  'combo.mine': 'Mine',

  // command palette
  'cmd.refresh': 'refresh',
  'cmd.refreshHint': 're-poll the server',
  'cmd.newBubble': 'new bubble…',
  'cmd.newBubbleHint': 'create a bubble (Plane module)',
  'cmd.birth': 'new thread…',
  'cmd.birthHint': 'a name is all it needs',
  'cmd.review': 'mark reviewed…',
  'cmd.unreview': 'un-review…',
  'cmd.close': 'close bubble (done)…',
  'cmd.setOwner': 'set owner…',
  'cmd.setOutcome': 'set outcome…',
  'cmd.renameBubble': 'rename bubble…',
  'cmd.renameBubbleHint': 'the handle only — the contract is untouched',
  'cmd.signOut': 'sign out',
  'cmd.viewAll': 'view all instances',
  'cmd.switchTo': 'switch to {slug}',
  'cmd.godPanel': 'godmode: open panel',
  'cmd.godPanelHint': 'all admin options (/god-mode)',
  'cmd.godCrossOrg': 'cross-org bubble view',
  'cmd.godStats': 'godmode: stats',
  'cmd.godInstances': 'godmode: instances',
  'cmd.godRefresh': 'godmode: refresh caches',
  'cmd.godTick': 'godmode: tick now',

  // toasts
  'toast.cachesRefreshed': 'caches refreshed',
  'toast.tickTriggered': 'cooling sweep triggered',
  'toast.calibrationReset': 'calibration reset to defaults',
  'toast.copied': 'copied to clipboard',
  'toast.copyFailed': 'copy failed — select and copy manually',
  'toast.kioskMinted': 'kiosk token minted',
  'toast.kioskRevoked': 'kiosk token revoked',
  'toast.applied': 'applied {n} change(s) — the board is already using them',
  'toast.outboxDropped': 'dropped outbox entry {id}',
  'toast.crossOrgOn': 'cross-org board on',
  'toast.crossOrgOff': 'cross-org board off',
  'toast.autoStateOn': '{slug}: now writing state to Plane',
  'toast.autoStateOff': '{slug}: read-only again',
  'toast.mirrorMatches': '{slug}: mirror matches Plane',
  'toast.mirrorFindings': '{slug}: {n} finding(s)',
  'toast.pickInstance': 'pick an instance',

  // knob help + group hints, keyed like the labels above
  'tune.hint.pulse': 'the rhythm both grains are measured against',
  'tune.hint.bubble': 'how a bubble reaches 🪦 vs 😴',
  'tune.hint.thread': 'how a single work item moves between bands',

  // The WORKSPACE tier. A Workspace is a Plane PROJECT — not a Plane workspace
  // (AGENTS.md) — and until deletion landed it was the one tier the web treated
  // as a filter and nothing more.
  // project pages: a workspace's documentation (docs, specs, decision records)
  'pages.title': 'Documents',
  'pages.new': 'New document',
  'pages.newInside': 'New document inside this one',
  'pages.empty': 'No documents yet. Specs and references live here.',
  'pages.pick': 'Pick a document to read it.',
  'pages.titlePlaceholder': 'what this document is called',
  'pages.rename': 'Rename document',
  'pages.delete': 'Delete document',
  'pages.locked': 'locked in Plane',
  'pages.held': 'kept in Bubble Work',
  'pages.heldHint':
    "this instance's Plane does not serve pages on its API, so Bubble Work is the record for these — they will not appear in Plane's own UI",
  'del.page': 'The document goes. Threads and bubbles are untouched.',

  'omni.workspace': 'workspace',
  'omni.scoped': 'showing {name}',
  // the Alt+Tab-style workspace switcher
  'switch.title': 'Switch workspace',
  'switch.hint': 'Tab to advance · ← → to move · a letter to jump · release Shift to go · Esc to cancel',
  'switch.bubbles': '{n} bubble(s)',
  'ws.menuTitle': 'Workspace actions',
  'ws.noneScoped': 'No workspace scoped',
  'ws.pickFirst': 'Pick one workspace in the filter to rename or delete it.',
  'ws.rename': 'Rename workspace',
  'ws.new': 'New workspace…',
  'ws.delete': 'Delete workspace',
  'ws.formNew': 'New workspace',
  'ws.formRename': 'Rename workspace',
  'ws.isAProject': 'a Plane project — the boundary for a body of work',
  'ws.namePlaceholder': 'what this body of work is called',
  'ws.identifier': 'Identifier',
  'ws.identifierHint': '(optional — derived from the name)',
  'ws.createBtn': 'Create',
  'ws.renameBtn': 'Rename',
  'ws.saving': 'saving…',
  'ws.renamed': 'renamed to {name}',
  'ws.created': '{name} created — pick it in the workspace filter and give it a bubble',
  'ws.deleted': 'deleted {name} — {b} bubble(s), {n} thread(s)',
  'del.workspace':
    'The whole workspace goes: {b} bubble(s) and {n} thread(s), with every artifact and comment in them.',
  'del.workspacePrefer':
    'To retire one body of work, closing its bubbles keeps the record of what was done.',

  // moving a thread between bubbles
  'thread.move': 'Move to bubble',
  'move.title': 'Move {what}',
  'move.explain':
    'Nothing is lost — its Brief, Logbook, comments and history come with it, and moving it back is the same step.',
  'move.sameWorkspace': 'Only bubbles in the same workspace: that is Plane\u2019s limit, not ours.',
  'move.pick': 'Destination bubble',
  'move.none': 'There is no other bubble in this workspace to move it to.',
  'move.go': 'Move',
  'move.moving': 'moving…',
  'move.done': 'moved to {name}',
} as const;

export type MsgKey = keyof typeof en;

// Two families are looked up by COMPUTED key and so have no literal call site:
//   band.*   — levelLabel() builds `band.${level}`
//   reason.* — i18n.reason() builds `reason.${server code}`
// Grepping for a literal will find nothing and make them look dead. They are
// not. Delete one and a band goes nameless.

// Record<MsgKey, string> is the point: adding an English key and forgetting the
// Spanish one fails the build.
const es: Record<MsgKey, string> = {
  'band.in_progress': 'En curso',
  'band.reviewed': 'Revisado',
  'band.zzzz': 'Dormido',
  'band.rip': 'Abandonado',
  'band.done': 'Terminado',
  'board.refreshed': 'tablero actualizado',
  'board.unreadNotices': 'avisos sin leer',
  'board.loading': 'Cargando…',

  'bubble.threads': '{n} hilo',
  'bubble.threads_plural': '{n} hilos',
  'bubble.burning': '{n} de {total} hilos en curso',
  'bubble.involved': '{n} participantes',
  'bubble.owner': 'responsable',
  'bubble.open': 'abrir',
  'bubble.markReviewed': 'revisado',
  'bubble.unreview': 'quitar revisión',
  'bubble.done': 'terminado',
  'bubble.reopen': 'reabrir',
  'bubble.openTimeline': 'Abrir línea de tiempo',
  'bubble.birthThread': 'Crear hilo…',
  'bubble.newBubble': 'Nueva burbuja…',

  // "salida real" rather than a literal "producción": the model means evidence
  // that something CHANGED, not activity, and that distinction is the whole
  // point of the vocabulary (AGENTS.md).
  'reason.hot_current': 'salida real en el ciclo actual',
  'reason.warm_previous': 'salida el ciclo pasado; quedan hilos activos',
  'reason.cooling': 'sin salida real este ciclo ni el anterior',
  'reason.dormant_ownerless': 'sin responsable activo',
  'reason.dormant_never': 'nunca registró salida real',
  'reason.dormant_silent': 'en silencio {cycles}+ ciclos',
  'reason.thread_warm_open': 'avanzó el ciclo pasado; sigue abierto',
  'reason.thread_newborn': 'recién creado; todavía sin producir',
  'reason.closed_bubble': 'resultado alcanzado o abandonado a propósito',
  'reason.closed_thread': 'hilo completado',
  'reason.all_threads_done': 'todos los hilos terminaron — cierra o redefine esta burbuja',
  'reason.no_threads': 'aún sin hilos',

  'chat.title': 'Discusión',
  'chat.empty': 'Aún no hay comentarios.',
  'chat.send': 'Enviar',
  'chat.placeholder': 'Escribe un comentario… (Markdown; Enter envía, Shift+Enter salto de línea)',
  'chat.readonly': 'Pantalla de solo lectura — inicia sesión para comentar.',
  'chat.unsent': 'sin enviar',
  'chat.retry': 'Reintentar',
  'chat.discard': 'Descartar',
  'chat.unreachable': 'no se pudo contactar a Plane',

  'status.stale': 'La sincronización con Plane está atrasada — estos datos pueden no estar al día',
  'status.neverSynced': 'nunca sincronizado',
  'status.lastSynced': 'sincronizado hace {ago}',
  'status.unsent': '{n} comentario sin enviar — abre el hilo para reintentar o descartar.',
  'status.unsent_plural': '{n} comentarios sin enviar — abre el hilo para reintentar o descartar.',

  'menu.jumpToBand': 'Ir a la banda',
  'menu.search': 'Buscar y comandos',
  'menu.refreshNow': 'Actualizar ahora',
  'menu.onlyMine': 'Solo mis burbujas',
  'menu.showEveryone': 'Ver todas',
  'menu.theme': 'Tema',
  'menu.language': 'Idioma',
  'menu.godMode': 'Modo Dios',
  'menu.exitKiosk': 'Salir de kiosco',

  'thread.aria': 'hilo',
  'thread.back': 'volver al tablero',
  'thread.loading': 'Cargando hilo…',
  'thread.contents': 'Contenido',
  'thread.fromPlane': '✎ escrito en Plane',
  'thread.fromPlaneHelp':
    'Este cuerpo se editó por última vez en Plane y se importó. Editar allá está permitido — Bubble Work es dueño del formato, no del derecho exclusivo de escribir — pero la importación pasa por el puente de HTML, así que revisa que no se haya perdido nada.',
  'thread.unpin': 'quitar fijado',
  'thread.work': 'Trabajo',
  'thread.logbook': 'Bitácora',
  'thread.revisions': 'Revisiones',
  'thread.noTasks': 'Sin tareas registradas.',
  'thread.dod': 'Definición de Terminado',
  'thread.done': 'Marcar terminado',
  'thread.doneHint': 'mueve el work item a Terminado en Plane',
  'thread.reopenThread': 'Volver al trabajo',
  'thread.completing': 'terminando…',
  'thread.dodLeftOpen': 'Terminado con la Definición de Terminado abierta:',
  'thread.links': 'Evidencia',
  'thread.addLink': 'agregar liga',
  'thread.linkTitle': 'qué es (opcional)',
  'thread.related': 'Relacionados',
  'thread.addRelation': 'relacionar un hilo',
  'thread.findThread': 'buscar un hilo…',
  'thread.labels': 'etiquetas',
  'thread.editLabels': 'editar etiquetas',
  'thread.labelsPlaceholder': 'bug, infra — separadas por coma',
  'thread.save': 'guardar',
  'thread.nothing': 'Nada que mostrar.',
  'thread.closeChat': 'cerrar discusión',
  'thread.openChat': 'abrir discusión',
  'thread.onThisPage': 'En esta página',
  'fmt.bold': 'Negrita (**)',
  'fmt.italic': 'Cursiva (*)',
  'fmt.code': 'Código en línea (`)',
  'fmt.link': 'Enlace',
  'fmt.list': 'Lista con viñetas',

  'detail.close': 'cerrar',
  'detail.noThreads': 'Esta burbuja aún no tiene hilos.',
  'detail.openThread': 'abrir hilo',
  'detail.planeState': 'estado en Plane',

  'combo.open': 'abrir opciones',
  'combo.viewScope': 'Ámbito de vista',
  'combo.view': 'Vista',
  'combo.openViews': 'Abrir ámbitos de vista',
  'combo.projectFilter': 'Filtro de proyecto',
  'combo.project': 'Proyecto',
  'combo.openProjects': 'Abrir proyectos',
  'combo.instanceFilter': 'filtro de instancia',

  'form.instance': 'Instancia',
  'form.project': 'Proyecto',
  'form.pickProject': 'elige un proyecto',
  'form.noProjects': 'Aquí aún no hay proyectos — crea uno con',
  'form.name': 'Nombre',
  'form.namePlaceholder': 'Rediseño de onboarding',
  'form.outcome': 'Resultado',
  'form.outcomePlaceholder': 'los registros convierten 20% mejor',
  'form.owner': 'Responsable',
  'form.ownerPlaceholder': 'buscar miembros…',
  'form.title': 'Título',
  'form.titlePlaceholder': 'Diseñar la pantalla de registro',
  'form.apiKey': 'Clave API de Plane',

  'chrome.kiosk': 'kiosco bubble.work',
  'chrome.readonly': 'pantalla de solo lectura',
  'chrome.exitKiosk': 'salir del modo kiosco',
  'chrome.searchCommands': 'buscar · comandos (⌘K)',
  'chrome.searchHint': '⌘K buscar',
  'chrome.commandsHint': 'comandos',
  'chrome.commandMode': 'modo comando',
  'chrome.openGodMode': 'abrir Modo Dios (/god-mode)',
  'chrome.navigate': 'Navegar burbujas',
  'chrome.noBubbles': 'Aquí aún no hay burbujas.',
  'chrome.press': 'Pulsa',

  'form.cancel': 'cancelar',
  'form.optionalDone': 'opcional · cómo se ve “terminado”',
  'form.optionalOwner': 'opcional · quién responde por esto',
  'form.newBubble': 'nueva burbuja',
  'form.newBubbleHint': 'una agrupación duradera de trabajo · equivale a un módulo de Plane',
  'form.creating': 'creando…',
  'form.createBubble': 'crear burbuja',
  'form.birthThread': 'crear un hilo',
  'form.into': 'en',
  'form.birthing': 'creando…',
  'form.birth': 'crear hilo',
  'form.document': 'Documento',
  'form.documentHint': 'opcional · markdown · tus títulos, tu orden',
  'form.documentPlaceholder':
    'Escríbelo como el trabajo tenga forma.\n\nVale la pena que en algún lado esté: por qué existe, qué es cierto cuando esté terminado, la siguiente acción concreta.\n\n- [ ] las casillas cuentan en cualquier parte — marcar una mantiene caliente este hilo',
  'form.login': 'entrar',

  'chrome.emptyHint': 'escribe {cmd} y elige {new} — luego crea hilos dentro.',
  'chrome.exitKioskShort': 'salir del kiosco',
  'chrome.newBubbleCmd': 'nueva burbuja',

  'god.aria': 'modo dios',
  'god.title': 'Modo Dios',
  'god.adminOnly': 'Solo para administradores del servicio.',
  'god.server': 'Servidor',
  'god.rateBudget': 'Cuota de peticiones a Plane',
  'god.atFloor': 'En el límite — el trabajo en segundo plano cede para mantener las páginas rápidas.',
  'god.noInstances': 'No hay instancias configuradas.',
  'god.outboxEmpty': 'Vacía — todas las escrituras llegaron a Plane.',
  'cmd.h1': 'Título 1',
  'cmd.h2': 'Título 2',
  'cmd.h3': 'Título 3',
  'cmd.bullet': 'Lista con viñetas',
  'cmd.numbered': 'Lista numerada',
  'cmd.todo': 'Pendiente',
  'cmd.quote': 'Cita',
  'cmd.code': 'Bloque de código',
  'cmd.mermaid': 'Diagrama',
  'cmd.table': 'Tabla',
  'cmd.divider': 'Separador',
  'cmd.link': 'Enlace',
  'cmd.none': 'Ningún bloque coincide',
  'cmd.hint': 'escribe para filtrar · ↑↓ para mover · ⏎ para insertar',
  'editor.aria': 'fuente del artefacto',
  'editor.escHint': '/ para bloques · esc para volver',
  'editor.vimHint': '/ para bloques · :w guarda · :q vuelve',
  'editor.vimTitle': 'atajos de vim',
  'editor.placeholder': 'Escribe en Markdown. Teclea / para bloques.',
  'editor.upToDate': 'al día',
  'editor.unsaved': 'sin guardar…',
  'editor.saving': 'guardando…',
  'editor.saved': 'guardado',
  'editor.failed': 'no se guardó',
  'editor.conflictShort': 'cambió en otro lado',
  'editor.conflict': 'Alguien (o un agente) cambió esto mientras lo editabas.',
  'editor.reload': 'Cargar lo suyo',
  'editor.changedElsewhere': 'Este hilo cambió mientras editabas. Tu trabajo sin guardar se conserva.',
  'editor.todoMoved': 'Ese pendiente se movió — no se marcó nada. Esto es lo que hay ahora.',
  'editor.heading': 'Encabezado',
  'editor.bold': 'Negrita',
  'editor.boldText': 'texto en negrita',
  'editor.italic': 'Cursiva',
  'editor.italicText': 'texto en cursiva',
  'editor.bullet': 'Lista con viñetas',
  'editor.todo': 'Pendiente',
  'editor.quote': 'Cita',
  'editor.code': 'Código',
  'editor.link': 'Enlace',
  'editor.linkText': 'texto del enlace',
  'thread.renameHint': 'clic para renombrar · enter para guardar · esc para cancelar',
  'thread.rendered': 'Vista',
  'thread.markdown': 'Markdown',
  'thread.viewMode': 'modo de vista',
  'thread.notEditable': 'Las revisiones son elementos de trabajo aparte — ábrela en Plane para editarla.',
  'del.title': '¿Eliminar {what}?',
  'del.irreversible': 'Esto lo elimina de Plane. No se puede deshacer.',
  'del.bubble': 'La burbuja se va; sus {n} hilo(s) siguen en Plane pero sin burbuja, así que salen del tablero.',
  'del.bubbleEmpty': 'La burbuja se va. No tiene hilos.',
  'del.thread': 'Su Brief, Bitácora, comentarios y revisiones se van con él.',
  'del.region': 'El hilo permanece; solo se elimina esta sección.',
  'del.prefer': 'Cerrar una burbuja conserva el registro de lo que se hizo.',
  'del.cancel': 'Cancelar',
  'del.confirm': 'Eliminar',
  'del.deleting': 'eliminando…',
  'del.bubbleWord': 'burbuja',
  'del.threadWord': 'hilo',
  'bubble.delete': 'Eliminar burbuja',
  'bubble.rename': 'Renombrar burbuja',
  'bubble.renameHint': 'clic para renombrar · enter para guardar · esc para cancelar',
  'thread.delete': 'Eliminar hilo',
  'thread.deleteRegion': 'Eliminar esta sección',
  'tour.start': 'Ver el recorrido',
  'tour.next': 'Siguiente',
  'tour.prev': 'Atrás',
  'tour.done': 'Listo',
  'tour.1.title': 'Una burbuja es un cuerpo de trabajo',
  'tour.1.body': 'No es una carpeta ni un sprint — es una agrupación duradera con un <b>resultado</b>, y la unidad de tu atención. Puede morir cuando ese resultado ya no justifica el trabajo.',
  'tour.2.title': 'Estas bandas se calculan, no se asignan',
  'tour.2.body': 'Nadie arrastra una burbuja a 🔥 ni a 🪦. Su banda se deriva de <b>evidencia de realidad cambiada</b> — un pendiente marcado, un plan que avanzó, un entregable que aterrizó — medida contra el ciclo.',
  'tour.3.title': 'Una burbuja son sus hilos',
  'tour.3.body': 'Un hilo es una unidad ejecutable de trabajo. La burbuja toma la banda de su hilo más caliente sin terminar, así que una burbuja que se hunde significa que el trabajo se detuvo, no que alguien olvidó actualizar un estado.',
  'tour.4.title': 'Una sola página, no un segundo rastreador',
  'tour.4.body': 'Cada hilo lleva un <b>Brief</b> — el problema y su Definición de Hecho — y una <b>Bitácora</b>, el plan por fases con sus pendientes. Ambos viven en la misma página de Plane, y un hilo no debería empezar sin ellos.',
  'tour.5.title': 'Marcar esto es la evidencia',
  'tour.5.body': 'Un pendiente completado es producción: calienta el hilo y la burbuja que lo contiene. Los comentarios no. Puedes marcarlo aquí mismo sin abrir el editor.',
  'tour.6.title': 'Vista, o Markdown',
  'tour.6.body': 'El mismo documento, dos vistas. En Markdown tienes resaltado de sintaxis, listas que se continúan solas y <b>/</b> para bloques. Se guarda mientras escribes; esc te regresa.',
  'tour.7.title': 'El calor es evidencia, nunca movimiento',
  'tour.7.body': 'Aquí nada premia parecer ocupado. Si una burbuja se enfrió, revívela con trabajo real, redefínela o ciérrala — que una burbuja muera es el sistema funcionando. Puedes volver a abrir esto desde el <b>?</b> de la esquina.',
  'tour.empty.title': 'Empieza con una burbuja',
  'tour.empty.body': 'Todavía no hay nada en el tablero. Presiona <b>⌘K</b> aquí para crear una burbuja, y luego crea un hilo dentro — un nombre y lo que quieras escribir. Lo que lo mantiene caliente es la escritura, no el formato.',
  'mcp.badge': 'MCP',
  'mcp.title': 'Conecta tu agente',
  'mcp.lead': 'Configura el servidor MCP con protocolo http(s) en tu configuración global, con este token. Tu agente actuará como tú — las reglas del marco se aplican en el servidor, así que ningún cliente puede saltárselas.',
  'mcp.url': 'URL del servidor',
  'mcp.token': 'Tu token',
  'mcp.copy': 'Copiar',
  'mcp.copied': 'Copiado',
  'mcp.reveal': 'Mostrar',
  'mcp.hide': 'Ocultar',
  'mcp.claudeCode': 'Claude Code — un comando',
  'mcp.globalConfig': 'Archivo de configuración global',
  'mcp.warn': 'Ésta es tu API key personal de Plane, no un token acotado. Quien la tenga puede leer y escribir como {who}. No la pegues en una máquina compartida ni en una captura.',
  'mcp.promptLabel': 'O pídele a tu asistente que lo haga',
  'mcp.promptHint': 'Pega esto en cualquier asistente que pueda editar archivos o ejecutar comandos en tu máquina.',
  'mcp.prompt':
    'Agrega el servidor MCP de Bubble Work a tu propia configuración de MCP.\n\nHazlo SÓLO PARA TI — el asistente con el que estoy hablando ahora. No configures ningún otro asistente, editor, CLI ni herramienta, y no edites archivos de configuración que pertenezcan a otro. Si no tienes claro qué cliente eres, pregúntame en vez de tocar varios.\n\nProtocolo: HTTP (streamable http)\nURL: {url}\nAutenticación: envía la cabecera  Authorization: Bearer {token}\n\nAverigua cuál es el archivo de configuración global que tú mismo lees y sigue tu propio esquema para un servidor HTTP con cabeceras personalizadas, en vez de adivinar la ruta.\n\nLuego confirma que funcionó listando las herramientas del servidor. Deberías ver list_bubbles, read_thread, create_thread y update_thread entre ellas. Si falla, dime el error exacto en vez de reintentar a ciegas.\n\nTrata el token como secreto: ponlo en el archivo de configuración, no me lo repitas y no lo escribas en ningún otro lado.',
  'mcp.hint': 'La URL debe ser alcanzable desde donde corra el agente — una laptop fuera de la red no puede llegar a un host sólo de LAN.',
  'god.mirror': 'Espejo de Plane',
  'god.fidelity': 'Revisar fidelidad',
  'god.fidelityRunning': 'revisando…',
  'god.fidelityOk': '✓ todos los cuerpos sobreviven una escritura ({n} revisados)',
  'god.fidelitySurvive': '{stable}/{bodies} cuerpos sobreviven una escritura',
  'god.fidelitySplice': '{clean}/{bodies} cuerpos se empalman a bytes idénticos',
  'god.fidelityLost': 'se destruirían {mentions} mención(es) y {assets} imagen(es)',
  'god.fidelityHint': 'El puente de markdown aún no conserva menciones ni imágenes; por eso las escrituras empalman bloques en vez de volver a renderizar.',
  'god.maintenance': 'Mantenimiento',
  'god.members': 'Miembros',
  'god.calibration': 'Calibración de flotabilidad',
  'god.differsDefault': 'difiere del valor de fábrica',
  'god.discardEdits': 'Descartar cambios',
  'god.noChanges': 'Sin cambios',
  'god.applyChanges': 'Aplicar {n} cambio(s)',
  'god.resetDefaults': 'Restaurar valores de fábrica',
  'god.instance': 'instancia',
  'god.kioskLabel': 'etiqueta (p. ej. pantalla de recepción)',
  'god.mintToken': 'Generar token',
  'god.noKiosk': 'No hay tokens de kiosco. Genera uno para arrancar un tablero de solo lectura.',
  'god.copyKiosk': 'copiar URL de kiosco',
  'god.revoke': 'revocar',

  'tune.group.pulse': 'Pulso',
  'tune.group.bubble': 'Burbujas',
  'tune.group.thread': 'Hilos',

  'god.allOrgs': 'todas las orgs',
  'god.revision': 'revisión',
  'god.built': 'compilado',
  'god.started': 'iniciado',
  'god.instances': 'instancias',
  'god.identities': 'identidades',
  'god.noMirror': 'sin espejo',
  'god.slug': 'slug',
  'god.baseUrl': 'url base',
  'god.workspace': 'espacio de trabajo',
  'god.projectCol': 'proyecto',
  'god.webhook': 'webhook',
  'god.cached': 'en caché',
  'god.nameCol': 'nombre',
  'god.emailCol': 'correo',
  'god.roleCol': 'rol',
  'god.modified': 'modificado',
  'chrome.kioskBadge': 'kiosco',
  'combo.allInstances': 'todas las instancias',

  'god.writesToPlane': 'escribe en Plane',
  'omni.applyHint': '⏎ aplicar · esc volver',
  'omni.navHint': '↑↓ mover · ⏎ abrir · esc cerrar · escribe',
  'omni.forCommands': 'para comandos',

  'combo.allProjects': 'Todos los proyectos',
  'combo.everyone': 'Todos',
  'combo.mine': 'Míos',

  'cmd.refresh': 'actualizar',
  'cmd.refreshHint': 'volver a consultar el servidor',
  'cmd.newBubble': 'nueva burbuja…',
  'cmd.newBubbleHint': 'crear una burbuja (módulo de Plane)',
  'cmd.birth': 'crear hilo…',
  'cmd.birthHint': 'sólo necesita un nombre',
  'cmd.review': 'marcar revisado…',
  'cmd.unreview': 'quitar revisión…',
  'cmd.close': 'cerrar burbuja (terminada)…',
  'cmd.setOwner': 'asignar responsable…',
  'cmd.setOutcome': 'definir resultado…',
  'cmd.renameBubble': 'renombrar burbuja…',
  'cmd.renameBubbleHint': 'solo el nombre — el contrato no se toca',
  'cmd.signOut': 'cerrar sesión',
  'cmd.viewAll': 'ver todas las instancias',
  'cmd.switchTo': 'cambiar a {slug}',
  'cmd.godPanel': 'modo dios: abrir panel',
  'cmd.godPanelHint': 'todas las opciones de admin (/god-mode)',
  'cmd.godCrossOrg': 'vista de burbujas entre orgs',
  'cmd.godStats': 'modo dios: estadísticas',
  'cmd.godInstances': 'modo dios: instancias',
  'cmd.godRefresh': 'modo dios: limpiar cachés',
  'cmd.godTick': 'modo dios: barrer ahora',

  'toast.cachesRefreshed': 'cachés limpiadas',
  'toast.tickTriggered': 'barrido de enfriamiento lanzado',
  'toast.calibrationReset': 'calibración restaurada a valores de fábrica',
  'toast.copied': 'copiado al portapapeles',
  'toast.copyFailed': 'no se pudo copiar — selecciona y copia a mano',
  'toast.kioskMinted': 'token de kiosco generado',
  'toast.kioskRevoked': 'token de kiosco revocado',
  'toast.applied': 'se aplicaron {n} cambio(s) — el tablero ya los está usando',
  'toast.outboxDropped': 'entrada {id} de la cola descartada',
  'toast.crossOrgOn': 'vista entre orgs activada',
  'toast.crossOrgOff': 'vista entre orgs desactivada',
  'toast.autoStateOn': '{slug}: ahora escribe el estado en Plane',
  'toast.autoStateOff': '{slug}: de nuevo en solo lectura',
  'toast.mirrorMatches': '{slug}: el espejo coincide con Plane',
  'toast.mirrorFindings': '{slug}: {n} hallazgo(s)',
  'toast.pickInstance': 'elige una instancia',

  'tune.hint.pulse': 'el ritmo contra el que se miden ambos niveles',
  'tune.hint.bubble': 'cómo una burbuja llega a 🪦 en vez de 😴',
  'tune.hint.thread': 'cómo un work item se mueve entre bandas',

  // el nivel de ESPACIO DE TRABAJO (un proyecto de Plane)
  // páginas de proyecto: la documentación del espacio de trabajo
  'pages.title': 'Documentos',
  'pages.new': 'Nuevo documento',
  'pages.newInside': 'Nuevo documento dentro de este',
  'pages.empty': 'Aún no hay documentos. Aquí viven las especificaciones y referencias.',
  'pages.pick': 'Elige un documento para leerlo.',
  'pages.titlePlaceholder': 'cómo se llama este documento',
  'pages.rename': 'Renombrar documento',
  'pages.delete': 'Eliminar documento',
  'pages.locked': 'bloqueado en Plane',
  'pages.held': 'guardadas en Bubble Work',
  'pages.heldHint':
    'el Plane de esta instancia no expone páginas en su API, así que Bubble Work es el record de estas — no van a aparecer en la UI de Plane',
  'del.page': 'Se va el documento. Los hilos y las burbujas no se tocan.',

  'omni.workspace': 'espacio de trabajo',
  'omni.scoped': 'mostrando {name}',
  // el cambiador de espacios (estilo Alt+Tab)
  'switch.title': 'Cambiar de espacio de trabajo',
  'switch.hint': 'Tab para avanzar · ← → para moverte · una letra para saltar · suelta Shift para ir · Esc para cancelar',
  'switch.bubbles': '{n} burbuja(s)',
  'ws.menuTitle': 'Acciones del espacio de trabajo',
  'ws.noneScoped': 'Ningún espacio seleccionado',
  'ws.pickFirst': 'Elige un espacio de trabajo en el filtro para renombrarlo o eliminarlo.',
  'ws.rename': 'Renombrar espacio de trabajo',
  'ws.new': 'Nuevo espacio de trabajo…',
  'ws.delete': 'Eliminar espacio de trabajo',
  'ws.formNew': 'Nuevo espacio de trabajo',
  'ws.formRename': 'Renombrar espacio de trabajo',
  'ws.isAProject': 'un proyecto de Plane — el límite de un cuerpo de trabajo',
  'ws.namePlaceholder': 'cómo se llama este cuerpo de trabajo',
  'ws.identifier': 'Identificador',
  'ws.identifierHint': '(opcional — se deriva del nombre)',
  'ws.createBtn': 'Crear',
  'ws.renameBtn': 'Renombrar',
  'ws.saving': 'guardando…',
  'ws.renamed': 'renombrado a {name}',
  'ws.created': '{name} creado — elígelo en el filtro y dale una burbuja',
  'ws.deleted': 'eliminado {name} — {b} burbuja(s), {n} hilo(s)',
  'del.workspace':
    'Se va el espacio completo: {b} burbuja(s) y {n} hilo(s), con todos sus artefactos y comentarios.',
  'del.workspacePrefer':
    'Para retirar un cuerpo de trabajo, cerrar sus burbujas conserva el registro de lo que se hizo.',

  // mover un hilo entre burbujas
  'thread.move': 'Mover a otra burbuja',
  'move.title': 'Mover {what}',
  'move.explain':
    'No se pierde nada — su Brief, Bitácora, comentarios e historial van con él, y devolverlo es el mismo paso.',
  'move.sameWorkspace': 'Solo burbujas del mismo espacio de trabajo: es el límite de Plane, no nuestro.',
  'move.pick': 'Burbuja destino',
  'move.none': 'No hay otra burbuja en este espacio de trabajo a la cual moverlo.',
  'move.go': 'Mover',
  'move.moving': 'moviendo…',
  'move.done': 'movido a {name}',
};

const catalogues: Record<Lang, Record<MsgKey, string>> = { en, es };

// Calibration knobs are the one place the SERVER owns the English: TuningFields
// exists so the CLI and God Mode cannot drift. So only the other languages get
// entries here — an English copy would be a second source of truth that silently
// falls out of step with the server's wording. Keyed by the server's stable
// field key, with "help." prefixing the explanatory paragraph.
const tuneEs: Record<string, string> = {
  cycle_hours: 'Duración del ciclo (horas)',
  dormant_cycles: 'Ciclos antes de quedar dormida',
  decay_cycles: 'Decaimiento del puntaje (ciclos)',
  ownerless_is_dormant: 'Sin responsable pasa a dormida',
  bubble_rip_needs_owner: 'Dormida y sin responsable se lee como 🪦',
  bubble_level_rollup: 'La banda de la burbuja = su hilo más activo',
  thread_grace_cycles: 'Gracia para recién creados (ciclos)',
  thread_rip_needs_owner: 'Sin asignar y dormido se lee como 🪦',
  pulse_cycles: 'Un comentario mantiene vivo el hilo (ciclos)',
  thread_terminal_state_wins: 'Las columnas terminado/cancelado de Plane mandan',

  'help.cycle_hours':
    'El pulso contra el que se mide la recencia. Solo se usa cuando el proyecto no tiene un ciclo activo en Plane — un ciclo real siempre manda.',
  'help.dormant_cycles':
    'Cuántos ciclos de silencio dejan algo dormido. 2 = «nada este ciclo ni el anterior». Más bajo hunde el tablero más rápido.',
  'help.decay_cycles':
    'Escala el puntaje de flotabilidad usado para ordenar. Más alto mantiene las cosas flotando más tiempo; no cambia las bandas.',
  'help.ownerless_is_dormant':
    'Algo sin nadie que responda queda dormido una vez que se calla. Lo que sigue produciendo se mantiene caliente igual.',
  'help.bubble_level_rollup':
    'La burbuja se sitúa en la banda de su hilo sin terminar más activo. Apagado, se clasifica por la unión de su evidencia — que cuenta la creación de un hilo como salida de la burbuja, así que una burbuja llena de work items nuevos sin tocar se lee 🔥. Cerrar y revisar explícitamente siempre mandan.',
  'help.bubble_rip_needs_owner':
    'Una burbuja dormida sin responsable se lee 🪦 en vez de 😴. Una que nunca produjo nada es 🪦 de todas formas. Solo se usa cuando la banda NO viene de sus hilos.',
  'help.thread_grace_cycles':
    'Cuánto tiempo un hilo nuevo que no ha producido nada sigue 😴 antes de llamarse 🪦. 0 = sin gracia.',
  'help.thread_rip_needs_owner': 'Un hilo dormido sin asignar se lee 🪦 en vez de 😴.',
  'help.pulse_cycles':
    'Cuánto tiempo un comentario mantiene un hilo fuera de 🪦. Los comentarios son presencia, no producción — nunca calientan un hilo a 🔥, pero no declaramos algo abandonado mientras la gente sigue discutiéndolo. 0 apaga el pulso.',
  'help.thread_terminal_state_wins':
    'Deja que Plane decida los dos estados terminales: completado → 🏆, cancelado → 🪦. Cancelar se deshace con producción real (una edición de bitácora o una revisión resucitan el hilo) pero nunca con comentarios. Las demás columnas se ignoran igual — mover una tarjeta es movimiento, no evidencia.',
};

function detect(): Lang {
  const stored = localStorage.getItem(KEY);
  if (stored === 'en' || stored === 'es') return stored;
  // navigator.language is "es-MX", "es", "en-GB"… — only the primary subtag
  // matters, and anything we don't speak falls back to English.
  return (navigator.language || '').toLowerCase().startsWith('es') ? 'es' : 'en';
}

class I18n {
  lang = $state<Lang>(detect());

  constructor() {
    this.apply();
  }

  set(l: Lang): void {
    this.lang = l;
    localStorage.setItem(KEY, l);
    this.apply();
  }

  toggle(): void {
    this.set(this.lang === 'en' ? 'es' : 'en');
  }

  /** <html lang> so screen readers and hyphenation follow the choice too. */
  private apply(): void {
    document.documentElement.setAttribute('lang', this.lang);
  }

  get label(): string {
    return this.lang === 'es' ? 'Español' : 'English';
  }

  /** Region flag for the badge. Spanish here is Mexican Spanish, so MX rather
   *  than ES — the wording ("¿qué onda?" register, "responsable") is written for
   *  that audience, and a Spain flag would misstate it. */
  get flag(): string {
    return this.lang === 'es' ? '🇲🇽' : '🇺🇸';
  }

  /** Short code for the badge label, matching the theme badge's terseness. */
  get code(): string {
    return this.lang === 'es' ? 'MX' : 'US';
  }

  /**
   * Translate a key, substituting {placeholders}.
   *
   * A missing key returns the key itself rather than an empty string: a label
   * reading "bubble.owner" is an obvious bug, whereas a blank one looks like a
   * design choice and survives to production.
   */
  t(key: MsgKey, args?: Record<string, string | number>): string {
    const s = catalogues[this.lang][key] ?? en[key] ?? key;
    if (!args) return s;
    return s.replace(/\{(\w+)\}/g, (m, k) => (k in args ? String(args[k]) : m));
  }

  /** English-style plurals (one vs other), which both languages share here. */
  plural(key: MsgKey, n: number, args?: Record<string, string | number>): string {
    const k = (n === 1 ? key : `${key}_plural`) as MsgKey;
    return this.t(k in en ? k : key, { n, ...args });
  }

  /**
   * The heat sentence under a bubble. The server sends a stable code plus the
   * English sentence; we translate the code and fall back to the server's own
   * words when it sends a code this build has never heard of — a newer server
   * then degrades to English rather than to a raw identifier.
   */
  /** A calibration knob's label. Same contract as reason(): the server owns the
   *  English (so the CLI and God Mode cannot drift), we translate by its stable
   *  key, and a knob this build has not heard of shows the server's own words. */
  tuning(key: string, fallback: string): string {
    // Group labels/hints are client-owned and live in the shared catalogue.
    const k = `tune.${key}` as MsgKey;
    if (k in en) return this.t(k);
    // Field labels and help are server-owned: English comes straight from it,
    // and an unknown key in any language shows the server's own words.
    if (this.lang === 'es') return tuneEs[key] ?? fallback;
    return fallback;
  }

  reason(code: string | undefined, fallback: string, args?: Record<string, string>): string {
    if (!code) return fallback;
    const key = `reason.${code}` as MsgKey;
    return key in en ? this.t(key, args) : fallback;
  }
}

export const i18n = new I18n();

/** Shorthand so markup reads `{t('bubble.open')}`. Reactive: reading i18n.lang
 *  inside makes every call site re-render on a language change. */
export function t(key: MsgKey, args?: Record<string, string | number>): string {
  return i18n.t(key, args);
}

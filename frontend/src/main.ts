// Self-hosted, from npm (Fontsource). No request leaves the machine the app is
// served from, which for company software is the difference between a typeface
// and a third party watching who reads what. Both OFL: commercial use, nothing
// to buy.
//
// Boogaloo ships ONE weight, and that is a constraint rather than an oversight:
// see `.display` in app.css for why nothing may embolden it.
import '@fontsource/boogaloo/400.css';
import '@fontsource-variable/dm-sans';

import './app.css';
import './lib/theme.svelte';
import { mount } from 'svelte';
import App from './App.svelte';
import { letEditorsKeepEscape } from './lib/escape';

// Escape inside a text editor is the editor's — see `lib/escape.ts` for the
// capture-phase reason this has to be installed here, at the window, rather
// than by whichever component happens to be hosting the editor.
letEditorsKeepEscape();

// No right-click we did not ask for.
//
// The listener is on the document and does NOT stop propagation, so it runs
// after the target's own handler and only acts on what nothing claimed:
// `defaultPrevented` is exactly the question "did a component already handle
// this?". Zag's context menus call preventDefault themselves, so an orb's menu
// still opens and the browser's does not appear behind it.
//
// `data-native-menu` on an element (or any ancestor) opts back in, for the day
// something genuinely wants the browser's own menu — a field where copy and
// paste matter more than the illusion of an application.
addEventListener(
  'contextmenu',
  (e) => {
    if (e.defaultPrevented) return;
    if ((e.target as Element | null)?.closest?.('[data-native-menu]')) return;
    e.preventDefault();
  },
  { capture: false },
);

mount(App, { target: document.getElementById('app')! });

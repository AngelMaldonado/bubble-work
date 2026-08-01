import './app.css';
import './lib/theme.svelte'; // side-effect: apply saved theme + track system changes
import { mount } from 'svelte';
import App from './App.svelte';

mount(App, { target: document.getElementById('app')! });
